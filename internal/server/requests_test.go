package server

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// fakeTMDB answers the few TMDB calls the discover page makes.
func fakeTMDB() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/movie/603":
			fmt.Fprint(w, `{"id":603,"imdb_id":"tt0133093","title":"Mátrix","original_title":"The Matrix","release_date":"1999-03-31","overview":"Neo.","runtime":136,"genres":[]}`)
		case r.URL.Path == "/tv/1399":
			fmt.Fprint(w, `{"id":1399,"name":"Trónok harca","original_name":"Game of Thrones","first_air_date":"2011-04-17","status":"Ended","external_ids":{"imdb_id":"tt0944947","tvdb_id":121361},"seasons":[{"season_number":0,"episode_count":1},{"season_number":1,"episode_count":2}]}`)
		case r.URL.Path == "/tv/1399/season/1":
			fmt.Fprint(w, `{"episodes":[{"episode_number":1,"air_date":"2011-04-17"},{"episode_number":2,"air_date":"2011-04-24"}]}`)
		case r.URL.Path == "/trending/tv/week":
			fmt.Fprint(w, `{"results":[{"id":1399,"name":"Trónok harca","first_air_date":"2011-04-17"}]}`)
		case r.URL.Path == "/watch/providers/movie":
			fmt.Fprint(w, `{"results":[{"provider_id":8,"provider_name":"Netflix","logo_path":"/nf.jpg"}]}`)
		case r.URL.Path == "/watch/providers/tv":
			fmt.Fprint(w, `{"results":[{"provider_id":8,"provider_name":"Netflix"},{"provider_id":350,"provider_name":"Apple TV+"}]}`)
		case r.URL.Path == "/genre/movie/list", r.URL.Path == "/genre/tv/list":
			if r.URL.Query().Get("language") == "en-US" {
				fmt.Fprint(w, `{"genres":[{"id":18,"name":"Drama"}]}`)
				return
			}
			fmt.Fprint(w, `{"genres":[{"id":18,"name":"Dráma"}]}`)
		case r.URL.Path == "/movie/603/watch/providers":
			fmt.Fprint(w, `{"results":{"HU":{"flatrate":[{"provider_id":8,"provider_name":"Netflix"}]}}}`)
		case r.URL.Path == "/discover/movie":
			if r.URL.Query().Get("with_watch_providers") != "8" {
				fmt.Fprint(w, `{"page":1,"total_pages":0,"results":[]}`)
				return
			}
			fmt.Fprint(w, `{"page":1,"total_pages":4,"total_results":70,"results":[{"id":603,"title":"Mátrix","release_date":"1999-03-31"}]}`)
		case r.URL.Path == "/trending/movie/week" && r.URL.Query().Get("language") == "en-US":
			fmt.Fprint(w, `{"results":[{"id":603,"title":"The Matrix","release_date":"1999-03-31","poster_path":"/m.jpg"}]}`)
		case r.URL.Path == "/trending/movie/week", r.URL.Path == "/search/movie":
			fmt.Fprint(w, `{"results":[{"id":603,"title":"Mátrix","release_date":"1999-03-31","poster_path":"/m.jpg"}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{}`)
		}
	}))
}

