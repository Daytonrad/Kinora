package meta

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/ScotyLucas/kinora/internal/store"
)

func TestTitleScores(t *testing.T) {
	var tmdbHits, omdbHits atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/3/movie/603", func(w http.ResponseWriter, r *http.Request) {
		tmdbHits.Add(1)
		io.WriteString(w, `{"id":603,"imdb_id":"tt0133093","vote_average":8.217,"vote_count":26000,"budget":63000000,"revenue":467222728}`)
	})
	mux.HandleFunc("/3/tv/1399", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("append_to_response") != "external_ids" {
			t.Errorf("tv scores without external_ids: %s", r.URL.RawQuery)
		}
		io.WriteString(w, `{"id":1399,"vote_average":8.4,"vote_count":100,"budget":5,"external_ids":{"imdb_id":"tt0944947"}}`)
	})
	mux.HandleFunc("/omdb/", func(w http.ResponseWriter, r *http.Request) {
		omdbHits.Add(1)
		if r.URL.Query().Get("apikey") != "ok" {
			io.WriteString(w, `{"Response":"False","Error":"Invalid API key!"}`)
			return
		}
		switch r.URL.Query().Get("i") {
		case "tt0133093":
			io.WriteString(w, `{"Response":"True","imdbRating":"8.7","imdbVotes":"2,123,456","Metascore":"73",
				"Ratings":[{"Source":"Internet Movie Database","Value":"8.7/10"},{"Source":"Rotten Tomatoes","Value":"83%"},{"Source":"Metacritic","Value":"73/100"}]}`)
		case "tt0944947":
			io.WriteString(w, `{"Response":"True","imdbRating":"9.2","imdbVotes":"2,300,000","Metascore":"N/A","Ratings":[]}`)
		default:
			io.WriteString(w, `{"Response":"False","Error":"Incorrect IMDb ID."}`)
		}
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	st, err := store.Open(filepath.Join(t.TempDir(), "k.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.SetSetting("tmdb_key", "k")
	f := New(st, t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	f.BaseURL, f.OmdbURL = ts.URL+"/3", ts.URL+"/omdb/"
	ctx := context.Background()

	// TMDB only: its own score and the money, no OMDb sites
	s := f.TitleScores(ctx, "movie", 603, "")
	if s.OMDb || len(s.Ratings) != 1 || s.Ratings[0].Source != "tmdb" || s.Ratings[0].Value != 8.2 || s.Budget != 63000000 || s.Revenue != 467222728 {
		t.Fatalf("tmdb only: %+v", s)
	}

	st.SetSetting("omdb_key", "ok")
	f.scores = nil
	s = f.TitleScores(ctx, "movie", 603, "")
	want := []string{"imdb", "rt", "metacritic", "tmdb"}
	if !s.OMDb || len(s.Ratings) != len(want) {
		t.Fatalf("with omdb: %+v", s)
	}
	for i, src := range want {
		if s.Ratings[i].Source != src {
			t.Fatalf("order: %+v", s.Ratings)
		}
	}
	if r := s.Ratings[0]; r.Value != 8.7 || r.Votes != 2123456 || r.URL != "https://www.imdb.com/title/tt0133093/" {
		t.Fatalf("imdb: %+v", r)
	}
	if s.Ratings[1].Value != 83 || s.Ratings[2].Value != 73 {
		t.Fatalf("rt/mc: %+v", s.Ratings)
	}

	// cached: no second round trip
	tm, om := tmdbHits.Load(), omdbHits.Load()
	f.TitleScores(ctx, "movie", 603, "")
	if tmdbHits.Load() != tm || omdbHits.Load() != om {
		t.Fatal("scores not cached")
	}

	// series: IMDb id from external_ids, no money, "N/A" Metascore skipped
	s = f.TitleScores(ctx, "series", 1399, "")
	if s.Budget != 0 || len(s.Ratings) != 2 || s.Ratings[0].Source != "imdb" || s.Ratings[0].Value != 9.2 {
		t.Fatalf("series: %+v", s)
	}

	// an NFO film with only an IMDb id, unknown to OMDb: nothing, but no error
	if s = f.TitleScores(ctx, "movie", 0, "tt9999999"); len(s.Ratings) != 0 {
		t.Fatalf("unknown: %+v", s)
	}
}
