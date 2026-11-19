package acquire

import (
	"context"
	"errors"
	"time"

	"github.com/ScotyLucas/kinora/internal/store"
)

// Errors shown to the person asking.
var (
	ErrKids        = errors.New("gyerekprofilból nem lehet filmet kérni")
	ErrHave        = errors.New("ez a film már megvan a könyvtárban")
	ErrQuota       = errors.New("elérted a heti kéréskorlátot – próbáld újra pár nap múlva")
	ErrNotYours    = errors.New("ez nem a te kérésed")
	ErrRequestBusy = errors.New("a kérés már letöltés alatt van – csak az admin szakíthatja meg")
)

// Requester is who asks for a film.
type Requester struct {
	UserID    int64
	IsAdmin   bool
	ProfileID int64
	Kids      bool
}

// RetryAfterError is how long a request waits after the indexers or the client failed.
const RetryAfterError = 30 * time.Minute

// MaxAttempts is how many failed downloads a request may use up before it is given up.
const MaxAttempts = 3

func (m *Manager) autoApprove(admin bool) bool {
	if m.st.Setting("acq_auto_approve") == "all" {
		return true
	}
	return admin
}

func (m *Manager) retryWait() time.Duration {
	h := m.st.SettingInt("acq_retry_hours", 6)
	if h < 1 {
		h = 1
	}
	return time.Duration(h) * time.Hour
}

// Request records that someone wants a film. A film already asked for returns the open request.
// libraryID is the films library the requester chose (0: the only one).
func (m *Manager) Request(ctx context.Context, who Requester, tmdbID, libraryID int64) (*store.AcqRequest, error) {
	if !m.Enabled() {
		return nil, ErrDisabled
	}
	if who.Kids {
		return nil, ErrKids
	}
	if have, _ := m.st.MovieTmdbIDs([]int64{tmdbID}); have[tmdbID] > 0 {
		return nil, ErrHave
	}
	if q, err := m.st.OpenRequestFor("movie", tmdbID); err == nil {
		return q, nil
	}
	if !who.IsAdmin {
		if quota := m.st.SettingInt("acq_quota_week", 0); quota > 0 && m.st.CountUserRequests(who.UserID, time.Now().Add(-7*24*time.Hour).Unix()) >= quota {
			return nil, ErrQuota
		}
	}
	lib, err := m.LibraryFor("movie", libraryID)
	if err != nil {
		return nil, err
	}
	mv, err := m.movies.Movie(ctx, tmdbID)
	if err != nil {
		return nil, err
	}
	q := &store.AcqRequest{Kind: "movie", TmdbID: mv.ID, ImdbID: mv.IMDb, Title: mv.Title, Year: mv.Year, Poster: mv.Poster,
		UserID: &who.UserID, Status: store.ReqPending, LibraryID: &lib.ID, LibraryName: lib.Name}
	if who.ProfileID > 0 {
		q.ProfileID = &who.ProfileID
	}
	if m.autoApprove(who.IsAdmin) {
		q.Status = store.ReqApproved
	}
	if err := m.st.AddAcqRequest(q); err != nil {
		return nil, err
	}
	m.log.Info("acquire: film requested", "title", mv.Title, "user", who.UserID, "status", q.Status)
	if q.Status == store.ReqApproved {
		m.Wake()
	}
	return q, nil
}

// Decide approves or refuses a request (also re-approves a failed or waiting one).
func (m *Manager) Decide(id, adminID int64, approve bool, note string) error {
	q, err := m.st.AcqRequest(id)
	if err != nil {
		return err
	}
	switch q.Status {
	case store.ReqPending, store.ReqWaiting, store.ReqFailed, store.ReqDenied:
	default:
		return errors.New("ez a kérés már nem dönthető el")
	}
	if !approve {
		return m.st.DecideRequest(id, store.ReqDenied, note, adminID)
	}
	if err := m.st.DecideRequest(id, store.ReqApproved, "", adminID); err != nil {
		return err
	}
	m.Wake()
	return nil
}