func TestRequestsAPI(t *testing.T) {
	admin, st, srv := newTestServerFull(t)
	tm := fakeTMDB()
	defer tm.Close()
	srv.Meta.BaseURL, srv.Meta.Delay = tm.URL, 0
	st.SetSetting("tmdb_key", "test")

	if code := admin.do("POST", "/api/admin/users", map[string]any{"username": "anna", "password": "secret123"}, nil); code != 201 && code != 200 {
		t.Fatalf("create user %d", code)
	}
	jar, _ := cookiejar.New(nil)
	anna := &client{t: t, base: admin.base, hc: &http.Client{Jar: jar}}
	if code := anna.do("POST", "/api/login", map[string]string{"username": "anna", "password": "secret123"}, nil); code != 200 {
		t.Fatalf("login %d", code)
	}
	var me struct {
		Profiles   []struct{ ID int64 } `json:"profiles"`
		CanRequest bool                 `json:"can_request"`
	}
	anna.do("GET", "/api/me", nil, &me)
	anna.do("POST", fmt.Sprintf("/api/profiles/%d/select", me.Profiles[0].ID), nil, nil)
	if me.CanRequest {
		t.Error("can_request while acquisition is off")
	}
	if code := anna.do("GET", "/api/discover?list=trending", nil, nil); code != http.StatusConflict {
		t.Errorf("discover while off: %d", code)
	}
	admin.do("PUT", "/api/admin/acquire/settings", map[string]any{"enabled": true}, nil)
	anna.do("GET", "/api/me", nil, &me)
	if !me.CanRequest {
		t.Error("can_request false while on")
	}

	var list []map[string]any
	if code := anna.do("GET", "/api/discover?list=trending", nil, &list); code != 200 || len(list) != 1 || !strings.HasSuffix(list[0]["poster"].(string), "/m.jpg") {
		t.Fatalf("discover: %d %v", code, list)
	}
	if code := anna.do("GET", "/api/discover?list=nope", nil, nil); code != 400 {
		t.Errorf("unknown list: %d", code)
	}
	// the filter bar: streaming services of the region (films first, then series-only ones) and genres
	var opts struct {
		Region    string `json:"region"`
		Providers []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"providers"`
		MovieGenres []struct{ ID int } `json:"movie_genres"`
	}
	if code := anna.do("GET", "/api/discover/options", nil, &opts); code != 200 || opts.Region != "HU" || len(opts.Providers) != 2 ||
		opts.Providers[1].Name != "Apple TV+" || len(opts.MovieGenres) != 1 {
		t.Fatalf("options: %d %+v", code, opts)
	}
	var page struct {
		Results    []map[string]any `json:"results"`
		TotalPages int              `json:"total_pages"`
	}
	if code := anna.do("GET", "/api/discover/browse?kind=movie&providers=8,x&sort=popular&page=1", nil, &page); code != 200 || len(page.Results) != 1 ||
		page.Results[0]["kind"] != "movie" || page.TotalPages != 4 {
		t.Fatalf("browse: %d %+v", code, page)
	}
	if code := anna.do("GET", "/api/discover/browse?sort=shuffle", nil, nil); code != 400 {
		t.Errorf("unknown sort: %d", code)
	}
	// where a request may go: films libraries for a film, series libraries for a series
	var libs struct {
		Movies []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"movies"`
		Shows []struct {
			ID int64 `json:"id"`
		} `json:"shows"`
	}
	if code := anna.do("GET", "/api/discover/libraries", nil, &libs); code != 200 || len(libs.Movies) != 1 || libs.Movies[0].Name != "Filmek" || len(libs.Shows) != 1 {
		t.Fatalf("libraries: %d %+v", code, libs)
	}
	if code := anna.do("POST", "/api/requests", map[string]any{"tmdb_id": 603, "library_id": libs.Shows[0].ID}, nil); code == 200 {
		t.Error("a film requested into a series library")
	}
	var q map[string]any
	if code := anna.do("POST", "/api/requests", map[string]any{"tmdb_id": 603, "library_id": libs.Movies[0].ID}, &q); code != 200 || q["status"] != "pending" || q["title"] != "Mátrix" ||
		q["library_name"] != "Filmek" {
		t.Fatalf("request: %d %v", code, q)
	}
	var d map[string]any
	anna.do("GET", "/api/discover/movies/603", nil, &d)
	if r, _ := d["request"].(map[string]any); r == nil || r["status"] != "pending" {
		t.Errorf("movie detail request: %v", d)
	}
	if wt, _ := d["watch"].(map[string]any); wt == nil || len(wt["stream"].([]any)) != 1 {
		t.Errorf("movie detail where to watch: %v", d["watch"])
	}
	var mine []map[string]any
	if anna.do("GET", "/api/requests", nil, &mine); len(mine) != 1 || mine[0]["user_name"] != "anna" {
		t.Errorf("mine: %v", mine)
	}
	if code := anna.do("GET", "/api/admin/acquire/requests", nil, nil); code != http.StatusForbidden {
		t.Errorf("non-admin sees the admin list: %d", code)
	}
	var open []map[string]any
	admin.do("GET", "/api/admin/acquire/requests?status=open", nil, &open)
	if len(open) != 1 {
		t.Fatalf("open: %v", open)
	}
	id := int64(open[0]["id"].(float64))
	if code := admin.do("POST", fmt.Sprintf("/api/admin/acquire/requests/%d/deny", id), map[string]string{"note": "nem most"}, nil); code != 200 {
		t.Errorf("deny %d", code)
	}
	r, _ := st.AcqRequest(id)
	if r.Status != "denied" || r.Note != "nem most" {
		t.Errorf("denied: %+v", r)
	}
	if code := admin.do("POST", fmt.Sprintf("/api/admin/acquire/requests/%d/approve", id), nil, nil); code != 200 {
		t.Errorf("approve %d", code)
	}
	if code := anna.do("DELETE", fmt.Sprintf("/api/requests/%d", id), nil, nil); code != 200 {
		t.Errorf("cancel own approved request: %d", code)
	}

	// settings for requests
	if code := admin.do("PUT", "/api/admin/acquire/settings", map[string]any{"auto_approve": "everyone"}, nil); code != 400 {
		t.Errorf("bad auto_approve: %d", code)
	}
	var set map[string]any
	if code := admin.do("PUT", "/api/admin/acquire/settings", map[string]any{"auto_approve": "all", "quota_week": 3, "retry_hours": 12}, &set); code != 200 ||
		set["auto_approve"] != "all" || set["quota_week"].(float64) != 3 || set["retry_hours"].(float64) != 12 {
		t.Errorf("request settings: %d %v", code, set)
	}

	// a kids profile cannot ask
	var kid map[string]any
	if code := anna.do("POST", "/api/profiles", map[string]any{"name": "Kicsi", "is_kids": true}, &kid); code != 201 && code != 200 {
		t.Fatalf("kids profile %d", code)
	}
	anna.do("POST", fmt.Sprintf("/api/profiles/%d/select", int64(kid["id"].(float64))), nil, nil)
	if code := anna.do("POST", "/api/requests", map[string]any{"tmdb_id": 603}, nil); code != http.StatusForbidden {
		t.Errorf("kids request: %d", code)
	}
	anna.do("GET", "/api/me", nil, &me)
	if me.CanRequest {
		t.Error("kids profile can_request")
	}
}

