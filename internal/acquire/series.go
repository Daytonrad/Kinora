package acquire

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ScotyLucas/kinora/internal/meta"
	"github.com/ScotyLucas/kinora/internal/store"
)

// Series acquisition (M3): Kinora follows a series, knows which episodes have aired and which are in the
// library, searches the missing ones (a whole season as one pack when it has fully aired), watches the
// indexers' newest releases for new episodes, and imports into "Show (Year)/Season 01/…S01E03….mkv".

// Monitor modes of a followed series.
const (
	MonitorAll     = "all"     // every season, also the ones to come
	MonitorFuture  = "future"  // only episodes that have not aired yet when following starts
	MonitorSeasons = "seasons" // the chosen seasons only
	MonitorNone    = "none"    // known, but nothing is searched
)

// Intervals of the series work.
const (
	SeriesRefreshEvery = 12 * time.Hour   // episode list and air dates from TMDB
	RSSEvery           = 15 * time.Minute // indexers' newest releases
	searchesPerRound   = 8                // indexer searches per round, over every series
)

// SeriesInput says which series to follow and how.
type SeriesInput struct {
	TmdbID    int64
	LibraryID int64 // 0: the series library from the settings
	Monitor   string
	Seasons   []int // with MonitorSeasons
	UserID    int64
}

func today() string { return time.Now().Format("2006-01-02") }

func (m *Manager) targetTVLibrary(id int64) (*store.Library, error) {
	return m.LibraryFor("series", id)
}

// AddSeries starts following a series, or changes the library and what is followed of one already followed.
func (m *Manager) AddSeries(ctx context.Context, in SeriesInput) (*store.AcqSeries, error) {
	if !m.Enabled() {
		return nil, ErrDisabled
	}
	switch in.Monitor {
	case MonitorAll, MonitorFuture, MonitorSeasons, MonitorNone:
	default:
		return nil, errors.New("ismeretlen figyelési mód")
	}
	if in.Monitor == MonitorSeasons && len(in.Seasons) == 0 {
		return nil, errors.New("válassz legalább egy évadot")
	}
	lib, err := m.targetTVLibrary(in.LibraryID)
	if err != nil {
		return nil, err
	}
	info, err := m.movies.TV(ctx, in.TmdbID)
	if err != nil {
		return nil, err
	}
	m.seriesMu.Lock()
	defer m.seriesMu.Unlock()
	x, err := m.st.AcqSeriesByTmdb(in.TmdbID)
	if err != nil {
		x = &store.AcqSeries{TmdbID: in.TmdbID}
		if in.UserID > 0 {
			x.AddedBy = &in.UserID
		}
	}
	applyInfo(x, info)
	x.LibraryID, x.Monitor = &lib.ID, in.Monitor
	if err := m.st.SaveAcqSeries(x); err != nil {
		return nil, err
	}
	if err := m.refreshEpisodes(ctx, x, info); err != nil {
		return nil, err
	}
	if err := m.applyMonitor(x.ID, in.Monitor, in.Seasons); err != nil {
		return nil, err
	}
	m.syncHave(x)
	m.st.ResetEpisodeSearch(x.ID)
	m.log.Info("acquire: following series", "title", x.Title, "monitor", in.Monitor)
	m.Wake()
	return m.st.AcqSeriesByID(x.ID)
}

func applyInfo(x *store.AcqSeries, info meta.TVInfo) {
	x.ImdbID, x.TVDB, x.Title, x.OriginalTitle, x.EnglishTitle = info.IMDb, info.TVDB, info.Title, info.OriginalTitle, info.EnglishTitle
	x.Year, x.Poster, x.Ended = info.Year, info.Poster, info.Ended
}

// applyMonitor sets which episodes are followed.
func (m *Manager) applyMonitor(seriesID int64, mode string, seasons []int) error {
	switch mode {
	case MonitorAll:
		return m.st.SetSeasonMonitored(seriesID, -1, true)
	case MonitorNone:
		return m.st.SetSeasonMonitored(seriesID, -1, false)
	case MonitorSeasons:
		if err := m.st.SetSeasonMonitored(seriesID, -1, false); err != nil {
			return err
		}
		for _, s := range seasons {
			if err := m.st.SetSeasonMonitored(seriesID, s, true); err != nil {
				return err
			}
		}
		return nil
	case MonitorFuture:
		_, err := m.st.DB.Exec(`UPDATE acq_episodes SET monitored=CASE WHEN air_date='' OR air_date>? THEN 1 ELSE 0 END WHERE series_id=? AND season>0`, today(), seriesID)
		return err
	}
	return nil
}

