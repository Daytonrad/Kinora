package arrimport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ScotyLucas/kinora/internal/acquire"
	"github.com/ScotyLucas/kinora/internal/meta"
	"github.com/ScotyLucas/kinora/internal/store"
)

// ------------------------------------------------------------------ fake *arr

// fixtures shaped like the v3 / v1 API answers (secrets masked the way the apps mask them)
const (
	radarrStatus   = `{"appName":"Radarr","instanceName":"Radarr","version":"5.14.0.9383"}`
	sonarrStatus   = `{"appName":"Sonarr","instanceName":"Sonarr","version":"4.0.11.2680"}`
	prowlarrStatus = `{"appName":"Prowlarr","instanceName":"Prowlarr","version":"1.28.2.4885"}`

	profiles = `[{"id":4,"name":"HD-1080p","upgradeAllowed":true,"cutoff":7,"items":[
	  {"quality":{"id":1,"name":"SDTV","source":"tv","resolution":480},"items":[],"allowed":false},
	  {"quality":{"id":4,"name":"HDTV-720p","source":"tv","resolution":720},"items":[],"allowed":true},
	  {"name":"WEB 1080p","id":1003,"allowed":true,"items":[
	    {"quality":{"id":3,"name":"WEBDL-1080p","source":"webdl","resolution":1080},"items":[],"allowed":true},
	    {"quality":{"id":15,"name":"WEBRip-1080p","source":"webrip","resolution":1080},"items":[],"allowed":true}]},
	  {"quality":{"id":7,"name":"Bluray-1080p","source":"bluray","resolution":1080},"items":[],"allowed":true},
	  {"quality":{"id":24,"name":"CAM","source":"cam","resolution":0},"items":[],"allowed":false},
	  {"quality":{"id":30,"name":"Remux-2160p","source":"bluray","resolution":2160},"items":[],"allowed":false}]}]`

	indexers = `[
	  {"id":1,"name":"Jackett: tracker","implementation":"Torznab","protocol":"torrent","priority":10,"enableRss":true,"enableAutomaticSearch":true,
	   "fields":[{"name":"baseUrl","value":"http://jackett:9117/api/v2.0/indexers/tracker/results/torznab"},{"name":"apiPath","value":"/api"},
	             {"name":"apiKey","value":"********"},{"name":"categories","value":[2000,2040]}]},
	  {"id":2,"name":"NZB site","implementation":"Newznab","protocol":"usenet","priority":25,"enableRss":true,"enableAutomaticSearch":true,
	   "fields":[{"name":"baseUrl","value":"https://nzb.example"},{"name":"apiPath","value":"/api"},{"name":"apiKey","value":"plainkey"},{"name":"categories","value":[5030]}]},
	  {"id":3,"name":"BroadcastheNet","implementation":"BroadcastheNet","protocol":"torrent","enableRss":true,"fields":[]}]`

	clients = `[
	  {"id":1,"name":"qBittorrent","implementation":"QBittorrent","protocol":"torrent","enable":true,
	   "fields":[{"name":"host","value":"qbit"},{"name":"port","value":8080},{"name":"useSsl","value":false},{"name":"urlBase","value":""},
	             {"name":"username","value":"admin"},{"name":"password","value":"********"},{"name":"movieCategory","value":"radarr"},{"name":"tvCategory","value":"tv-sonarr"}]},
	  {"id":2,"name":"SAB","implementation":"Sabnzbd","protocol":"usenet","enable":true,
	   "fields":[{"name":"host","value":"sab"},{"name":"port","value":8085},{"name":"useSsl","value":true},{"name":"urlBase","value":"/sabnzbd"},
	             {"name":"apiKey","value":"sabkey"},{"name":"movieCategory","value":"movies"},{"name":"tvCategory","value":"tv"}]},
	  {"id":3,"name":"Blackhole","implementation":"TorrentBlackhole","protocol":"torrent","enable":true,"fields":[]}]`

	movies = `[
	  {"id":1,"title":"Dune: Part Two","year":2024,"tmdbId":693134,"imdbId":"tt15239678","monitored":true,"hasFile":false,"qualityProfileId":4,"path":"/movies/Dune Part Two (2024)","rootFolderPath":"/movies"},
	  {"id":2,"title":"Arrival","year":2016,"tmdbId":329865,"monitored":true,"hasFile":true,"qualityProfileId":4,"path":"/movies/Arrival (2016)"},
	  {"id":3,"title":"Old Film","year":1950,"tmdbId":111,"monitored":false,"hasFile":false,"qualityProfileId":4,"path":"/movies/Old Film (1950)"},
	  {"id":4,"title":"In Kinora","year":2010,"tmdbId":27205,"monitored":true,"hasFile":false,"qualityProfileId":4,"path":"/movies/In Kinora (2010)"}]`

	series = `[
	  {"id":1,"title":"Severance","year":2022,"tvdbId":371980,"tmdbId":95396,"imdbId":"tt11280740","monitored":true,"qualityProfileId":4,"path":"/tv/Severance","rootFolderPath":"/tv",
	   "seasons":[{"seasonNumber":0,"monitored":false},{"seasonNumber":1,"monitored":true},{"seasonNumber":2,"monitored":true}]},
	  {"id":2,"title":"Old Show","year":2001,"tvdbId":7001,"tmdbId":0,"monitored":true,"qualityProfileId":4,"path":"/tv/Old Show","rootFolderPath":"/tv",
	   "seasons":[{"seasonNumber":1,"monitored":false},{"seasonNumber":2,"monitored":true}]},
	  {"id":3,"title":"Dropped","year":2015,"tvdbId":7002,"tmdbId":5000,"monitored":false,"qualityProfileId":4,"path":"/tv/Dropped","seasons":[]}]`

	prowlarrIndexers = `[
	  {"id":7,"name":"1337x","protocol":"torrent","enable":true,"priority":25,"definitionName":"1337x","fields":[]},
	  {"id":8,"name":"NZBgeek","protocol":"usenet","enable":false,"priority":20,"definitionName":"nzbgeek","fields":[]}]`
)

