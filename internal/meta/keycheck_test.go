package meta

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ScotyLucas/kinora/internal/store"
)

func TestExtractKey(t *testing.T) {
	for in, want := range map[string]string{
		"  abcd1234 ": "abcd1234",
		"http://www.omdbapi.com/?i=tt3896198&apikey=abcd1234":       "abcd1234",
		"https://api.themoviedb.org/3/movie/550?api_key=abc123&x=1": "abc123",
		"eyJhbGciOiJIUzI1NiJ9.eyJhdWQiOiJ4In0.sig":                  "eyJhbGciOiJIUzI1NiJ9.eyJhdWQiOiJ4In0.sig",
	} {
		if got := ExtractKey(in); got != want {
			t.Errorf("ExtractKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestKeyChecks(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/3/configuration", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("api_key") != "good" {
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"status_message":"Invalid API key: You must be granted a valid key."}`)
			return
		}
		io.WriteString(w, `{"images":{}}`)
	})
	mux.HandleFunc("/omdb/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("apikey") {
		case "good":
			io.WriteString(w, `{"Response":"True","imdbRating":"9.3"}`)
		case "spent":
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"Response":"False","Error":"Request limit reached!"}`)
		default:
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"Response":"False","Error":"Invalid API key!"}`)
		}
	})
	mux.HandleFunc("/os/infos/formats", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Api-Key") != "good" {
			w.WriteHeader(http.StatusForbidden)
			io.WriteString(w, `{"message":"You cannot consume this service"}`)
			return
		}
		io.WriteString(w, `{"data":{}}`)
	})
	mux.HandleFunc("/os/login", func(w http.ResponseWriter, r *http.Request) {
		var b map[string]string
		json.NewDecoder(r.Body).Decode(&b)
		if b["password"] != "pw" {
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"message":"Error, invalid username/password"}`)
			return
		}
		io.WriteString(w, `{"token":"t"}`)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	st, err := store.Open(filepath.Join(t.TempDir(), "k.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	f := New(st, t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	f.BaseURL, f.OmdbURL = ts.URL+"/3", ts.URL+"/omdb/"
	os := NewOpenSubtitles(st, "Kinora test")
	os.BaseURL = ts.URL + "/os"
	ctx := context.Background()

	check := func(name string, got KeyCheck, state string) {
		t.Helper()
		if got.State != state || got.OK != (state == "ok") {
			t.Errorf("%s: %+v, want %s", name, got, state)
		}
	}
	check("tmdb good", f.CheckTMDB(ctx, "good"), "ok")
	check("tmdb bad", f.CheckTMDB(ctx, "bad"), "invalid")
	check("tmdb empty", f.CheckTMDB(ctx, " "), "missing")
	check("omdb good", f.CheckOmdb(ctx, "good"), "ok")
	check("omdb pasted url", f.CheckOmdb(ctx, "http://www.omdbapi.com/?i=tt3896198&apikey=good"), "ok")
	check("omdb bad", f.CheckOmdb(ctx, "bad"), "invalid")
	check("omdb limit", f.CheckOmdb(ctx, "spent"), "limit")
	check("os key only", os.CheckKey(ctx, "good", "", ""), "ok")
	check("os bad key", os.CheckKey(ctx, "bad", "u", "pw"), "invalid")
	check("os login", os.CheckKey(ctx, "good", "u", "pw"), "ok")
	check("os bad login", os.CheckKey(ctx, "good", "u", "nope"), "login")
}