// refreshEpisodes stores the series' episode list from TMDB. New episodes are followed when the series follows
// everything, only what is to come, or their season is already followed.
func (m *Manager) refreshEpisodes(ctx context.Context, x *store.AcqSeries, info meta.TVInfo) error {
	known, _ := m.st.Episodes(x.ID)
	followed := map[int]bool{}
	for _, e := range known {
		if e.Monitored {
			followed[e.Season] = true
		}
	}
	var rows []store.EpisodeRow
	for _, s := range info.Seasons {
		if s.Number <= 0 { // specials are not followed
			continue
		}
		eps, err := m.movies.TVSeasonEpisodes(ctx, x.TmdbID, s.Number)
		if err != nil {
			return fmt.Errorf("%d. évad: %w", s.Number, err)
		}
		for _, e := range eps {
			rows = append(rows, store.EpisodeRow{Season: e.Season, Episode: e.Episode, Title: e.Title, AirDate: e.AirDate})
		}
	}
	mode := x.Monitor
	newSeries := len(known) == 0
	err := m.st.UpsertEpisodes(x.ID, rows, func(season int, air string) bool {
		switch mode {
		case MonitorAll, MonitorFuture:
			return true // a new episode of a followed series is one to come
		case MonitorSeasons:
			return followed[season] || newSeries
		}
		return false
	})
	if err != nil {
		return err
	}
	x.RefreshedAt = time.Now().Unix()
	return m.st.SaveAcqSeries(x)
}

// syncHave marks the episodes that are in the library.
func (m *Manager) syncHave(x *store.AcqSeries) {
	pairs, err := m.st.LibraryEpisodes(x.TmdbID, []string{x.Title, x.OriginalTitle, x.EnglishTitle}, x.Year)
	if err == nil && len(pairs) > 0 {
		m.st.MarkEpisodesHave(x.ID, pairs)
	}
}

// SearchSeriesNow makes every missing episode due for a search on the next round.
func (m *Manager) SearchSeriesNow(id int64) error {
	if _, err := m.st.AcqSeriesByID(id); err != nil {
		return err
	}
	m.st.ResetEpisodeSearch(id)
	m.Wake()
	return nil
}

// SetSeriesMonitor changes what is followed of a series (and its library).
func (m *Manager) SetSeriesMonitor(ctx context.Context, id int64, mode string, seasons []int, libraryID int64) (*store.AcqSeries, error) {
	x, err := m.st.AcqSeriesByID(id)
	if err != nil {
		return nil, err
	}
	lib := int64(0)
	if libraryID > 0 {
		lib = libraryID
	} else if x.LibraryID != nil {
		lib = *x.LibraryID
	}
	return m.AddSeries(ctx, SeriesInput{TmdbID: x.TmdbID, LibraryID: lib, Monitor: mode, Seasons: seasons})
}

// ------------------------------------------------------------------ the round

// ProcessSeries refreshes, checks the library and searches the missing episodes of every followed series.
func (m *Manager) ProcessSeries(ctx context.Context) {
	if !m.Enabled() {
		return
	}
	m.seriesMu.Lock()
	defer m.seriesMu.Unlock()
	list, err := m.st.AcqSeriesList()
	if err != nil || len(list) == 0 {
		return
	}
	budget := searchesPerRound
	for _, x := range list {
		if ctx.Err() != nil {
			return
		}
		if time.Since(time.Unix(x.RefreshedAt, 0)) > SeriesRefreshEvery {
			if info, err := m.movies.TV(ctx, x.TmdbID); err == nil {
				applyInfo(x, info)
				if err := m.refreshEpisodes(ctx, x, info); err != nil {
					m.log.Warn("acquire: series refresh", "title", x.Title, "err", err)
				}
			}
		}
		m.syncHave(x)
		if x.Monitor != MonitorNone && budget > 0 {
			budget -= m.searchSeries(ctx, x, budget)
		}
	}
	if time.Since(m.lastRSS) > RSSEvery {
		m.lastRSS = time.Now()
		m.rss(ctx, list)
	}
	m.seriesRequestsDone(list)
}

func (x *seriesTitles) match(r Result) bool {
	return TitleMatches(r.Title, r.Release, 0, x.titles...)
}