func fakeArr(t *testing.T, app, key string, rootFolders string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != key {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		api := "/api/v3"
		if app == Prowlarr {
			api = "/api/v1"
		}
		body := map[string]string{
			"/system/status":  map[string]string{Radarr: radarrStatus, Sonarr: sonarrStatus, Prowlarr: prowlarrStatus}[app],
			"/qualityprofile": profiles, "/downloadclient": clients, "/rootfolder": rootFolders,
			"/movie": movies, "/series": series,
		}
		if app == Prowlarr {
			body["/indexer"] = prowlarrIndexers
		} else {
			body["/indexer"] = indexers
		}
		b, ok := body[strings.TrimPrefix(r.URL.Path, api)]
		if !ok || !strings.HasPrefix(r.URL.Path, api) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(b))
	}))
}

// ------------------------------------------------------------------ fake TMDB

type fakeTMDB struct{ finds int }

func (f *fakeTMDB) Enabled() bool { return true }
func (f *fakeTMDB) FindByExternal(_ context.Context, kind, source, id string) (int64, error) {
	f.finds++
	if kind == "tv" && source == "tvdb_id" && id == "7001" {
		return 4242, nil
	}
	return 0, nil
}
func (f *fakeTMDB) Search(context.Context, string, string, int) ([]meta.SearchResult, error) {
	return nil, nil
}
func (f *fakeTMDB) Movie(_ context.Context, id int64) (meta.MovieInfo, error) {
	return meta.MovieInfo{ID: id, Title: "Film " + jsonInt(id), Year: 2024}, nil
}
func (f *fakeTMDB) MovieList(context.Context, string, int) ([]meta.SearchResult, error) {
	return nil, nil
}
func (f *fakeTMDB) TV(_ context.Context, id int64) (meta.TVInfo, error) {
	return meta.TVInfo{ID: id, Title: "Series " + jsonInt(id), Year: 2022, Seasons: []meta.TVSeason{{Number: 1, EpisodeCount: 2}, {Number: 2, EpisodeCount: 2}}}, nil
}
func (f *fakeTMDB) TVSeasonEpisodes(_ context.Context, id int64, season int) ([]meta.TVEpisode, error) {
	return []meta.TVEpisode{{Season: season, Episode: 1, AirDate: "2020-01-01"}, {Season: season, Episode: 2, AirDate: "2020-01-08"}}, nil
}
func (f *fakeTMDB) TVList(context.Context, string, int) ([]meta.SearchResult, error) { return nil, nil }
func (f *fakeTMDB) Browse(context.Context, meta.BrowseFilter) (meta.BrowsePage, error) {
	return meta.BrowsePage{}, nil
}
func (f *fakeTMDB) WatchProviders(context.Context, string) ([]meta.WatchProvider, error) {
	return nil, nil
}
func (f *fakeTMDB) Genres(context.Context, string) ([]meta.Genre, error) { return nil, nil }
func (f *fakeTMDB) TitleProviders(context.Context, string, int64) (meta.WhereToWatch, error) {
	return meta.WhereToWatch{}, nil
}
func (f *fakeTMDB) WatchRegion() string { return "HU" }

