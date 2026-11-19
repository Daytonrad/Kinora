package acquire

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ScotyLucas/kinora/internal/meta"
	"github.com/ScotyLucas/kinora/internal/store"
)

// Movies is the TMDB side the manager needs (meta.Fetcher; a fake in tests).
type Movies interface {
	Search(ctx context.Context, kind, query string, year int) ([]meta.SearchResult, error)
	Movie(ctx context.Context, id int64) (meta.MovieInfo, error)
}

// Manager runs the acquisition: search, grab, follow, import.
type Manager struct {
	st     *store.Store
	movies Movies
	scan   func(libID int64) // start a library scan after an import
	log    *slog.Logger
	hc     *http.Client
	tz     *Torznab

	// NewClient builds a download client; tests swap it for a fake.
	NewClient func(cfg ClientConfig) (DownloadClient, error)
	// Interval between status polls while something downloads.
	Interval time.Duration

	mu      sync.Mutex
	clients map[int64]cachedClient
	wake    chan struct{}
	pollMu  sync.Mutex
}

type cachedClient struct {
	cfg ClientConfig
	c   DownloadClient
}

// New makes the manager. Run must be started for downloads to be followed and imported.
func New(st *store.Store, movies Movies, scan func(libID int64), log *slog.Logger) *Manager {
	hc := &http.Client{Timeout: 30 * time.Second}
	m := &Manager{st: st, movies: movies, scan: scan, log: log, hc: hc, tz: &Torznab{HC: hc},
		Interval: 20 * time.Second, clients: map[int64]cachedClient{}, wake: make(chan struct{}, 1)}
	m.NewClient = func(cfg ClientConfig) (DownloadClient, error) { return NewClient(cfg, hc) }
	return m
}

// Errors shown to the admin.
var (
	ErrDisabled = errors.New("a beszerzés ki van kapcsolva (Admin → Beszerzés → Beállítások)")
	ErrNoClient = errors.New("nincs bekapcsolt letöltőkliens (Admin → Beszerzés → Beállítások)")
	ErrNoIndex  = errors.New("nincs bekapcsolt indexer (Admin → Beszerzés → Beállítások)")
)

// Enabled reports the acq_enabled setting.
func (m *Manager) Enabled() bool { return m.st.Setting("acq_enabled") == "1" }

// Profile is the stored quality profile.
func (m *Manager) Profile() Profile { return ParseProfile(m.st.Setting("acq_profile")) }

// ------------------------------------------------------------------ lookup

// LookupItem is a TMDB film with what Kinora already knows about it.
type LookupItem struct {
	meta.SearchResult
	InLibrary bool   `json:"in_library"`
	Grab      string `json:"grab,omitempty"` // state of the latest download, if any
}

// Lookup searches TMDB for films.
func (m *Manager) Lookup(ctx context.Context, q string, year int) ([]LookupItem, error) {
	res, err := m.movies.Search(ctx, "movie", q, year)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, len(res))
	for i, r := range res {
		ids[i] = r.ID
	}
	have, _ := m.st.MovieTmdbIDs(ids)
	grabs, _ := m.st.GrabbedTmdbIDs(ids)
	out := make([]LookupItem, len(res))
	for i, r := range res {
		out[i] = LookupItem{SearchResult: r, InLibrary: have[r.ID], Grab: grabs[r.ID]}
	}
	return out, nil
}

// ------------------------------------------------------------------ releases

// IndexerError is one indexer that failed during a search.
type IndexerError struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Error string `json:"error"`
}

// Releases is the answer for one film.
type Releases struct {
	Movie   meta.MovieInfo `json:"movie"`
	Results []Result       `json:"results"`
	Errors  []IndexerError `json:"errors"`
}

func (m *Manager) indexers() []Indexer {
	list, _ := m.st.AcqIndexers()
	var out []Indexer
	for _, x := range list {
		if x.Enabled {
			out = append(out, Indexer{ID: x.ID, Name: x.Name, URL: x.URL, APIKey: x.APIKey, Categories: x.Categories})
		}
	}
	return out
}