type seriesTitles struct{ titles []string }

func titlesOf(x *store.AcqSeries) *seriesTitles {
	return &seriesTitles{titles: []string{x.Title, x.OriginalTitle, x.EnglishTitle}}
}

// rankTV scores results for a season (pack, episode < 0) or one episode; seasonCount is the season's episode count.
func (m *Manager) rankTV(rs []Result, x *store.AcqSeries, season, episode, seasonCount int) []Result {
	p := m.tvProfileFor(x)
	st := titlesOf(x)
	for i := range rs {
		r := &rs[i]
		rel := r.Release
		switch {
		case !st.match(*r) || rel.Season != season:
			r.Rejected = RejectTitle
			continue
		case episode < 0 && rel.Episode >= 0: // wanted a pack, this is an episode
			r.Rejected = RejectTitle
			continue
		case episode >= 0 && !containsInt(rel.Episodes, episode):
			r.Rejected = RejectTitle
			continue
		}
		n := len(rel.Episodes)
		if episode < 0 {
			n = seasonCount
		}
		if n < 1 {
			n = 1
		}
		r.Score, r.Rejected = p.Score(rel, r.Size/int64(n), r.Seeders)
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

func containsInt(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// searchTV asks every enabled indexer and returns the results together.
func (m *Manager) searchTV(ctx context.Context, x *store.AcqSeries, season, episode int) []Result {
	title := x.EnglishTitle
	if title == "" {
		title = x.OriginalTitle
	}
	q := Query{TV: true, TVDB: x.TVDB, IMDb: x.ImdbID, Title: title, Season: season, Episode: episode}
	var all []Result
	for _, ix := range m.indexers() {
		cctx, cancel := context.WithTimeout(ctx, 40*time.Second)
		res, err := m.searchIx(cctx, ix, q)
		cancel()
		if err != nil {
			m.st.MarkAcqIndexer(ix.ID, err.Error())
			continue
		}
		m.st.MarkAcqIndexer(ix.ID, "")
		all = append(all, res...)
	}
	return dedupe(all)
}

// searchSeries searches the due missing episodes of one series; returns the number of searches made.
func (m *Manager) searchSeries(ctx context.Context, x *store.AcqSeries, budget int) int {
	due := time.Now().Add(-m.retryWait()).Unix()
	wanted, err := m.st.WantedEpisodes(x.ID, due)
	if err != nil || len(wanted) == 0 {
		return 0
	}
	all, _ := m.st.Episodes(x.ID)
	bySeason := map[int][]int{}
	for _, e := range wanted {
		bySeason[e.Season] = append(bySeason[e.Season], e.Episode)
	}
	seasons := make([]int, 0, len(bySeason))
	for s := range bySeason {
		seasons = append(seasons, s)
	}
	sort.Ints(seasons)
	used := 0
	now := time.Now().Unix()
	for _, season := range seasons {
		eps := bySeason[season]
		count, aired := 0, true
		for _, e := range all {
			if e.Season == season {
				count++
				if e.AirDate == "" || e.AirDate > today() {
					aired = false
				}
			}
		}
		// a fully aired season with several episodes missing: one pack is kinder to everyone
		if aired && len(eps) >= 2 && used < budget {
			used++
			rs := m.rankTV(m.searchTV(ctx, x, season, -1), x, season, -1, count)
			if rel := m.bestRelease(rs); rel != nil {
				if err := m.grabSeries(ctx, x, season, nil, rel, 0); err == nil {
					m.st.MarkEpisodesSearched(x.ID, pairs(season, eps), now)
					continue
				} else {
					m.log.Warn("acquire: season grab", "title", x.Title, "season", season, "err", err)
				}
			}
		}
		for _, ep := range eps {
			if used >= budget {
				break
			}
			used++
			rs := m.rankTV(m.searchTV(ctx, x, season, ep), x, season, ep, count)
			m.st.MarkEpisodesSearched(x.ID, pairs(season, []int{ep}), now)
			if rel := m.bestRelease(rs); rel != nil {
				if err := m.grabSeries(ctx, x, season, intersect(rel.Release.Episodes, eps, ep), rel, 0); err != nil {
					m.log.Warn("acquire: episode grab", "title", x.Title, "season", season, "episode", ep, "err", err)
				}
			}
		}
	}
	return used
}

// intersect keeps the episodes of a (multi-episode) release that are wanted; at least ep itself.
func intersect(have, wanted []int, ep int) []int {
	out := []int{}
	for _, e := range have {
		if containsInt(wanted, e) {
			out = append(out, e)
		}
	}
	if !containsInt(out, ep) {
		out = append(out, ep)
	}
	sort.Ints(out)
	return out
}

func pairs(season int, eps []int) [][2]int {
	out := make([][2]int, len(eps))
	for i, e := range eps {
		out[i] = [2]int{season, e}
	}
	return out
}

func joinInts(v []int) string {
	s := make([]string, len(v))
	for i, x := range v {
		s[i] = strconv.Itoa(x)
	}
	return strings.Join(s, ",")
}

func splitInts(s string) []int {
	var out []int
	for _, p := range strings.Split(s, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// grabSeries downloads a season pack (eps nil) or some episodes of a season.
func (m *Manager) grabSeries(ctx context.Context, x *store.AcqSeries, season int, eps []int, rel *Result, requestID int64) error {
	if x.LibraryID == nil {
		return errors.New("a sorozatnak nincs könyvtára")
	}
	cfg, err := m.firstClient(rel.Protocol)
	if err != nil {
		return err
	}
	cl, cc, err := m.client(cfg)
	if err != nil {
		return err
	}
	f, err := m.fetch(ctx, *rel)
	if err != nil {
		return err
	}
	if m.st.Blocklisted([]string{f.InfoHash})[f.InfoHash] {
		return errors.New("a kiadás feketelistán van")
	}
	all, _ := m.st.Episodes(x.ID)
	cover := eps
	if eps == nil { // a pack: every wanted episode of the season
		for _, e := range all {
			if e.Season == season && e.State == "wanted" {
				cover = append(cover, e.Episode)
			}
		}
	}
	if g, err := m.st.ActiveGrabForHash(f.InfoHash); err == nil {
		m.st.SetEpisodesGrab(x.ID, pairs(season, cover), g.ID)
		return nil
	}
	// series have their own category / folder in the client; empty: the films' one
	dlID, err := cl.Add(ctx, AddRequest{Fetched: f, Tag: "kinora", Category: cc.CategoryTV, SavePath: cc.SavePathTV})
	if err != nil {
		return err
	}
	kind := "episode"
	if eps == nil {
		kind = "season"
	}
	g := &store.AcqGrab{Kind: kind, TmdbID: x.TmdbID, ImdbID: x.ImdbID, Title: x.Title, Year: x.Year, LibraryID: x.LibraryID,
		ReleaseTitle: rel.Title, IndexerID: &rel.IndexerID, IndexerName: rel.Indexer, ClientID: &cfg.ID, InfoHash: f.InfoHash, DownloadID: dlID,
		Size: rel.Size, Quality: rel.Quality, Score: rel.Score, SeriesID: &x.ID, Season: season, Episodes: joinInts(eps)}
	if requestID > 0 {
		g.RequestID = &requestID
	}
	if err := m.st.AddAcqGrab(g); err != nil {
		return err
	}
	m.st.SetEpisodesGrab(x.ID, pairs(season, cover), g.ID)
	m.log.Info("acquire: series download started", "title", x.Title, "season", season, "episodes", joinInts(cover), "release", rel.Title)
	m.Wake()
	return nil
}

// rss looks through the indexers' newest series releases for wanted episodes of followed series.
func (m *Manager) rss(ctx context.Context, list []*store.AcqSeries) {
	var latest []Result
	for _, ix := range m.indexers() {
		cctx, cancel := context.WithTimeout(ctx, 40*time.Second)
		res, err := m.latestIx(cctx, ix, true)
		cancel()
		if err != nil {
			continue
		}
		latest = append(latest, res...)
	}
	if len(latest) == 0 {
		return
	}
	for _, x := range list {
		if x.Monitor == MonitorNone {
			continue
		}
		wanted, err := m.st.WantedEpisodes(x.ID, time.Now().Unix()+1) // also the ones searched lately
		if err != nil || len(wanted) == 0 {
			continue
		}
		st := titlesOf(x)
		for _, e := range wanted {
			var cands []Result
			for _, r := range latest {
				if r.Release.Season == e.Season && containsInt(r.Release.Episodes, e.Episode) && st.match(r) {
					cands = append(cands, r)
				}
			}
			if len(cands) == 0 {
				continue
			}
			rs := m.rankTV(cands, x, e.Season, e.Episode, 1)
			if rel := m.bestRelease(rs); rel != nil {
				if err := m.grabSeries(ctx, x, e.Season, []int{e.Episode}, rel, 0); err == nil {
					break // the list of wanted episodes changed: the next round goes on
				}
			}
		}
	}
}

// ------------------------------------------------------------------ import

func (m *Manager) importSeriesGrab(g *store.AcqGrab, cc ClientConfig, s TorrentStatus) {
	if g.SeriesID == nil || g.LibraryID == nil {
		m.grabFailed(g, "a sorozat vagy a könyvtára törölve lett", false)
		return
	}
	x, err := m.st.AcqSeriesByID(*g.SeriesID)
	if err != nil {
		m.grabFailed(g, "a sorozat már nincs figyelve", false)
		return
	}
	lib, err := m.st.Library(*g.LibraryID)
	if err != nil {
		m.grabFailed(g, "a célkönyvtár törölve lett", false)
		return
	}
	want := splitInts(g.Episodes)
	if len(want) == 0 { // a pack: every episode of the season Kinora does not have yet
		eps, _ := m.st.Episodes(x.ID)
		for _, e := range eps {
			if e.Season == g.Season && !e.Have {
				want = append(want, e.Episode)
			}
		}
	}
	m.st.SetAcqGrabStatus(g.ID, store.GrabImporting, "", "")
	src := MapPath(s.ContentPath, cc.PathRemote, cc.PathLocal)
	if src == "" {
		m.grabFailed(g, errOutsideMapping, false)
		return
	}
	res, err := ImportEpisodes(ImportShow{Source: src, LibRoot: lib.Path, Title: x.Title, Year: x.Year, Quality: g.Quality,
		Mode: m.st.Setting("acq_import_mode"), Season: g.Season, Want: want})
	if err != nil {
		m.grabFailed(g, err.Error(), false)
		m.log.Warn("acquire: episode import failed", "title", x.Title, "src", src, "err", err)
		return
	}
	got := make([]int, 0, len(res.Episodes))
	for ep := range res.Episodes {
		got = append(got, ep)
	}
	sort.Ints(got)
	m.st.MarkEpisodesHave(x.ID, pairs(g.Season, got))
	m.st.SetAcqGrabStatus(g.ID, store.GrabDone, "", res.Folder)
	m.log.Info("acquire: episodes imported", "title", x.Title, "season", g.Season, "episodes", joinInts(got))
	if m.scan != nil {
		m.scan(lib.ID)
	}
}

// ------------------------------------------------------------------ requests for series

// seriesRequestsDone closes the open series requests whose followed episodes have all arrived.
func (m *Manager) seriesRequestsDone(list []*store.AcqSeries) {
	open, err := m.st.AcqRequests(0, 500, store.ReqDownloading)
	if err != nil {
		return
	}
	byTmdb := map[int64]*store.AcqSeries{}
	for _, x := range list {
		byTmdb[x.TmdbID] = x
	}
	for _, q := range open {
		if q.Kind != "series" {
			continue
		}
		x := byTmdb[q.TmdbID]
		if x == nil {
			continue
		}
		if fresh, err := m.st.AcqSeriesByID(x.ID); err == nil && fresh.Wanted == 0 && fresh.Downloading == 0 && fresh.Have > 0 {
			m.st.MarkFilmAvailable("series", q.TmdbID)
		}
	}
}

// requestMonitor turns a request's seasons ("all", "future", "1,2") into a monitor mode.
func requestMonitor(seasons string) (string, []int) {
	switch seasons {
	case "", MonitorAll:
		return MonitorAll, nil
	case MonitorFuture:
		return MonitorFuture, nil
	}
	return MonitorSeasons, splitInts(seasons)
}

// mergeMonitor keeps what a series already follows when a request adds to it.
func (m *Manager) followForRequest(ctx context.Context, q *store.AcqRequest) error {
	mode, seasons := requestMonitor(q.Seasons)
	uid := int64(0)
	if q.UserID != nil {
		uid = *q.UserID
	}
	if x, err := m.st.AcqSeriesByTmdb(q.TmdbID); err == nil && x.Monitor != MonitorNone {
		switch {
		case x.Monitor == MonitorAll || mode == MonitorAll:
			mode, seasons = MonitorAll, nil
		case mode == MonitorSeasons:
			eps, _ := m.st.Episodes(x.ID)
			have := map[int]bool{}
			for _, s := range seasons {
				have[s] = true
			}
			for _, e := range eps {
				if e.Monitored && !have[e.Season] {
					have[e.Season] = true
					seasons = append(seasons, e.Season)
				}
			}
			if x.Monitor == MonitorFuture {
				// keep following what is to come as well: the new seasons arrive monitored
				mode = MonitorSeasons
			}
		}
	}
	// a series already followed stays in its library; otherwise the one the requester chose
	lib := idOf(q.LibraryID)
	if x, err := m.st.AcqSeriesByTmdb(q.TmdbID); err == nil && x.LibraryID != nil {
		lib = *x.LibraryID
	}
	_, err := m.AddSeries(ctx, SeriesInput{TmdbID: q.TmdbID, LibraryID: lib, Monitor: mode, Seasons: seasons, UserID: uid})
	return err
}

// ValidSeasons checks a request's seasons: "all", "future" or a list of season numbers.
func ValidSeasons(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == MonitorAll || s == MonitorFuture {
		if s == "" {
			s = MonitorAll
		}
		return s, nil
	}
	list := splitInts(s)
	if len(list) == 0 {
		return "", errors.New("hibás évadlista")
	}
	for _, n := range list {
		if n < 1 || n > 200 {
			return "", errors.New("hibás évadszám")
		}
	}
	sort.Ints(list)
	return joinInts(list), nil
}

// ------------------------------------------------------------------ interactive search

// SeriesReleases searches a season (episode < 0: season packs and its single episodes) or one episode for the
// interactive search. Every result is listed; the ones that do not fit carry the reason.
func (m *Manager) SeriesReleases(ctx context.Context, id int64, season, episode int) (*store.AcqSeries, []Result, error) {
	x, err := m.st.AcqSeriesByID(id)
	if err != nil {
		return nil, nil, err
	}
	if len(m.indexers()) == 0 {
		return nil, nil, ErrNoIndex
	}
	count := 0
	if eps, err := m.st.Episodes(x.ID); err == nil {
		for _, e := range eps {
			if e.Season == season {
				count++
			}
		}
	}
	rs := m.searchTV(ctx, x, season, episode)
	p := m.tvProfileFor(x)
	st := titlesOf(x)
	var hashes []string
	for _, r := range rs {
		hashes = append(hashes, r.InfoHash)
	}
	bad := m.st.Blocklisted(hashes)
	for i := range rs {
		r := &rs[i]
		rel := r.Release
		switch {
		case !st.match(*r) || rel.Season != season:
			r.Rejected = RejectTitle
			continue
		case episode >= 0 && !containsInt(rel.Episodes, episode):
			r.Rejected = RejectTitle
			continue
		}
		n := len(rel.Episodes)
		if n == 0 {
			n = count
		}
		if n < 1 {
			n = 1
		}
		r.Score, r.Rejected = p.Score(rel, r.Size/int64(n), r.Seeders)
		if r.Rejected == "" && bad[r.InfoHash] {
			r.Rejected = RejectBlocklisted
		}
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
	return x, rs, nil
}

// GrabSeriesRelease downloads a release chosen in the interactive search: a season pack, or the episodes in its name.
func (m *Manager) GrabSeriesRelease(ctx context.Context, id int64, season int, rel Result) error {
	if !m.Enabled() {
		return ErrDisabled
	}
	x, err := m.st.AcqSeriesByID(id)
	if err != nil {
		return err
	}
	if _, err := m.st.AcqIndexer(rel.IndexerID); err != nil {
		return errors.New("ismeretlen indexer")
	}
	r := ParseRelease(rel.Title) // scored again here: the browser only says which release it wants
	rel.Release, rel.Quality = r, r.QualityLabel()
	rel.Score, _ = m.tvProfileFor(x).Score(r, rel.Size, rel.Seeders)
	var eps []int
	if len(r.Episodes) > 0 {
		eps = r.Episodes
	}
	return m.grabSeries(ctx, x, season, eps, &rel, 0)
}

// SearchSeason makes the missing episodes of one season due on the next round (the automatic search).
func (m *Manager) SearchSeason(id int64, season int) error {
	if _, err := m.st.AcqSeriesByID(id); err != nil {
		return err
	}
	m.st.DB.Exec(`UPDATE acq_episodes SET searched_at=0 WHERE series_id=? AND (?<0 OR season=?)`, id, season, season)
	m.Wake()
	return nil
}