func jsonInt(n int64) string { b, _ := json.Marshal(n); return string(b) }

// ------------------------------------------------------------------ setup

type env struct {
	st      *store.Store
	deps    Deps
	tmdb    *fakeTMDB
	root    string // the folder Kinora sees as the app's root folders
	scanned []int64
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "k.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if _, err := st.CreateFirstAdmin("admin", "secret123"); err != nil {
		t.Fatal(err)
	}
	e := &env{st: st, tmdb: &fakeTMDB{}, root: filepath.Join(dir, "media")}
	for _, d := range []string{"movies", "tv"} {
		os.MkdirAll(filepath.Join(e.root, d), 0o755)
	}
	// Kinora already has a films library on the Radarr root folder, and one film Radarr still wants
	lib, _ := st.CreateLibrary("Filmek", filepath.Join(e.root, "movies"), "movies")
	st.DB.Exec(`INSERT INTO items(library_id,kind,key,title,sort_title,tmdb_id,added_at,updated_at) VALUES(?,'movie','x','In Kinora','In Kinora',27205,1,1)`, lib.ID)
	st.SetSetting("acq_enabled", "1")
	m := acquire.New(st, e.tmdb, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	e.deps = Deps{Store: st, Acquire: m, TMDB: e.tmdb, Scan: func(id int64) { e.scanned = append(e.scanned, id) }}
	return e
}

func (e *env) maps() []PathMap {
	return []PathMap{{From: "/movies", To: filepath.Join(e.root, "movies")}, {From: "/tv", To: filepath.Join(e.root, "tv")}}
}

func run(t *testing.T, p *Plan, d Deps, sections ...string) ProgressView {
	t.Helper()
	pr := NewProgress(p, sections)
	Apply(context.Background(), p, d, sections, pr)
	v := pr.Snapshot()
	if v.Running || v.Done != v.Total {
		t.Fatalf("progress: %+v", v)
	}
	return v
}

// ------------------------------------------------------------------ tests