// Releases searches every enabled indexer for a film and scores what they return, best first.
func (m *Manager) Releases(ctx context.Context, tmdbID int64) (*Releases, error) {
	mv, err := m.movies.Movie(ctx, tmdbID)
	if err != nil {
		return nil, err
	}
	ixs := m.indexers()
	if len(ixs) == 0 {
		return nil, ErrNoIndex
	}
	title := mv.EnglishTitle
	if title == "" {
		title = mv.OriginalTitle
	}
	q := Query{IMDb: mv.IMDb, Title: title, Year: mv.Year}
	var mu sync.Mutex
	var wg sync.WaitGroup
	out := &Releases{Movie: mv, Results: []Result{}, Errors: []IndexerError{}}
	for _, ix := range ixs {
		wg.Add(1)
		go func(ix Indexer) {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, 40*time.Second)
			defer cancel()
			res, err := m.tz.Search(cctx, ix, q)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				m.st.MarkAcqIndexer(ix.ID, err.Error())
				out.Errors = append(out.Errors, IndexerError{ID: ix.ID, Name: ix.Name, Error: err.Error()})
				return
			}
			m.st.MarkAcqIndexer(ix.ID, "")
			out.Results = append(out.Results, res...)
		}(ix)
	}
	wg.Wait()
	out.Results = rank(dedupe(out.Results), m.Profile(), mv)
	return out, nil
}

// dedupe keeps one result per info hash (the best seeded) across indexers.
func dedupe(rs []Result) []Result {
	seen := map[string]int{}
	out := rs[:0:0]
	for _, r := range rs {
		if r.InfoHash == "" {
			out = append(out, r)
			continue
		}
		if i, ok := seen[r.InfoHash]; ok {
			if r.Seeders > out[i].Seeders {
				out[i] = r
			}
			continue
		}
		seen[r.InfoHash] = len(out)
		out = append(out, r)
	}
	return out
}

// rank scores and orders results: acceptable ones by score (then seeders), rejected ones last.
func rank(rs []Result, p Profile, mv meta.MovieInfo) []Result {
	for i := range rs {
		r := &rs[i]
		if r.IMDb != "" && mv.IMDb != "" && r.IMDb != mv.IMDb {
			r.Rejected = RejectTitle
			continue
		}
		if r.IMDb == "" || mv.IMDb == "" {
			if !TitleMatches(r.Title, r.Release, mv.Year, mv.Title, mv.OriginalTitle, mv.EnglishTitle) {
				r.Rejected = RejectTitle
				continue
			}
		}
		r.Score, r.Rejected = p.Score(r.Release, r.Size, r.Seeders)
	}
	sort.SliceStable(rs, func(i, j int) bool {
		a, b := rs[i], rs[j]
		if (a.Rejected == "") != (b.Rejected == "") {
			return a.Rejected == ""
		}
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		return a.Seeders > b.Seeders
	})
	return rs
}

// ------------------------------------------------------------------ grab

// GrabInput says what to download and where to put it.
type GrabInput struct {
	TmdbID    int64
	LibraryID int64
	Release   *Result // nil: the best acceptable release
	UserID    int64
}

func (m *Manager) client(cfg *store.AcqClient) (DownloadClient, ClientConfig, error) {
	cc := ClientConfig{ID: cfg.ID, Kind: cfg.Kind, URL: cfg.URL, Username: cfg.Username, Password: cfg.Password,
		Category: cfg.Category, SavePath: cfg.SavePath, PathRemote: cfg.PathRemote, PathLocal: cfg.PathLocal}
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.clients[cfg.ID]; ok && c.cfg == cc {
		return c.c, cc, nil
	}
	c, err := m.NewClient(cc)
	if err != nil {
		return nil, cc, err
	}
	m.clients[cfg.ID] = cachedClient{cfg: cc, c: c}
	return c, cc, nil
}

func (m *Manager) firstClient() (*store.AcqClient, error) {
	list, err := m.st.AcqClients()
	if err != nil {
		return nil, err
	}
	for _, c := range list {
		if c.Enabled {
			return c, nil
		}
	}
	return nil, ErrNoClient
}

