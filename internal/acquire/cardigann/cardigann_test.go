package cardigann

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// A private test site: a sign-in form with a hidden token, an HTML results table behind the session,
// details pages with the download link, and .torrent files only for signed-in users.
const privateDef = `---
id: testtracker
name: Test Tracker
type: private
language: hu-HU
links:
  - %s/
caps:
  categorymappings:
    - {id: 1, cat: Movies/HD, desc: "Filmek HD"}
    - {id: 2, cat: TV/HD, desc: "Sorozatok HD"}
  modes:
    search: [q]
    movie-search: [q, imdbid]
settings:
  - {name: username, type: text, label: Felhasználónév}
  - {name: password, type: password, label: Jelszó}
  - {name: freeleech, type: checkbox, label: Csak freeleech, default: false}
login:
  path: login.php
  method: form
  form: form#login
  inputs:
    user: "{{ .Config.username }}"
    pass: "{{ .Config.password }}"
  error:
    - selector: div.error
  test:
    path: index.php
    selector: a.logout
search:
  paths:
    - path: torrents.php
  keywordsfilters:
    - name: re_replace
      args: ["[^a-zA-Z0-9]+", "%%"]
  inputs:
    search: "{{ if .Query.IMDBID }}{{ .Query.IMDBID }}{{ else }}{{ .Keywords }}{{ end }}"
    $raw: "{{ range .Categories }}cat[]={{ . }}&{{ end }}"
  rows:
    selector: table#torrents > tbody > tr
  fields:
    category:
      selector: a[href*="cat="]
      attribute: href
      filters:
        - name: querystring
          args: cat
    title_raw:
      selector: a.title
    title:
      text: "{{ .Result.title_raw }}"
      filters:
        - name: replace
          args: [" ", "."]
    details:
      selector: a.title
      attribute: href
    download:
      selector: a.title
      attribute: href
    size:
      selector: td.size
    seeders:
      selector: td.seed
    leechers:
      selector: td.leech
    date:
      selector: td.added
      filters:
        - name: dateparse
          args: "yyyy-MM-dd HH:mm"
    imdbid:
      selector: a[href*="imdb.com"]
      attribute: href
      optional: true
    downloadvolumefactor:
      case:
        img.free: 0
        "*": 1
download:
  selectors:
    - selector: a[href*="download.php"]
      attribute: href
`

const publicJSONDef = `---
id: testjson
name: Test JSON
type: public
links:
  - %s/
caps:
  categorymappings:
    - {id: movies, cat: Movies}
search:
  paths:
    - path: "api/search/{{ .Keywords }}"
      response:
        type: json
  rows:
    selector: data.results
  fields:
    title:
      selector: name
    magnet:
      selector: links.magnet
    size:
      selector: size
    seeders:
      selector: stats.seeders
    category:
      text: movies
    date:
      selector: added
      filters:
        - name: timeago
`

type fakeSite struct {
	mu       sync.Mutex
	sessions map[string]bool
	searches []string
}