// CancelRequest withdraws a request that has not started downloading.
func (m *Manager) CancelRequest(id int64, who Requester) error {
	q, err := m.st.AcqRequest(id)
	if err != nil {
		return err
	}
	if !who.IsAdmin && (q.UserID == nil || *q.UserID != who.UserID) {
		return ErrNotYours
	}
	switch q.Status {
	case store.ReqPending, store.ReqApproved, store.ReqWaiting:
		return m.st.SetRequestStatus(id, store.ReqCanceled, "", 0)
	case store.ReqDownloading:
		return ErrRequestBusy
	}
	return errors.New("ez a kérés már lezárult")
}

// LibraryFor is the library a film (kind "movie") or a series goes to. id > 0 is the one
// chosen: a film never goes to a series library, nor a series to a films library. With 0
// the only library of that kind is used; with several, someone has to choose.
func (m *Manager) LibraryFor(kind string, id int64) (*store.Library, error) {
	want, other := "movies", "shows"
	if kind == "series" {
		want, other = "shows", "movies"
	}
	if id > 0 {
		l, err := m.st.Library(id)
		if err != nil {
			return nil, errors.New("a kiválasztott könyvtár nem létezik")
		}
		if l.Kind == other {
			if kind == "series" {
				return nil, errors.New("sorozat csak sorozatkönyvtárba kerülhet")
			}
			return nil, errors.New("film csak filmkönyvtárba kerülhet")
		}
		return l, nil
	}
	libs, err := m.st.Libraries()
	if err != nil {
		return nil, err
	}
	var found []*store.Library
	for _, l := range libs {
		if l.Kind == want {
			found = append(found, l)
		}
	}
	switch {
	case len(found) == 1:
		return found[0], nil
	case len(found) > 1:
		return nil, errors.New("válaszd ki, melyik könyvtárba kerüljön")
	case kind == "series":
		return nil, errors.New("nincs sorozatkönyvtár, ahová a sorozat kerülhetne")
	}
	return nil, errors.New("nincs filmkönyvtár, ahová a film kerülhetne")
}