// Grab starts a download.
func (m *Manager) Grab(ctx context.Context, in GrabInput) (*store.AcqGrab, error) {
	if !m.Enabled() {
		return nil, ErrDisabled
	}
	lib, err := m.st.Library(in.LibraryID)
	if err != nil {
		return nil, errors.New("válassz célkönyvtárat")
	}
	if lib.Kind == "shows" {
		return nil, errors.New("filmet filmkönyvtárba lehet letölteni")
	}
	cfg, err := m.firstClient()
	if err != nil {
		return nil, err
	}
	cl, _, err := m.client(cfg)
	if err != nil {
		return nil, err
	}
	mv, err := m.movies.Movie(ctx, in.TmdbID)
	if err != nil {
		return nil, err
	}
	rel := in.Release
	if rel == nil {
		rs, err := m.Releases(ctx, in.TmdbID)
		if err != nil {
			return nil, err
		}
		for i := range rs.Results {
			if rs.Results[i].Rejected == "" {
				rel = &rs.Results[i]
				break
			}
		}
		if rel == nil {
			return nil, errors.New("nincs a minőségprofilnak megfelelő kiadás")
		}
	} else {
		if _, err := m.st.AcqIndexer(rel.IndexerID); err != nil {
			return nil, errors.New("ismeretlen indexer")
		}
		// scored again on the server: the client only says which release it wants
		r := ParseRelease(rel.Title)
		rel.Release, rel.Quality = r, r.QualityLabel()
		rel.Score, _ = m.Profile().Score(r, rel.Size, rel.Seeders)
	}
	f, err := FetchRelease(ctx, m.hc, *rel)
	if err != nil {
		return nil, err
	}
	if g, err := m.st.ActiveGrabForHash(f.InfoHash); err == nil {
		return g, nil // already downloading
	}
	if err := cl.Add(ctx, AddRequest{Fetched: f, Tag: "kinora"}); err != nil {
		return nil, err
	}
	g := &store.AcqGrab{Kind: "movie", TmdbID: mv.ID, ImdbID: mv.IMDb, Title: mv.Title, Year: mv.Year, LibraryID: &lib.ID,
		ReleaseTitle: rel.Title, IndexerID: &rel.IndexerID, IndexerName: rel.Indexer, ClientID: &cfg.ID, InfoHash: f.InfoHash,
		Size: rel.Size, Quality: rel.Quality, Score: rel.Score}
	if in.UserID > 0 {
		g.RequestedBy = &in.UserID
	}
	if err := m.st.AddAcqGrab(g); err != nil {
		return nil, err
	}
	m.log.Info("acquire: download started", "title", mv.Title, "release", rel.Title, "hash", f.InfoHash)
	m.Wake()
	return g, nil
}

// ------------------------------------------------------------------ follow + import

// Wake makes the poller look now.
func (m *Manager) Wake() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

// Run follows downloads until ctx ends.
func (m *Manager) Run(ctx context.Context) {
	t := time.NewTicker(m.Interval)
	defer t.Stop()
	m.Poll(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-m.wake:
		}
		m.Poll(ctx)
	}
}

// Poll asks the clients about every running download once, and imports the finished ones.
func (m *Manager) Poll(ctx context.Context) {
	m.pollMu.Lock()
	defer m.pollMu.Unlock()
	grabs, err := m.st.AcqGrabs(500, store.GrabDownloading, store.GrabImporting)
	if err != nil || len(grabs) == 0 {
		return
	}
	byClient := map[int64][]*store.AcqGrab{}
	for _, g := range grabs {
		if g.ClientID == nil {
			m.st.SetAcqGrabStatus(g.ID, store.GrabFailed, "a letöltőkliens törölve lett", "")
			continue
		}
		byClient[*g.ClientID] = append(byClient[*g.ClientID], g)
	}
	for cid, gs := range byClient {
		cfg, err := m.st.AcqClient(cid)
		if err != nil {
			continue
		}
		cl, cc, err := m.client(cfg)
		if err != nil {
			continue
		}
		hashes := make([]string, len(gs))
		for i, g := range gs {
			hashes[i] = g.InfoHash
		}
		cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		st, err := cl.Status(cctx, hashes)
		cancel()
		if err != nil {
			m.log.Warn("acquire: download client unreachable", "client", cfg.Name, "err", err)
			continue // try again on the next round
		}
		for _, g := range gs {
			s, ok := st[g.InfoHash]
			switch {
			case !ok:
				m.st.SetAcqGrabStatus(g.ID, store.GrabFailed, "a letöltés eltűnt a letöltőkliensből", "")
			case s.Failed:
				m.st.SetAcqGrabStatus(g.ID, store.GrabFailed, "a letöltőkliens hibát jelez: "+s.State, "")
			case s.Done:
				m.importGrab(g, cc, s)
			default:
				m.st.UpdateAcqGrabProgress(g.ID, s.Progress, s.ETA, s.Speed)
			}
		}
	}
}

