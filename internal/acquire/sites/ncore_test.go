package sites

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ScotyLucas/kinora/internal/acquire/cardigann"
)

// fakeNCore mimics the pages of ncore.pro the indexer reads.
type fakeNCore struct {
	mu       sync.Mutex
	sessions map[string]bool
	twoFA    bool
	queries  []string
}

const passkey = "0123456789abcdef0123456789abcdef"

func torrentRow(id, cat, title, size, seed, leech, date, imdb string) string {
	return fmt.Sprintf(`<div class="box_torrent">
 <div class="box_alap_img"><a href="/torrents.php?tipus=%s"><img class="categ_link" alt="%s"></a></div>
 <div class="box_nagy"><div class="torrent_txt"><a href="torrents.php?action=details&amp;id=%s" title="%s"><nobr>%s</nobr></a></div>
  <div class="siterank"><a class="infolink" href="https://imdb.com/title/%s/">imdb</a></div></div>
 <div class="box_feltoltve2">%s</div><div class="box_meret2">%s</div><div class="box_d2">7</div>
 <div class="box_s2"><a class="torrent" href="#">%s</a></div><div class="box_l2"><a class="torrent" href="#">%s</a></div>
</div>`, cat, cat, id, title, title, imdb, date, size, seed, leech)
}

func (f *fakeNCore) handler() http.Handler {
	mux := http.NewServeMux()
	signed := func(r *http.Request) bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		c, err := r.Cookie("PHPSESSID")
		return err == nil && f.sessions[c.Value]
	}
	loginPage := `<form action="login.php" method="post"><input name="nev"><input name="pass" type="password"></form>`
	mux.HandleFunc("/login.php", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			fmt.Fprint(w, loginPage)
			return
		}
		r.ParseForm()
		if r.Form.Get("nev") != "anna" || r.Form.Get("pass") != "titok" || r.Form.Get("submitted") != "1" {
			fmt.Fprint(w, `<div class="box_alert_red">Hibás felhasználónév vagy jelszó!</div>`+loginPage)
			return
		}
		if f.twoFA {
			fmt.Fprint(w, `<input name="2factor">`+loginPage)
			return
		}
		f.mu.Lock()
		f.sessions["s1"] = true
		f.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "PHPSESSID", Value: "s1", Path: "/"})
		http.Redirect(w, r, "/index.php", http.StatusFound)
	})
	mux.HandleFunc("/index.php", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "index") })
	mux.HandleFunc("/torrents.php", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("action") == "download" {
			if q.Get("key") != passkey {
				fmt.Fprint(w, "<html>hibás kulcs</html>")
				return
			}
			fmt.Fprintf(w, "d8:announce4:none4:infod4:name%d:%se", len(q.Get("id")), q.Get("id"))
			return
		}
		if !signed(r) {
			http.Redirect(w, r, "/login.php", http.StatusFound)
			return
		}
		f.mu.Lock()
		f.queries = append(f.queries, r.URL.RawQuery)
		f.mu.Unlock()
		fmt.Fprintf(w, `<html><head><link rel="alternate" type="application/rss+xml" href="/rss.php?key=%s"></head><body>`, passkey)
		if strings.Contains(q.Get("kivalasztott_tipus"), "hdser") && !strings.Contains(q.Get("kivalasztott_tipus"), "hd_hun") {
			fmt.Fprint(w, torrentRow("31", "hdser_hun", "Game.of.Thrones.S01.1080p.BluRay.x264.HUN-GRP", "45.3 GiB", "120", "3", "2019-05-20<br>18:00:00", "tt0944947"))
			fmt.Fprint(w, torrentRow("32", "hdser", "Game.of.Thrones.S02E01.1080p.WEB-DL", "2.1 GiB", "15", "0", "2019-05-21<br>18:00:00", "tt0944947"))
		} else {
			fmt.Fprint(w, torrentRow("11", "hd_hun", "The.Matrix.1999.1080p.BluRay.DTS.HUN-KN", "12.5 GiB", "1 234", "5", "2023-01-02<br>10:00:00", "tt0133093"))
			fmt.Fprint(w, torrentRow("12", "hd", "The.Matrix.1999.2160p.UHD.BluRay.x265-ENG", "60 GiB", "40", "1", "2023-01-03<br>11:30:00", "tt0133093"))
		}
		fmt.Fprint(w, `</body></html>`)
	})
	return mux
}