func TestRadarrImport(t *testing.T) {
	e := newEnv(t)
	srv := fakeArr(t, Radarr, "rk", `[{"id":1,"path":"/movies","accessible":true},{"id":2,"path":"/elsewhere","accessible":true}]`)
	defer srv.Close()
	c, _ := NewClient(Radarr, srv.URL, "rk")
	p, err := Preview(context.Background(), c, e.deps, e.maps())
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := e.st.AcqIndexers(); len(n) != 0 {
		t.Fatal("preview wrote indexers")
	}

	// profile: 1080p and 720p allowed (a group counts), 2160p not; CAM blocked; cutoff Bluray-1080p
	pr := p.Profiles[0].Profile
	if got := pr.Resolutions; len(got) != 2 || got[0] != 1080 || got[1] != 720 {
		t.Fatalf("resolutions %v", got)
	}
	if !pr.Upgrade || pr.CutoffRes != 1080 || pr.CutoffSource != "bluray" {
		t.Fatalf("cutoff: %+v", pr)
	}
	if strings.Join(pr.BlockedSources, ",") != "cam,ts,tc,scr" {
		t.Fatalf("blocked %v", pr.BlockedSources)
	}

	byName := func(list []*IndexerPlan, n string) *IndexerPlan {
		for _, x := range list {
			if x.Name == n {
				return x
			}
		}
		t.Fatalf("%s missing", n)
		return nil
	}
	jk := byName(p.Indexers, "Jackett: tracker")
	if jk.Kind != "torznab" || jk.URL != "http://jackett:9117/api/v2.0/indexers/tracker/results/torznab/api" || jk.Categories != "2000,2040" || len(jk.Notes) != 1 {
		t.Fatalf("torznab indexer: %+v", jk)
	}
	if byName(p.Indexers, "NZB site").Kind != "newznab" || byName(p.Indexers, "BroadcastheNet").Reason != RsUnsupported {
		t.Fatal("indexer kinds")
	}
	if p.Clients[0].URL != "http://qbit:8080" || p.Clients[0].Category != "radarr" || len(p.Clients[0].Notes) != 1 {
		t.Fatalf("qbittorrent: %+v", p.Clients[0])
	}
	if p.Clients[1].URL != "https://sab:8085/sabnzbd" || p.Clients[1].password != "sabkey" || p.Clients[2].Action != ActSkip {
		t.Fatalf("clients: %+v %+v", p.Clients[1], p.Clients[2])
	}
	if p.Folders[0].Action != ActLink || p.Folders[1].Action != ActSkip || p.Folders[1].Reason != RsUnreachable {
		t.Fatalf("folders: %+v %+v", p.Folders[0], p.Folders[1])
	}
	acts := map[string]string{}
	for _, m := range p.Movies {
		acts[m.Title] = m.Action + "/" + m.Reason
	}
	want := map[string]string{"Dune: Part Two": "request/", "Arrival": "skip/has_file", "Old Film": "skip/unmonitored", "In Kinora": "skip/in_library"}
	for k, v := range want {
		if acts[k] != v {
			t.Errorf("%s: %s, want %s", k, acts[k], v)
		}
	}

	v := run(t, p, e.deps)
	if len(v.Errors) != 0 {
		t.Fatalf("errors: %+v", v.Errors)
	}
	ixs, _ := e.st.AcqIndexers()
	if len(ixs) != 2 {
		t.Fatalf("indexers created: %d", len(ixs))
	}
	for _, x := range ixs {
		if x.Name == "Jackett: tracker" && (x.Enabled || x.APIKey != "") {
			t.Fatal("an indexer whose key the app hides must stay off")
		}
		if x.Name == "NZB site" && (!x.Enabled || x.APIKey != "plainkey") {
			t.Fatalf("newznab: %+v", x)
		}
	}
	cls, _ := e.st.AcqClients()
	if len(cls) != 2 {
		t.Fatalf("clients: %d", len(cls))
	}
	q, err := e.st.OpenRequestFor("movie", 693134)
	if err != nil || q.Status != store.ReqApproved {
		t.Fatalf("wanted film: %+v %v", q, err)
	}

	// again: nothing new, updated in place; a key the admin entered in Kinora survives
	for _, x := range ixs {
		if x.Name == "Jackett: tracker" {
			x.APIKey, x.Enabled = "typed-in", true
			e.st.SaveAcqIndexer(x)
		}
	}
	p2, err := Preview(context.Background(), c, e.deps, e.maps())
	if err != nil {
		t.Fatal(err)
	}
	if byName(p2.Indexers, "Jackett: tracker").Action != ActUpdate || p2.Profiles[0].Action != ActUpdate || p2.Movies[0].Action != ActExisting {
		t.Fatalf("second preview: %s %s %s", byName(p2.Indexers, "Jackett: tracker").Action, p2.Profiles[0].Action, p2.Movies[0].Action)
	}
	run(t, p2, e.deps)
	ixs, _ = e.st.AcqIndexers()
	cls, _ = e.st.AcqClients()
	ps, _ := e.st.AcqProfiles()
	if len(ixs) != 2 || len(cls) != 2 || len(ps) != 1 {
		t.Fatalf("duplicates after a second run: %d indexers, %d clients, %d profiles", len(ixs), len(cls), len(ps))
	}
	for _, x := range ixs {
		if x.Name == "Jackett: tracker" && (x.APIKey != "typed-in" || !x.Enabled) {
			t.Fatalf("the typed-in key was lost: %+v", x)
		}
	}
}

