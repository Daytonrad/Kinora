package cardigann

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// A public JSON site behind a Cloudflare-style check: without the clearance cookie and the solving
// browser's User-Agent it answers 403 "Just a moment...". A fake FlareSolverr "solves" it.
func TestFlareSolverr(t *testing.T) {
	const ua = "SolvingBrowser/1.0"
	var direct int
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("cf_clearance"); err != nil || c.Value != "ok" || r.UserAgent() != ua {
			w.Header().Set("Server", "cloudflare")
			w.WriteHeader(403)
			fmt.Fprint(w, "<html><title>Just a moment...</title></html>")
			return
		}
		direct++
		fmt.Fprint(w, `[{"name":"Film.2024.1080p.WEB-DL","hash":"0123456789abcdef0123456789abcdef01234567","size":"1000"}]`)
	}))
	defer site.Close()
	var solved int
	fs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			fmt.Fprint(w, `{"msg":"FlareSolverr is ready!","version":"3.3.21"}`)
			return
		}
		var in map[string]any
		json.NewDecoder(r.Body).Decode(&in)
		solved++
		host := strings.TrimPrefix(site.URL, "http://")
		json.NewEncoder(w).Encode(map[string]any{"status": "ok", "solution": map[string]any{
			"url": in["url"], "status": 200, "userAgent": ua,
			"cookies":  []map[string]any{{"name": "cf_clearance", "value": "ok", "domain": strings.Split(host, ":")[0], "path": "/"}},
			"response": `<html><head></head><body><pre>[{"name":"Film.2024.1080p.WEB-DL","hash":"0123456789abcdef0123456789abcdef01234567","size":"1000"}]</pre></body></html>`,
		}})
	}))
	defer fs.Close()

	def, err := Parse([]byte(`id: cfsite
name: CF Site
type: public
links: [` + site.URL + `/]
search:
  paths: [{path: "api?q={{ .Keywords }}", response: {type: json}}]
  rows: {selector: $}
  fields:
    title: {selector: name}
    infohash: {selector: hash}
    size: {selector: size}
`))
	if err != nil {
		t.Fatal(err)
	}
	q := Query{Type: "movie", Keywords: "Film"}
	hc := &http.Client{Timeout: 5 * time.Second}

	// no solver: a clear message instead of "HTTP 403"
	if _, err := New(def, nil, hc).Search(context.Background(), q); err == nil || !strings.Contains(err.Error(), "FlareSolverr") {
		t.Fatalf("without a solver: %v", err)
	}
	ix := New(def, nil, hc)
	ix.Solver = NewFlareSolverr(fs.URL + "/")
	rows, err := ix.Search(context.Background(), q)
	if err != nil || len(rows) != 1 || !strings.HasPrefix(rows[0].Magnet, "magnet:?xt=urn:btih:0123456789abcdef") {
		t.Fatalf("solved search: %v %+v", err, rows)
	}
	// the clearance is kept: the next search goes straight to the site
	if rows, err = ix.Search(context.Background(), q); err != nil || len(rows) != 1 || solved != 1 || direct != 1 {
		t.Errorf("second search: %v rows=%d solved=%d direct=%d", err, len(rows), solved, direct)
	}
	if v, err := ix.Solver.Test(context.Background()); err != nil || v != "3.3.21" {
		t.Errorf("test: %q %v", v, err)
	}
	if _, err := NewFlareSolverr(site.URL).Test(context.Background()); err == nil {
		t.Error("a non-FlareSolverr address passed the test")
	}
}

// Prowlarr definitions: an object's array attribute gives the rows, with "..x" reading the object (YTS),
// and a setting default such as 0_0 stays a string.
func TestJSONRowAttributeAndDefaults(t *testing.T) {
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("c") != "0_0" {
			http.Error(w, "bad category "+r.URL.Query().Get("c"), 400)
			return
		}
		fmt.Fprint(w, `{"data":{"movies":[{"title":"Film","year":2024,"torrents":[
			{"quality":"1080p","hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},{"quality":"2160p","hash":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}]}]}}`)
	}))
	defer site.Close()
	def, err := Parse([]byte(`id: ytsish
name: YTSish
type: public
links: [` + site.URL + `/]
settings:
  - {name: cat-id, type: select, label: Category, default: 0_0, options: {0_0: All}}
search:
  paths: [{path: "list movies.json", response: {type: json}}]
  inputs: {c: "{{ .Config.cat-id }}"}
  rows: {selector: data.movies, attribute: torrents, multiple: true}
  fields:
    year: {selector: ..year}
    title: {selector: ..title, filters: [{name: append, args: " ({{ .Result.year }})"}]}
    q: {selector: quality}
    infohash: {selector: hash}
`))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := New(def, nil, &http.Client{Timeout: 5 * time.Second}).Search(context.Background(), Query{Keywords: "Film"})
	if err != nil || len(rows) != 2 || rows[0].Title != "Film (2024)" || rows[1].InfoHash != strings.Repeat("b", 40) {
		t.Fatalf("rows: %v %+v", err, rows)
	}
}