func (f *fakeSite) handler() http.Handler {
	mux := http.NewServeMux()
	signed := func(r *http.Request) bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		c, err := r.Cookie("sid")
		return err == nil && f.sessions[c.Value]
	}
	mux.HandleFunc("/login.php", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			fmt.Fprint(w, `<html><form id="login" action="takelogin.php" method="post"><input type="hidden" name="csrf" value="tok42"><input name="user"><input name="pass" type="password"></form></html>`)
		}
	})
	mux.HandleFunc("/takelogin.php", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("csrf") != "tok42" || r.Form.Get("user") != "anna" || r.Form.Get("pass") != "titok" {
			fmt.Fprint(w, `<html><div class="error">Hibás felhasználónév vagy jelszó!</div></html>`)
			return
		}
		f.mu.Lock()
		f.sessions["s1"] = true
		f.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "sid", Value: "s1", Path: "/"})
		http.Redirect(w, r, "/index.php", http.StatusFound)
	})
	mux.HandleFunc("/index.php", func(w http.ResponseWriter, r *http.Request) {
		if signed(r) {
			fmt.Fprint(w, `<a class="logout" href="logout.php">Kilépés</a>`)
			return
		}
		fmt.Fprint(w, `<a href="login.php">Belépés</a>`)
	})
	mux.HandleFunc("/torrents.php", func(w http.ResponseWriter, r *http.Request) {
		if !signed(r) {
			http.Redirect(w, r, "/login.php", http.StatusFound)
			return
		}
		f.mu.Lock()
		f.searches = append(f.searches, r.URL.RawQuery)
		f.mu.Unlock()
		fmt.Fprint(w, `<table id="torrents"><tbody>
<tr><td><a href="torrents.php?cat=1">HD</a></td><td><a class="title" href="details.php?id=1">The Matrix 1999 1080p BluRay x264 HUN</a>
<a href="https://www.imdb.com/title/tt0133093/">imdb</a><img class="free"></td>
<td class="size">8,4 GiB</td><td class="seed">1 234</td><td class="leech">5</td><td class="added">2023-01-02 10:00</td></tr>
<tr><td><a href="torrents.php?cat=2">TV</a></td><td><a class="title" href="details.php?id=2">Show S01E01 720p WEB</a></td>
<td class="size">700 MB</td><td class="seed">12</td><td class="leech">0</td><td class="added">2023-02-03 08:30</td></tr>
<tr><td>a row without a title link: skipped</td></tr>
</tbody></table>`)
	})
	mux.HandleFunc("/details.php", func(w http.ResponseWriter, r *http.Request) {
		if !signed(r) {
			http.Redirect(w, r, "/login.php", http.StatusFound)
			return
		}
		fmt.Fprintf(w, `<h1>Details</h1><a href="download.php?id=%s&amp;key=abc">Letöltés</a>`, r.URL.Query().Get("id"))
	})
	mux.HandleFunc("/download.php", func(w http.ResponseWriter, r *http.Request) {
		if !signed(r) || r.URL.Query().Get("key") != "abc" {
			fmt.Fprint(w, `<html>please log in</html>`)
			return
		}
		fmt.Fprint(w, "d8:announce4:none4:infod4:name4:testee")
	})
	mux.HandleFunc("/api/search/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":{"results":[{"name":"Big.Buck.Bunny.2008.1080p","links":{"magnet":"magnet:?xt=urn:btih:dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c"},"size":1073741824,"stats":{"seeders":7},"added":"2 days ago"},{"name":"No.Link"}]}}`)
	})
	return mux
}

func TestPrivateHTMLDefinition(t *testing.T) {
	site := &fakeSite{sessions: map[string]bool{}}
	srv := httptest.NewServer(site.handler())
	defer srv.Close()
	def, err := Parse([]byte(fmt.Sprintf(privateDef, srv.URL)))
	if err != nil {
		t.Fatal(err)
	}
	if def.ID != "testtracker" || len(def.Settings) != 3 || !def.Settings[1].Secret() {
		t.Fatalf("parse: %+v", def)
	}
	ctx := context.Background()
	bad := New(def, map[string]any{"username": "anna", "password": "rossz"}, srv.Client())
	if _, err := bad.Test(ctx); err == nil || !strings.Contains(err.Error(), "Hibás felhasználónév") {
		t.Errorf("wrong password: %v", err)
	}
	ix := New(def, map[string]any{"username": "anna", "password": "titok"}, srv.Client())
	n, err := ix.Test(ctx)
	if err != nil || n != 2 {
		t.Fatalf("test: %d %v", n, err)
	}
	rows, err := ix.Search(ctx, Query{Type: "movie", Keywords: "The Matrix: 1999"})
	if err != nil || len(rows) != 2 {
		t.Fatalf("search: %v %v", rows, err)
	}
	last := site.searches[len(site.searches)-1]
	if !strings.Contains(last, "search=The%25Matrix%251999") || !strings.Contains(last, "cat[]=1") || strings.Contains(last, "cat[]=2") {
		t.Errorf("query: %s", last)
	}
	r := rows[0]
	if r.Title != "The.Matrix.1999.1080p.BluRay.x264.HUN" || r.Size != parseSize("8.4 GiB") || r.Seeders != 1234 || r.Leechers != 5 ||
		r.IMDb != "tt0133093" || !r.Freeleech || len(r.Categories) != 1 || r.Categories[0] != "Movies/HD" ||
		r.Date.Format("2006-01-02 15:04") != "2023-01-02 10:00" || !strings.HasSuffix(r.Download, "/details.php?id=1") {
		t.Errorf("row: %+v", r)
	}
	if rows[1].Size != 700<<20 || rows[1].Freeleech || rows[1].Categories[0] != "TV/HD" {
		t.Errorf("second row: %+v", rows[1])
	}
	// an IMDb search puts the id in the input
	ix.Search(ctx, Query{Type: "movie", Keywords: "x", IMDBID: "tt0133093", IMDBIDShort: "0133093"})
	if last = site.searches[len(site.searches)-1]; !strings.Contains(last, "search=tt0133093") {
		t.Errorf("imdb query: %s", last)
	}
	// download: the details page, then the .torrent with the session
	b, mag, err := ix.Download(ctx, r.Download)
	if err != nil || mag != "" || !strings.HasPrefix(string(b), "d8:announce") {
		t.Fatalf("download: %q %q %v", b, mag, err)
	}
	// the session runs out: the next search signs in again
	site.mu.Lock()
	site.sessions = map[string]bool{}
	site.mu.Unlock()
	if rows, err := ix.Search(ctx, Query{Type: "search", Keywords: "matrix"}); err != nil || len(rows) != 2 {
		t.Errorf("after the session ran out: %d %v", len(rows), err)
	}
}

func TestPublicJSONDefinition(t *testing.T) {
	srv := httptest.NewServer((&fakeSite{sessions: map[string]bool{}}).handler())
	defer srv.Close()
	def, err := Parse([]byte(fmt.Sprintf(publicJSONDef, srv.URL)))
	if err != nil {
		t.Fatal(err)
	}
	ix := New(def, nil, srv.Client())
	rows, err := ix.Search(context.Background(), Query{Type: "movie", Keywords: "big buck"})
	if err != nil || len(rows) != 1 {
		t.Fatalf("search: %+v %v", rows, err)
	}
	r := rows[0]
	if r.Title != "Big.Buck.Bunny.2008.1080p" || !strings.HasPrefix(r.Magnet, "magnet:") || r.Size != 1<<30 || r.Seeders != 7 ||
		r.Categories[0] != "Movies" || time.Since(r.Date) < 47*time.Hour || time.Since(r.Date) > 49*time.Hour {
		t.Errorf("row: %+v", r)
	}
	if _, mag, err := ix.Download(context.Background(), r.Magnet); err != nil || mag != r.Magnet {
		t.Errorf("magnet download: %v", err)
	}
}

func TestParseErrorsAndHelpers(t *testing.T) {
	for _, bad := range []string{"id: x\nname: y", "name: y\nlinks: [http://a/]\nsearch: {path: s}", ": : :"} {
		if _, err := Parse([]byte(bad)); err == nil {
			t.Errorf("accepted: %q", bad)
		}
	}
	sizes := map[string]int64{"1.5 GB": 1610612736, "700 MB": 700 << 20, "1,234.5 KB": 1264128, "1,4 GiB": 1503238553, "123456": 123456, "2 TB": 2 << 40}
	for in, want := range sizes {
		if got := parseSize(in); got != want {
			t.Errorf("parseSize(%q) = %d, want %d", in, got, want)
		}
	}
	if l := dotnetLayout("dd.MM.yyyy HH:mm"); l != "02.01.2006 15:04" {
		t.Errorf("layout: %q", l)
	}
	if v, _ := applyFilter("Ünnepi Mátrix", Filter{Name: "diacritics"}, nil); v != "Unnepi Matrix" {
		t.Errorf("diacritics: %q", v)
	}
	if v, _ := applyFilter("a-b-c", Filter{Name: "split", Args: []any{"-", -1}}, nil); v != "c" {
		t.Errorf("split: %q", v)
	}
	if v, _ := applyFilter("Size: 12 GB", Filter{Name: "regexp", Args: `(\d+) GB`}, nil); v != "12" {
		t.Errorf("regexp: %q", v)
	}
	// definitions are written for .NET: lookarounds, Unicode blocks and "$1" replacements work
	if v, err := applyFilter("ab cb", Filter{Name: "re_replace", Args: []any{"(?<=a)b", "X"}}, nil); err != nil || v != "aX cb" {
		t.Errorf("lookbehind: %q %v", v, err)
	}
	if v, err := applyFilter("Mátrix 黑客帝国", Filter{Name: "re_replace", Args: []any{`([\p{IsCJKUnifiedIdeographs}\W]+)`, "."}}, nil); err != nil || v != "Mátrix." {
		t.Errorf("unicode block: %q %v", v, err)
	}
	if _, err := applyFilter("x", Filter{Name: "re_replace", Args: []any{"(a", ""}}, nil); err == nil {
		t.Error("a broken pattern should fail with a clear error")
	}
	// a .NET pattern inside a template literal, a dashed setting name, a filter argument that is a template
	ctx := newCtx(map[string]any{"cat-id": "0_0", "y": true}, Query{Keywords: "Show 01"}, nil)
	if v, err := render(`{{ re_replace .Keywords "\b0(\d)\b" "$1" }}-{{ .Config.cat-id }}`, ctx); err != nil || v != "Show 1-0_0" {
		t.Errorf("template: %q %v", v, err)
	}
	if v, _ := applyFilter("a", Filter{Name: "append", Args: "{{ if .Config.y }}!{{ end }}"}, ctx); v != "a!" {
		t.Errorf("templated argument: %q", v)
	}
	if v, err := render(`{{ re_replace .Keywords "\\s+" "+" }}|{{ join .Categories "," }}|{{ if .Query.IMDBID }}i{{ else }}k{{ end }}`,
		newCtx(nil, Query{Keywords: "a  b c"}, []string{"1", "2"})); err != nil || v != "a+b+c|1,2|k" {
		t.Errorf("render: %q %v", v, err)
	}
}