// Felfedezés follows the client's UI language, like the library's own metadata.
func TestDiscoverLanguage(t *testing.T) {
	admin, st, srv := newTestServerFull(t)
	tm := fakeTMDB()
	defer tm.Close()
	srv.Meta.BaseURL, srv.Meta.Delay = tm.URL, 0
	st.SetSetting("tmdb_key", "test")
	admin.do("PUT", "/api/admin/acquire/settings", map[string]any{"enabled": true}, nil)
	var me struct {
		Profiles []struct{ ID int64 } `json:"profiles"`
	}
	admin.do("GET", "/api/me", nil, &me)
	admin.do("POST", fmt.Sprintf("/api/profiles/%d/select", me.Profiles[0].ID), nil, nil)

	var list []map[string]any
	if code := admin.do("GET", "/api/discover?list=trending", nil, &list); code != 200 || len(list) != 1 || list[0]["title"] != "Mátrix" {
		t.Fatalf("discover without a language: %d %v", code, list)
	}
	admin.hdr = map[string]string{"X-Kinora-Lang": "en"}
	if code := admin.do("GET", "/api/discover?list=trending", nil, &list); code != 200 || len(list) != 1 || list[0]["title"] != "The Matrix" {
		t.Fatalf("discover in English: %d %v", code, list)
	}
	var en struct {
		MovieGenres []struct{ Name string } `json:"movie_genres"`
	}
	if code := admin.do("GET", "/api/discover/options", nil, &en); code != 200 || len(en.MovieGenres) != 1 || en.MovieGenres[0].Name != "Drama" {
		t.Fatalf("genres in English: %d %+v", code, en)
	}
	admin.hdr = map[string]string{"X-Kinora-Lang": "hu"}
	if code := admin.do("GET", "/api/discover/options", nil, &en); code != 200 || len(en.MovieGenres) != 1 || en.MovieGenres[0].Name != "Dráma" {
		t.Fatalf("genres in Hungarian: %d %+v", code, en)
	}
}