func idOf(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

// bestRelease is the first acceptable result that is not on the blocklist.
func (m *Manager) bestRelease(rs []Result) *Result {
	hashes := make([]string, 0, len(rs))
	for _, r := range rs {
		if r.InfoHash != "" {
			hashes = append(hashes, r.InfoHash)
		}
	}
	bad := m.st.Blocklisted(hashes)
	for i := range rs {
		if rs[i].Rejected == "" && !bad[rs[i].InfoHash] {
			return &rs[i]
		}
	}
	return nil
}

// ProcessRequests works on the approved requests and on the waiting ones whose time has come:
// search, pick the best release, download it. Run calls it on every round.
func (m *Manager) ProcessRequests(ctx context.Context) {
	if !m.Enabled() {
		return
	}
	m.reqMu.Lock()
	defer m.reqMu.Unlock()
	due, err := m.st.DueRequests(time.Now().Unix(), 5)
	if err != nil || len(due) == 0 {
		return
	}
	var ids []int64
	for _, q := range due {
		if q.Kind == "movie" {
			ids = append(ids, q.TmdbID)
		}
	}
	have, _ := m.st.MovieTmdbIDs(ids)
	for _, q := range due {
		if ctx.Err() != nil {
			return
		}
		if q.Kind == "series" { // followed from now on: the series round searches and downloads
			if err := m.followForRequest(ctx, q); err != nil {
				m.st.SetRequestStatus(q.ID, store.ReqWaiting, err.Error(), time.Now().Add(RetryAfterError).Unix())
				continue
			}
			m.st.SetRequestStatus(q.ID, store.ReqDownloading, "", 0)
			continue
		}
		if have[q.TmdbID] > 0 { // arrived some other way meanwhile
			m.st.MarkFilmAvailable("movie", q.TmdbID)
			continue
		}
		m.fulfil(ctx, q)
	}
}

func (m *Manager) fulfil(ctx context.Context, q *store.AcqRequest) {
	later := func(note string, wait time.Duration) {
		m.st.SetRequestStatus(q.ID, store.ReqWaiting, note, time.Now().Add(wait).Unix())
	}
	lib, err := m.LibraryFor("movie", idOf(q.LibraryID))
	if err != nil {
		later(err.Error(), RetryAfterError)
		return
	}
	rs, err := m.Releases(ctx, q.TmdbID)
	if err != nil {
		later(err.Error(), RetryAfterError)
		return
	}
	rel := m.bestRelease(rs.Results)
	if rel == nil {
		later("még nincs a minőségprofilnak megfelelő kiadás – később újra keresem", m.retryWait())
		return
	}
	uid := int64(0)
	if q.UserID != nil {
		uid = *q.UserID
	}
	if _, err := m.Grab(ctx, GrabInput{TmdbID: q.TmdbID, LibraryID: lib.ID, Release: rel, UserID: uid, RequestID: q.ID}); err != nil {
		later(err.Error(), RetryAfterError)
		return
	}
	m.st.SetRequestStatus(q.ID, store.ReqDownloading, "", 0)
}

// grabFailed handles a download that went wrong: a bad release goes on the blocklist and its
// request tries the next best one (until MaxAttempts); otherwise the request fails with the reason.
func (m *Manager) grabFailed(g *store.AcqGrab, reason string, badRelease bool) {
	m.st.SetAcqGrabStatus(g.ID, store.GrabFailed, reason, "")
	if badRelease {
		m.st.Blocklist(g.InfoHash, g.ReleaseTitle, reason)
	}
	if g.SeriesID != nil && badRelease {
		m.st.ResetSearchForGrab(g.ID) // the episodes are searched again: the next best release
	}
	if g.RequestID == nil {
		return
	}
	if !badRelease {
		m.st.SetRequestStatus(*g.RequestID, store.ReqFailed, reason, 0)
		return
	}
	if n := m.st.BumpRequestAttempts(*g.RequestID); n >= MaxAttempts {
		m.st.SetRequestStatus(*g.RequestID, store.ReqFailed, reason, 0)
		return
	}
	m.st.SetRequestStatus(*g.RequestID, store.ReqApproved, reason, 0) // next round: the next best release
	m.Wake()
}

// RequestSeries records that someone wants a series: "all" its seasons, the "future" episodes, or some seasons ("1,2").
// libraryID is the series library the requester chose (0: the only one).
func (m *Manager) RequestSeries(ctx context.Context, who Requester, tmdbID int64, seasons string, libraryID int64) (*store.AcqRequest, error) {
	if !m.Enabled() {
		return nil, ErrDisabled
	}
	if who.Kids {
		return nil, ErrKids
	}
	seasons, err := ValidSeasons(seasons)
	if err != nil {
		return nil, err
	}
	if q, err := m.st.OpenRequestFor("series", tmdbID); err == nil && q.Seasons == seasons {
		return q, nil
	}
	if !who.IsAdmin {
		if quota := m.st.SettingInt("acq_quota_week", 0); quota > 0 && m.st.CountUserRequests(who.UserID, time.Now().Add(-7*24*time.Hour).Unix()) >= quota {
			return nil, ErrQuota
		}
	}
	lib, err := m.LibraryFor("series", libraryID)
	if err != nil {
		return nil, err
	}
	info, err := m.movies.TV(ctx, tmdbID)
	if err != nil {
		return nil, err
	}
	q := &store.AcqRequest{Kind: "series", TmdbID: info.ID, ImdbID: info.IMDb, Title: info.Title, Year: info.Year, Poster: info.Poster,
		UserID: &who.UserID, Status: store.ReqPending, Seasons: seasons, LibraryID: &lib.ID, LibraryName: lib.Name}
	if who.ProfileID > 0 {
		q.ProfileID = &who.ProfileID
	}
	if m.autoApprove(who.IsAdmin) {
		q.Status = store.ReqApproved
	}
	if err := m.st.AddAcqRequest(q); err != nil {
		return nil, err
	}
	m.log.Info("acquire: series requested", "title", info.Title, "seasons", seasons, "user", who.UserID, "status", q.Status)
	if q.Status == store.ReqApproved {
		m.Wake()
	}
	return q, nil
}
