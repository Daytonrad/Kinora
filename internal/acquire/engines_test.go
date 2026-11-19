package acquire

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ScotyLucas/kinora/internal/acquire/cardigann"
	"github.com/ScotyLucas/kinora/internal/store"
)

// A test-only definition for a fake private site (Kinora ships none).
const engineDef = `id: fakeprivate
name: Fake Private
type: private
links: [%s/]
caps:
  categorymappings:
    - {id: 5, cat: Movies/HD}
    - {id: 7, cat: TV/HD}
settings:
  - {name: username, type: text, label: User}
  - {name: password, type: password, label: Password}
login:
  path: login
  method: post
  inputs:
    u: "{{ .Config.username }}"
    p: "{{ .Config.password }}"
  error:
    - selector: .err
search:
  paths:
    - path: browse
  inputs:
    q: "{{ .Keywords }}"
    imdb: "{{ .Query.IMDBID }}"
  rows:
    selector: tr.t
  fields:
    title: {selector: a.n}
    details: {selector: a.n, attribute: href}
    download: {selector: a.n, attribute: href}
    size: {selector: .s}
    seeders: {selector: .se}
    leechers: {text: 0}
download:
  selectors:
    - {selector: a.dl, attribute: href}
`

func fakePrivateSite(t *testing.T, torrent []byte) (*httptest.Server, *[]string) {
	var mu sync.Mutex
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _ := r.Cookie("s")
		in := c != nil && c.Value == "ok"
		switch r.URL.Path {
		case "/login":
			r.ParseForm()
			if r.Form.Get("u") != "kinora" || r.Form.Get("p") != "pw" {
				fmt.Fprint(w, `<p class="err">Rossz jelszó</p>`)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "s", Value: "ok", Path: "/"})
			fmt.Fprint(w, `welcome`)
		case "/browse":
			if !in {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}
			mu.Lock()
			queries = append(queries, r.URL.RawQuery)
			mu.Unlock()
			fmt.Fprint(w, `<table>
<tr class="t"><td><a class="n" href="/t/1">The.Matrix.1999.1080p.BluRay.x264.HUN-KN</a></td><td class="s">9.5 GB</td><td class="se">40</td></tr>
<tr class="t"><td><a class="n" href="/t/2">The.Matrix.Reloaded.2003.1080p.BluRay.x264-X</a></td><td class="s">9 GB</td><td class="se">10</td></tr>
</table>`)
		case "/t/1", "/t/2":
			if !in {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}
			fmt.Fprintf(w, `<a class="dl" href="/dl?id=%s">get</a>`, strings.TrimPrefix(r.URL.Path, "/t/"))
		case "/dl":
			if !in {
				fmt.Fprint(w, `<html>login first</html>`)
				return
			}
			w.Write(torrent)
		}
	}))
	return srv, &queries
}

func TestEngineIndexerFlow(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "k.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	torrent, hash := bencodeTorrent("The.Matrix.1999.1080p.BluRay.x264.HUN-KN")
	site, queries := fakePrivateSite(t, torrent)
	defer site.Close()
	lib, _ := st.CreateLibrary("Filmek", filepath.Join(dir, "Movies"), "movies")
	if err := st.SaveAcqDefinition(&store.AcqDefinition{ID: "fakeprivate", Name: "Fake Private", Type: "private", YAML: fmt.Sprintf(engineDef, site.URL)}); err != nil {
		t.Fatal(err)
	}
	ix := &store.AcqIndexer{Name: "Fake Private", Kind: "cardigann", DefinitionID: "fakeprivate", Config: `{"username":"kinora","password":"rossz"}`, Enabled: true, Priority: 1}
	st.SaveAcqIndexer(ix)
	st.SaveAcqClient(&store.AcqClient{Name: "qB", URL: "http://qb", Enabled: true})
	st.SetSetting("acq_enabled", "1")
	m := New(st, fakeMovies{}, func(int64) {}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	m.hc = site.Client()
	mem := &memClient{st: map[string]TorrentStatus{}}
	m.NewClient = func(ClientConfig) (DownloadClient, error) { return mem, nil }
	ctx := context.Background()

	// a wrong password: the site's error message comes back
	if _, err := m.TestIndexer(ctx, Indexer{Kind: "cardigann", DefinitionID: "fakeprivate", Config: ix.Config}); err == nil || !strings.Contains(err.Error(), "Rossz jelszó") {
		t.Fatalf("wrong password: %v", err)
	}
	ix.Config = `{"username":"kinora","password":"pw"}`
	st.SaveAcqIndexer(ix)
	if name, err := m.TestIndexer(ctx, Indexer{Kind: "cardigann", DefinitionID: "fakeprivate", Config: ix.Config}); err != nil || !strings.HasPrefix(name, "Fake Private") {
		t.Fatalf("test: %q %v", name, err)
	}

	rs, err := m.Releases(ctx, 603)
	if err != nil || len(rs.Errors) != 0 || len(rs.Results) != 2 {
		t.Fatalf("releases: %+v %v", rs, err)
	}
	if rs.Results[0].Title != "The.Matrix.1999.1080p.BluRay.x264.HUN-KN" || rs.Results[0].Rejected != "" || rs.Results[0].Indexer != "Fake Private" {
		t.Errorf("ranking: %+v", rs.Results[0])
	}
	if rs.Results[1].Rejected == "" {
		t.Errorf("Reloaded should be rejected for the wrong title: %+v", rs.Results[1])
	}
	if q := (*queries)[len(*queries)-1]; !strings.Contains(q, "imdb=tt0133093") || !strings.Contains(q, "q=The+Matrix+1999") {
		t.Errorf("query: %s", q)
	}

	g, err := m.Grab(ctx, GrabInput{TmdbID: 603, LibraryID: lib.ID})
	if err != nil {
		t.Fatal(err)
	}
	if g.InfoHash != hash || mem.adds != 1 {
		t.Errorf("grab: %+v adds %d", g, mem.adds)
	}
}

func TestRowLanguageCountsAsHungarian(t *testing.T) {
	res := rowsToResults(Indexer{ID: 1, Name: "nCore"}, []cardigann.Row{{Title: "The.Matrix.1999.1080p.BluRay.x264-KN", Download: "http://x/dl", Language: "hun"}})
	if len(res) != 1 || !containsStr(res[0].Release.Languages, "hun") {
		t.Fatalf("languages: %+v", res)
	}
	p := DefaultProfile()
	withHun, _ := p.Score(res[0].Release, 8<<30, 10)
	res[0].Release.Languages = nil
	without, _ := p.Score(res[0].Release, 8<<30, 10)
	if withHun <= without {
		t.Errorf("the Hungarian category should rank higher: %d vs %d", withHun, without)
	}
}