func TestSeriesAPI(t *testing.T) {
	admin, st, srv := newTestServerFull(t)
	tm := fakeTMDB()
	defer tm.Close()
	srv.Meta.BaseURL, srv.Meta.Delay = tm.URL, 0
	st.SetSetting("tmdb_key", "test")
	admin.do("PUT", "/api/admin/acquire/settings", map[string]any{"enabled": true}, nil)
	if code := admin.do("POST", "/api/admin/acquire/series", map[string]any{"tmdb_id": 1399, "monitor": "every"}, nil); code == 200 {
		t.Error("unknown monitor mode accepted")
	}
	shows, _ := filepath.Abs("../../testmedia/Shows")
	var lib map[string]any
	admin.do("POST", "/api/libraries", map[string]any{"name": "Sorozatok", "path": shows, "kind": "shows"}, &lib)
	var x map[string]any
	// two series libraries now: one has to be chosen
	if code := admin.do("POST", "/api/admin/acquire/series", map[string]any{"tmdb_id": 1399, "monitor": "seasons", "seasons": []int{1}}, &x); code == 200 {
		t.Error("followed without choosing among two series libraries")
	}
	if code := admin.do("POST", "/api/admin/acquire/series", map[string]any{"tmdb_id": 1399, "monitor": "seasons", "seasons": []int{1}, "library_id": lib["id"]}, &x); code != 200 {
		t.Fatalf("follow %d %v", code, x)
	}
	if x["total"].(float64) != 2 || x["wanted"].(float64) != 2 || x["monitor"] != "seasons" {
		t.Errorf("followed: %v", x)
	}
	id := int64(x["id"].(float64))
	var d struct {
		Series   map[string]any   `json:"series"`
		Episodes []map[string]any `json:"episodes"`
	}
	admin.do("GET", fmt.Sprintf("/api/admin/acquire/series/%d", id), nil, &d)
	if len(d.Episodes) != 2 || d.Episodes[0]["state"] != "wanted" {
		t.Errorf("episodes: %v", d.Episodes)
	}
	if code := admin.do("PATCH", fmt.Sprintf("/api/admin/acquire/series/%d", id), map[string]any{"monitor": "none"}, &x); code != 200 || x["wanted"].(float64) != 0 {
		t.Errorf("monitor none: %d %v", code, x)
	}
	var me struct {
		Profiles []struct{ ID int64 } `json:"profiles"`
	}
	admin.do("GET", "/api/me", nil, &me)
	admin.do("POST", fmt.Sprintf("/api/profiles/%d/select", me.Profiles[0].ID), nil, nil)
	var list []map[string]any
	if code := admin.do("GET", "/api/discover?list=tv_trending", nil, &list); code != 200 {
		t.Fatalf("tv discover %d", code)
	}
	if len(list) != 1 || list[0]["kind"] != "series" || list[0]["followed"] != true {
		t.Errorf("tv discover: %v", list)
	}
	var sd map[string]any
	if admin.do("GET", "/api/discover/series/1399", nil, &sd); len(sd["seasons"].([]any)) != 1 || sd["followed"] != true {
		t.Errorf("series detail: %v", sd)
	}
	var q map[string]any
	if code := admin.do("POST", "/api/requests", map[string]any{"kind": "series", "tmdb_id": 1399, "seasons": "1", "library_id": lib["id"]}, &q); code != 200 || q["kind"] != "series" || q["seasons"] != "1" {
		t.Errorf("series request: %d %v", code, q)
	}
	if code := admin.do("POST", "/api/requests", map[string]any{"kind": "series", "tmdb_id": 1399, "seasons": "0"}, nil); code == 200 {
		t.Error("season 0 requested")
	}
	if code := admin.do("DELETE", fmt.Sprintf("/api/admin/acquire/series/%d", id), nil, nil); code != 200 {
		t.Errorf("stop %d", code)
	}
}