func TestSonarrImport(t *testing.T) {
	e := newEnv(t)
	srv := fakeArr(t, Sonarr, "sk", `[{"id":1,"path":"/tv","accessible":true}]`)
	defer srv.Close()
	c, _ := NewClient(Sonarr, srv.URL, "sk")
	p, err := Preview(context.Background(), c, e.deps, e.maps())
	if err != nil {
		t.Fatal(err)
	}
	if p.Folders[0].Action != ActCreate {
		t.Fatalf("the series folder has no library yet: %+v", p.Folders[0])
	}
	sev, old, dropped := p.Series[0], p.Series[1], p.Series[2]
	if sev.Monitor != acquire.MonitorAll || sev.Action != ActFollow {
		t.Fatalf("severance (specials do not count): %+v", sev)
	}
	if old.TmdbID != 4242 || old.Monitor != acquire.MonitorSeasons || len(old.Seasons) != 1 || old.Seasons[0] != 2 {
		t.Fatalf("old show (found by TVDB id, one season): %+v", old)
	}
	if dropped.Action != ActSkip || dropped.Reason != RsUnmonitored {
		t.Fatalf("unmonitored: %+v", dropped)
	}

	// Radarr made the qBittorrent client already: Sonarr's import fills in the series category only
	e.st.SaveAcqClient(&store.AcqClient{Name: "qBittorrent", Kind: "qbittorrent", URL: "http://qbit:8080", Category: "radarr", Password: "pw", Enabled: true})
	p, _ = Preview(context.Background(), c, e.deps, e.maps())
	if p.Clients[0].Action != ActUpdate {
		t.Fatalf("shared client: %+v", p.Clients[0])
	}

	v := run(t, p, e.deps)
	if len(v.Errors) != 0 {
		t.Fatalf("errors: %+v", v.Errors)
	}
	if len(e.scanned) != 1 {
		t.Fatal("the new library must be scanned")
	}
	cls, _ := e.st.AcqClients()
	var qb *store.AcqClient
	for _, c := range cls {
		if c.Kind == "qbittorrent" {
			qb = c
		}
	}
	if len(cls) != 2 || qb.Category != "radarr" || qb.CategoryTV != "tv-sonarr" || qb.Password != "pw" || !qb.Enabled {
		t.Fatalf("shared client after Sonarr: %d clients, %+v", len(cls), qb)
	}
	list, _ := e.st.AcqSeriesList()
	if len(list) != 2 {
		t.Fatalf("followed series: %d", len(list))
	}
	for _, x := range list {
		var prof *int64
		e.st.DB.QueryRow(`SELECT profile_id FROM acq_series WHERE id=?`, x.ID).Scan(&prof)
		if x.TmdbID == 95396 && (x.Monitor != acquire.MonitorAll || prof == nil) {
			t.Fatalf("severance: %+v (profile %v)", x, prof)
		}
	}
	eps, _ := e.st.Episodes(lookup(t, list, 4242).ID)
	for _, ep := range eps {
		if ep.Monitored != (ep.Season == 2) {
			t.Fatalf("old show: season %d monitored %v", ep.Season, ep.Monitored)
		}
	}
}