func TestNCore(t *testing.T) {
	site := &fakeNCore{sessions: map[string]bool{}}
	srv := httptest.NewServer(site.handler())
	defer srv.Close()
	ctx := context.Background()
	cfg := func(u, p string) map[string]any { return map[string]any{"username": u, "password": p, "url": srv.URL} }

	if _, err := NewNCore(cfg("anna", "rossz"), srv.Client()).Test(ctx); err == nil || !strings.Contains(err.Error(), "Hibás felhasználónév") {
		t.Errorf("wrong password: %v", err)
	}
	if _, err := NewNCore(map[string]any{"url": srv.URL}, srv.Client()).Test(ctx); err == nil || !strings.Contains(err.Error(), "felhasználónevet") {
		t.Errorf("no credentials: %v", err)
	}
	site.twoFA = true
	if _, err := NewNCore(cfg("anna", "titok"), srv.Client()).Test(ctx); err == nil || !strings.Contains(err.Error(), "kétlépcsős") {
		t.Errorf("two-factor: %v", err)
	}
	site.twoFA = false

	n := NewNCore(cfg("anna", "titok"), srv.Client())
	if c, err := n.Test(ctx); err != nil || c != 2 {
		t.Fatalf("test: %d %v", c, err)
	}
	if n.key != passkey {
		t.Errorf("passkey: %q", n.key)
	}
	rows, err := n.Search(ctx, cardigann.Query{Type: "movie", Keywords: "The Matrix 1999", IMDBID: "tt0133093"})
	if err != nil || len(rows) != 2 {
		t.Fatalf("movie search: %+v %v", rows, err)
	}
	q := site.queries[len(site.queries)-1]
	if !strings.Contains(q, "miben=imdb") || !strings.Contains(q, "mire=tt0133093") || !strings.Contains(q, "hd_hun%2Chd") || strings.Contains(q, "hdser") {
		t.Errorf("movie query: %s", q)
	}
	r := rows[0]
	if r.Title != "The.Matrix.1999.1080p.BluRay.DTS.HUN-KN" || r.Language != "hun" || r.Categories[0] != "Movies/HD" || r.Seeders != 1234 || r.Leechers != 5 ||
		r.Grabs != 7 || r.Size != cardigann.ParseSize("12.5 GiB") || r.IMDb != "tt0133093" || r.Date.Format("2006-01-02 15:04") != "2023-01-02 10:00" ||
		r.Download != srv.URL+"/torrents.php?action=download&id=11&key="+passkey || !strings.HasSuffix(r.Details, "action=details&id=11") {
		t.Errorf("row: %+v", r)
	}
	if rows[1].Language != "" {
		t.Errorf("an English release marked Hungarian: %+v", rows[1])
	}
	// series: by IMDb, only the asked season
	rows, err = n.Search(ctx, cardigann.Query{Type: "tv", Keywords: "Game of Thrones S01", IMDBID: "tt0944947", Season: "1"})
	if err != nil || len(rows) != 1 || rows[0].Categories[0] != "TV/HD" || rows[0].Language != "hun" {
		t.Fatalf("tv search: %+v %v", rows, err)
	}
	// a name search
	n.Search(ctx, cardigann.Query{Type: "movie", Keywords: "Mátrix"})
	if q = site.queries[len(site.queries)-1]; !strings.Contains(q, "miben=name") || !strings.Contains(q, "mire=M%C3%A1trix") {
		t.Errorf("name query: %s", q)
	}
	// the session runs out: signs in again
	site.mu.Lock()
	site.sessions = map[string]bool{}
	site.mu.Unlock()
	if rows, err := n.Search(ctx, cardigann.Query{Type: "movie", Keywords: "x"}); err != nil || len(rows) != 2 {
		t.Errorf("after expiry: %d %v", len(rows), err)
	}
	b, _, err := n.Download(ctx, r.Download)
	if err != nil || !strings.HasPrefix(string(b), "d8:announce") {
		t.Errorf("download: %q %v", b, err)
	}
	if _, _, err := n.Download(ctx, srv.URL+"/torrents.php?action=download&id=11&key=bad"); err == nil {
		t.Error("a page instead of a torrent should fail")
	}
	// a pasted cookie (two-factor accounts) instead of the password
	site.mu.Lock()
	site.sessions["fromcookie"] = true
	site.mu.Unlock()
	c := NewNCore(map[string]any{"cookie": "nick=anna; PHPSESSID=fromcookie", "url": srv.URL}, srv.Client())
	if cnt, err := c.Test(ctx); err != nil || cnt != 2 {
		t.Errorf("cookie login: %d %v", cnt, err)
	}
	if _, err := NewNCore(map[string]any{"cookie": "PHPSESSID=old", "url": srv.URL}, srv.Client()).Test(ctx); err == nil || !strings.Contains(err.Error(), "süti") {
		t.Errorf("stale cookie: %v", err)
	}
}
