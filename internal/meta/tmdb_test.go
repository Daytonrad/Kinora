package meta

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/scoty_lucas/kinora/internal/store"
)

func TestRefreshMovieAndSeries(t *testing.T) {
	var hits429 atomic.Int32
	mux := http.NewServeMux()
	j := func(w http.ResponseWriter, v any) { json.NewEncoder(w).Encode(v) }
	mux.HandleFunc("/3/search/movie", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("api_key") != "k123" || r.URL.Query().Get("language") != "hu-HU" {
			w.WriteHeader(401)
			return
		}
		if hits429.Add(1) == 1 { // first call is rate limited
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
			return
		}
		switch r.URL.Query().Get("query") {
		case "Arrival":
			j(w, map[string]any{"results": []map[string]any{{"id": 1, "title": "Wrong", "release_date": "1996-01-01"}, {"id": 329865, "title": "Érkezés", "release_date": "2016-11-10"}}})
		default:
			j(w, map[string]any{"results": []any{}})
		}
	})
	mux.HandleFunc("/3/movie/329865", func(w http.ResponseWriter, r *http.Request) {
		j(w, map[string]any{"id": 329865, "title": "Érkezés", "release_date": "2016-11-10", "overview": "Nyelvész.", "vote_average": 7.6,
			"genres": []map[string]any{{"name": "Sci-Fi"}, {"name": "Dráma"}}, "belongs_to_collection": map[string]any{"name": "Villeneuve gyűjtemény"},
			"release_dates": map[string]any{"results": []map[string]any{
				{"iso_3166_1": "US", "release_dates": []map[string]any{{"certification": "PG-13"}}},
				{"iso_3166_1": "HU", "release_dates": []map[string]any{{"certification": "12"}}}}}})
	})
	mux.HandleFunc("/3/search/tv", func(w http.ResponseWriter, r *http.Request) {
		j(w, map[string]any{"results": []map[string]any{{"id": 77, "name": "Teszt Sorozat", "first_air_date": "2020-01-01"}}})
	})
	mux.HandleFunc("/3/tv/77", func(w http.ResponseWriter, r *http.Request) {
		j(w, map[string]any{"id": 77, "name": "Teszt Sorozat", "first_air_date": "2020-01-01", "overview": "Leírás", "vote_average": 8.2,
			"genres": []map[string]any{{"name": "Sci-Fi & Fantasy"}},
			"content_ratings": map[string]any{"results": []map[string]any{{"iso_3166_1": "US", "rating": "TV-MA"}}}})
	})
	mux.HandleFunc("/3/tv/77/season/1", func(w http.ResponseWriter, r *http.Request) {
		j(w, map[string]any{"episodes": []map[string]any{{"episode_number": 1, "name": "A kezdet", "overview": "Első rész."}}})
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	st, err := store.Open(filepath.Join(t.TempDir(), "k.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.SetSetting("tmdb_key", "k123")
	lib, _ := st.CreateLibrary("x", "/x", "mixed")
	ins := func(kind, key, title string, year int, tmdb any) int64 {
		res, err := st.DB.Exec(`INSERT INTO items(library_id,kind,key,title,sort_title,year,tmdb_id,added_at,updated_at) VALUES(?,?,?,?,?,?,?,0,0)`,
			lib.ID, kind, key, title, title, year, tmdb)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	movie := ins("movie", "m", "Arrival", 2016, nil)
	nfo := ins("movie", "n", "Local NFO", 2000, -1)
	unknown := ins("movie", "u", "Nothing Like This", 2001, nil)
	series := ins("series", "s", "Test Show", 2020, nil)
	st.DB.Exec(`UPDATE items SET series_id=? WHERE id=?`, series, series)
	season := ins("season", "s/s1", "1. évad", 0, nil)
	st.DB.Exec(`UPDATE items SET parent_id=?,series_id=?,season_num=1 WHERE id=?`, series, series, season)
	ep := ins("episode", "e", "1. rész", 0, nil)
	st.DB.Exec(`UPDATE items SET parent_id=?,series_id=?,season_num=1,episode_num=1 WHERE id=?`, season, series, ep)

	f := New(st, t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	f.BaseURL, f.ImageURL, f.Delay = ts.URL+"/3", ts.URL+"/img/", 0
	if err := f.Refresh(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	it, _ := st.ItemFull(movie, nil)
	if it.Title != "Érkezés" || *it.TmdbID != 329865 || it.ContentRating != "HU:12" || *it.RatingLevel != 2 || len(it.Genres) != 2 {
		t.Fatalf("movie: %+v", it)
	}
	if len(it.Tags) != 1 || it.Tags[0] != "Villeneuve gyűjtemény" {
		t.Fatalf("collection tag: %v", it.Tags)
	}
	if n, _ := st.Item(nfo, nil); n.Title != "Local NFO" {
		t.Fatal("NFO item overwritten")
	}
	if u, _ := st.Item(unknown, nil); u.TmdbID == nil || *u.TmdbID != 0 {
		t.Fatal("no-match must be remembered as 0")
	}
	sr, _ := st.ItemFull(series, nil)
	if sr.Title != "Teszt Sorozat" || *sr.RatingLevel != 3 || len(sr.Genres) != 2 {
		t.Fatalf("series: %+v %v", sr, sr.Genres)
	}
	e, _ := st.Item(ep, nil)
	if e.Title != "A kezdet" || e.RatingLevel == nil || *e.RatingLevel != 3 {
		t.Fatalf("episode: %+v", e)
	}
	if st := f.Status(); st.Matched != 2 || st.NoMatch != 1 || st.Running {
		t.Fatalf("status: %+v", st)
	}
	// manual fix
	if err := f.Match(context.Background(), unknown, 329865); err != nil {
		t.Fatal(err)
	}
	if u, _ := st.Item(unknown, nil); u.Title != "Érkezés" {
		t.Fatal("manual match")
	}
}