// MapPath turns a path as the download client sees it into one Kinora can open.
func MapPath(p, remote, local string) string {
	if remote == "" || local == "" {
		return p
	}
	norm := func(s string) string { return strings.TrimRight(strings.ReplaceAll(s, "\\", "/"), "/") }
	pp, rr := norm(p), norm(remote)
	if pp != rr && !strings.HasPrefix(pp, rr+"/") {
		return p
	}
	rest := strings.TrimPrefix(pp, rr)
	return filepath.Join(local, filepath.FromSlash(rest))
}

func (m *Manager) importGrab(g *store.AcqGrab, cc ClientConfig, s TorrentStatus) {
	if g.LibraryID == nil {
		m.st.SetAcqGrabStatus(g.ID, store.GrabFailed, "a célkönyvtár törölve lett", "")
		return
	}
	lib, err := m.st.Library(*g.LibraryID)
	if err != nil {
		m.st.SetAcqGrabStatus(g.ID, store.GrabFailed, "a célkönyvtár törölve lett", "")
		return
	}
	m.st.SetAcqGrabStatus(g.ID, store.GrabImporting, "", "")
	src := MapPath(s.ContentPath, cc.PathRemote, cc.PathLocal)
	res, err := Import(ImportMovie{Source: src, LibRoot: lib.Path, Title: g.Title, Year: g.Year, IMDb: g.ImdbID,
		Quality: g.Quality, Mode: m.st.Setting("acq_import_mode")})
	if err != nil {
		msg := err.Error()
		if cc.PathRemote == "" && strings.Contains(msg, "nem található") {
			msg += " – ha a letöltőkliens máshol látja a fájlokat, állíts be útvonal-leképezést"
		}
		m.st.SetAcqGrabStatus(g.ID, store.GrabFailed, msg, "")
		m.log.Warn("acquire: import failed", "title", g.Title, "src", src, "err", err)
		return
	}
	m.st.SetAcqGrabStatus(g.ID, store.GrabDone, "", res.Path)
	m.log.Info("acquire: imported", "title", g.Title, "path", res.Path, "hardlink", res.Linked)
	if m.scan != nil {
		m.scan(lib.ID)
	}
}

// Retry imports a failed download again (after fixing the path mapping, for example).
func (m *Manager) Retry(ctx context.Context, id int64) error {
	g, err := m.st.AcqGrab(id)
	if err != nil {
		return err
	}
	if g.Status != store.GrabFailed {
		return errors.New("csak sikertelen letöltés próbálható újra")
	}
	m.st.SetAcqGrabStatus(g.ID, store.GrabDownloading, "", "")
	m.Wake()
	return nil
}

// Remove stops following a download and takes it out of the client (with its files when asked).
func (m *Manager) Remove(ctx context.Context, id int64, deleteFiles bool) error {
	g, err := m.st.AcqGrab(id)
	if err != nil {
		return err
	}
	if g.ClientID != nil && g.Status != store.GrabRemoved {
		if cfg, err := m.st.AcqClient(*g.ClientID); err == nil {
			if cl, _, err := m.client(cfg); err == nil {
				if err := cl.Remove(ctx, g.InfoHash, deleteFiles); err != nil {
					return fmt.Errorf("a letöltőkliens nem törölte: %w", err)
				}
			}
		}
	}
	return m.st.SetAcqGrabStatus(g.ID, store.GrabRemoved, "", "")
}

// TestIndexer checks an indexer configuration without saving it.
func (m *Manager) TestIndexer(ctx context.Context, ix Indexer) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return m.tz.Caps(cctx, ix)
}

// TestClient checks a download client configuration without saving it.
func (m *Manager) TestClient(ctx context.Context, cfg ClientConfig) (string, error) {
	c, err := m.NewClient(cfg)
	if err != nil {
		return "", err
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return c.Test(cctx)
}