func lookup(t *testing.T, list []*store.AcqSeries, tmdb int64) *store.AcqSeries {
	for _, x := range list {
		if x.TmdbID == tmdb {
			return x
		}
	}
	t.Fatalf("series %d not followed", tmdb)
	return nil
}

func TestProwlarrImport(t *testing.T) {
	e := newEnv(t)
	srv := fakeArr(t, Prowlarr, "pk", `[]`)
	defer srv.Close()
	c, _ := NewClient(Prowlarr, srv.URL+"/", "pk")
	p, err := Preview(context.Background(), c, e.deps, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Indexers) != 2 || p.Indexers[0].URL != srv.URL+"/7/api" || p.Indexers[0].Kind != "torznab" || p.Indexers[1].Kind != "newznab" {
		t.Fatalf("prowlarr indexers: %+v %+v", p.Indexers[0], p.Indexers[1])
	}
	run(t, p, e.deps)
	ixs, _ := e.st.AcqIndexers()
	if len(ixs) != 2 || ixs[0].APIKey != "pk" {
		t.Fatalf("indexers: %+v", ixs)
	}
	for _, x := range ixs {
		if x.Name == "NZBgeek" && x.Enabled {
			t.Fatal("an indexer disabled in Prowlarr stays off")
		}
	}
}

func TestBlockersWithoutAcquisition(t *testing.T) {
	e := newEnv(t)
	e.st.SetSetting("acq_enabled", "0")
	srv := fakeArr(t, Radarr, "rk", `[]`)
	defer srv.Close()
	c, _ := NewClient(Radarr, srv.URL, "rk")
	p, err := Preview(context.Background(), c, e.deps, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Blockers) != 1 || p.Blockers[0] != RsAcqOff {
		t.Fatalf("blockers: %v", p.Blockers)
	}
	v := run(t, p, e.deps, SecTitles)
	if len(v.Errors) != 1 || v.Errors[0].Error != RsAcqOff {
		t.Fatalf("errors: %+v", v.Errors)
	}
}

func TestClientErrors(t *testing.T) {
	if _, err := NewClient("lidarr", "http://x", "k"); !errors.Is(err, ErrWrongApp) {
		t.Fatal(err)
	}
	if _, err := NewClient(Sonarr, "x", "k"); !errors.Is(err, ErrBadURL) {
		t.Fatal(err)
	}
	if _, err := NewClient(Sonarr, "http://x", ""); !errors.Is(err, ErrNoKey) {
		t.Fatal(err)
	}
	e := newEnv(t)
	srv := fakeArr(t, Radarr, "rk", `[]`)
	defer srv.Close()
	c, _ := NewClient(Sonarr, srv.URL, "rk") // Radarr answers where Sonarr was expected
	if _, err := Preview(context.Background(), c, e.deps, nil); !errors.Is(err, ErrWrongApp) {
		t.Fatalf("wrong app: %v", err)
	}
	c, _ = NewClient(Radarr, srv.URL, "bad")
	if _, err := Preview(context.Background(), c, e.deps, nil); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("bad key: %v", err)
	}
}

func TestMapPath(t *testing.T) {
	maps := []PathMap{{From: "/movies/", To: "/media/films"}, {From: `D:\TV`, To: "/media/tv"}}
	for in, want := range map[string]string{
		"/movies":       filepath.FromSlash("/media/films"),
		"/movies/A (1)": filepath.FromSlash("/media/films/A (1)"),
		"/moviesX":      "/moviesX",
		`D:\TV\Show`:    filepath.FromSlash("/media/tv/Show"),
	} {
		if got := mapPath(in, maps); got != want {
			t.Errorf("mapPath(%q) = %q, want %q", in, got, want)
		}
	}
}
