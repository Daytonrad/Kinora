package acquire

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ScotyLucas/kinora/internal/meta"
	"github.com/ScotyLucas/kinora/internal/store"
)

func TestParseRelease(t *testing.T) {
	cases := []struct {
		in   string
		want Release
	}{
		{"The.Matrix.1999.1080p.BluRay.DTS.HUN.ENG.x264-GRP", Release{Title: "The Matrix", Year: 1999, Resolution: 1080, Source: "bluray", Codec: "h264", Audio: "dts", Languages: []string{"hun", "eng"}, Group: "GRP"}},
		{"Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX", Release{Title: "Dune Part Two", Year: 2024, Resolution: 2160, Source: "webdl", Codec: "hevc", HDR: "dv", Audio: "atmos", Group: "FLUX"}},
		{"Blade Runner 2049 (2017) 720p WEBRip x265", Release{Title: "Blade Runner", Year: 2049, Resolution: 720, Source: "webrip", Codec: "hevc"}},
		{"2001.A.Space.Odyssey.1968.REMASTERED.1080p.BluRay.REMUX.AVC.TrueHD-X", Release{Title: "2001 A Space Odyssey", Year: 1968, Resolution: 1080, Source: "remux", Codec: "h264", Audio: "truehd", Group: "X"}},
		{"Some.Movie.2023.HDCAM.x264-BAD", Release{Title: "Some Movie", Year: 2023, Source: "cam", Codec: "h264", Group: "BAD"}},
		{"Tsunami.2009.PROPER.1080p.BluRay.x264-GRP", Release{Title: "Tsunami", Year: 2009, Resolution: 1080, Source: "bluray", Codec: "h264", Group: "GRP", Proper: true}},
		{"Kiddo.2023.HUN.1080p.WEB-DL.AAC2.0.H.264", Release{Title: "Kiddo", Year: 2023, Resolution: 1080, Source: "webdl", Codec: "h264", Audio: "aac", Languages: []string{"hun"}}},
	}
	for _, c := range cases {
		got := ParseRelease(c.in)
		c.want.Season, c.want.Episode = -1, -1
		if fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("ParseRelease(%q)\n got  %+v\n want %+v", c.in, got, c.want)
		}
	}
	if r := ParseRelease("Show.S02E05.1080p.WEB.h264-GRP"); r.Season != 2 || r.Episode != 5 || r.Title != "Show" {
		t.Errorf("episode: %+v", r)
	}
	if r := ParseRelease("Show.S03.1080p.BluRay.x264-GRP"); r.Season != 3 || r.Episode != -1 {
		t.Errorf("season pack: %+v", r)
	}
}

func TestTitleMatches(t *testing.T) {
	ok := []struct {
		name   string
		year   int
		titles []string
	}{
		{"Blade.Runner.2049.2017.1080p.BluRay.x264", 2017, []string{"Blade Runner 2049"}},
		{"The.Matrix.1999.1080p", 1999, []string{"Mátrix", "The Matrix"}},
		{"Matrix.1999.720p", 1999, []string{"The Matrix"}},
		{"Amelie.2001.1080p", 2001, []string{"Le Fabuleux Destin d'Amélie Poulain", "Amélie"}},
		{"Oppenheimer.2023.1080p", 2024, []string{"Oppenheimer"}}, // a year off is fine
	}
	for _, c := range ok {
		if !TitleMatches(c.name, ParseRelease(c.name), c.year, c.titles...) {
			t.Errorf("should match: %s %v", c.name, c.titles)
		}
	}
	bad := []struct {
		name   string
		year   int
		titles []string
	}{
		{"The.Matrix.Reloaded.2003.1080p", 1999, []string{"The Matrix"}},
		{"Aliens.1986.1080p", 1979, []string{"Alien"}},
		{"Something.Else.1999.1080p", 1999, []string{"The Matrix"}},
	}
	for _, c := range bad {
		if TitleMatches(c.name, ParseRelease(c.name), c.year, c.titles...) {
			t.Errorf("should not match: %s %v", c.name, c.titles)
		}
	}
}

func TestScore(t *testing.T) {
	p := DefaultProfile()
	gb := int64(1 << 30)
	s1080, rej := p.Score(ParseRelease("A.2020.1080p.BluRay.x264"), 8*gb, 20)
	if rej != "" {
		t.Fatal(rej)
	}
	s2160, _ := p.Score(ParseRelease("A.2020.2160p.BluRay.x265"), 20*gb, 20)
	sHun, _ := p.Score(ParseRelease("A.2020.1080p.WEB-DL.HUN.x264"), 6*gb, 20)
	if !(s1080 > s2160) {
		t.Errorf("1080p preferred over 2160p: %d vs %d", s1080, s2160)
	}
	if !(sHun > s1080) {
		t.Errorf("Hungarian preferred: %d vs %d", sHun, s1080)
	}
	if _, rej := p.Score(ParseRelease("A.2020.HDCAM"), 2*gb, 50); rej != RejectResolution && rej != RejectSource {
		t.Errorf("cam not rejected: %q", rej)
	}
	if _, rej := p.Score(ParseRelease("A.2020.1080p.CAM"), 2*gb, 50); rej != RejectSource {
		t.Errorf("1080p cam: %q", rej)
	}
	if _, rej := p.Score(ParseRelease("A.2020.1080p.BluRay"), 80*gb, 50); rej != RejectSize {
		t.Errorf("too big: %q", rej)
	}
	if _, rej := p.Score(ParseRelease("A.2020.1080p.BluRay"), 8*gb, 0); rej != RejectSeeders {
		t.Errorf("no seeders: %q", rej)
	}
	p.RequireHungarian = true
	if _, rej := p.Score(ParseRelease("A.2020.1080p.BluRay"), 8*gb, 10); rej != RejectLanguage {
		t.Errorf("require hun: %q", rej)
	}
	if q := ParseProfile(`{"resolutions":[720],"max_size_gb":4}`); q.Resolutions[0] != 720 || q.MaxSizeGB != 4 || len(q.BlockedSources) == 0 {
		t.Errorf("profile: %+v", q)
	}
	if q := ParseProfile("{broken"); len(q.Resolutions) != 3 {
		t.Errorf("broken profile → default: %+v", q)
	}
}

// ---------------------------------------------------------------- torrents

func bencodeTorrent(name string) ([]byte, string) {
	info := fmt.Sprintf("d6:lengthi1024e4:name%d:%s12:piece lengthi16384e6:pieces20:%se", len(name), name, strings.Repeat("x", 20))
	sum := sha1.Sum([]byte(info))
	t := "d8:announce20:http://tracker/x/ann7:comment4:test4:info" + info + "e"
	return []byte(t), hex.EncodeToString(sum[:])
}

func TestHashes(t *testing.T) {
	b, want := bencodeTorrent("Big.Buck.Bunny.2008.1080p.mkv")
	got, err := InfoHash(b)
	if err != nil || got != want {
		t.Fatalf("InfoHash = %q, %v; want %q", got, err, want)
	}
	if _, err := InfoHash([]byte("<html>")); err == nil {
		t.Error("html accepted as torrent")
	}
	h, ok := MagnetHash("magnet:?xt=urn:btih:DD8255ECDC7CA55FB0BBF81323D87062DB1F6D1C&dn=Big+Buck+Bunny")
	if !ok || h != "dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c" {
		t.Errorf("hex magnet: %q %v", h, ok)
	}
	h, ok = MagnetHash("magnet:?xt=urn:btih:3WBFL3G4PSSV7MF37AJSHWDQMLNR63I4")
	if !ok || h != "dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c" {
		t.Errorf("base32 magnet: %q %v", h, ok)
	}
	if _, ok := MagnetHash("http://x"); ok {
		t.Error("http accepted as magnet")
	}
}

func TestFetchRelease(t *testing.T) {
	tor, hash := bencodeTorrent("Movie.2020.1080p.mkv")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/file.torrent":
			w.Write(tor)
		case "/magnet":
			http.Redirect(w, r, "magnet:?xt=urn:btih:"+hash, http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	f, err := FetchRelease(ctx, srv.Client(), Result{Link: srv.URL + "/file.torrent"})
	if err != nil || f.InfoHash != hash || len(f.Torrent) == 0 {
		t.Fatalf("torrent: %+v %v", f, err)
	}
	f, err = FetchRelease(ctx, srv.Client(), Result{Link: srv.URL + "/magnet"})
	if err != nil || f.InfoHash != hash || f.Magnet == "" {
		t.Fatalf("redirect to magnet: %+v %v", f, err)
	}
	if _, err := FetchRelease(ctx, srv.Client(), Result{Link: srv.URL + "/missing"}); err == nil {
		t.Error("404 accepted")
	}
	if _, err := FetchRelease(ctx, srv.Client(), Result{Link: "file:///etc/passwd"}); err == nil {
		t.Error("file:// link accepted")
	}
}

// ---------------------------------------------------------------- torznab

const torznabFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:torznab="http://torznab.com/schemas/2015/feed">
<channel><title>Test</title>
<item>
  <title>The.Matrix.1999.1080p.BluRay.x264-GRP</title>
  <guid>https://tracker.example/t/1</guid>
  <link>%[1]s/dl/1.torrent?apikey=secret</link>
  <comments>https://tracker.example/t/1</comments>
  <pubDate>Mon, 02 Jan 2023 15:04:05 +0000</pubDate>
  <size>8589934592</size>
  <enclosure url="%[1]s/dl/1.torrent?apikey=secret" length="8589934592" type="application/x-bittorrent"/>
  <torznab:attr name="seeders" value="42"/>
  <torznab:attr name="peers" value="50"/>
  <torznab:attr name="imdbid" value="0133093"/>
</item>
<item>
  <title>The.Matrix.1999.720p.WEB.HUN.x264-OTHER</title>
  <guid>magnet-guid</guid>
  <link>magnet:?xt=urn:btih:dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c</link>
  <size>2147483648</size>
  <torznab:attr name="seeders" value="3"/>
</item>
</channel></rss>`

func fakeIndexer(t *testing.T, onQuery func(q map[string]string)) *httptest.Server {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/dl/") {
			b, _ := bencodeTorrent("The.Matrix.1999.1080p.BluRay.x264-GRP.mkv")
			w.Write(b)
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/api") {
			http.NotFound(w, r)
			return
		}
		q := map[string]string{}
		for k := range r.URL.Query() {
			q[k] = r.URL.Query().Get(k)
		}
		if onQuery != nil {
			onQuery(q)
		}
		if q["apikey"] != "secret" {
			fmt.Fprint(w, `<?xml version="1.0"?><error code="100" description="Invalid API Key"/>`)
			return
		}
		switch q["t"] {
		case "caps":
			fmt.Fprint(w, `<?xml version="1.0"?><caps><server title="Fake" version="1.0"/></caps>`)
		default:
			fmt.Fprintf(w, torznabFeed, srv.URL)
		}
	}))
	return srv
}

func TestTorznab(t *testing.T) {
	var last map[string]string
	srv := fakeIndexer(t, func(q map[string]string) { last = q })
	defer srv.Close()
	tz := &Torznab{HC: srv.Client()}
	ctx := context.Background()
	ix := Indexer{ID: 7, Name: "Fake", URL: srv.URL + "/torznab/", APIKey: "secret"}
	name, err := tz.Caps(ctx, ix)
	if err != nil || name != "Fake 1.0" {
		t.Fatalf("caps: %q %v", name, err)
	}
	res, err := tz.Search(ctx, ix, Query{IMDb: "tt0133093", Title: "The Matrix", Year: 1999})
	if err != nil || len(res) != 2 {
		t.Fatalf("search: %v %v", res, err)
	}
	if last["t"] != "movie" || last["imdbid"] != "0133093" || last["cat"] != "2000" {
		t.Errorf("query: %v", last)
	}
	a, b := res[0], res[1]
	if a.Seeders != 42 || a.Size != 8<<30 || a.IMDb != "tt0133093" || a.Link == "" || a.Quality != "1080p BluRay" || a.Published.Year() != 2023 {
		t.Errorf("first: %+v", a)
	}
	if b.Magnet == "" || b.InfoHash != "dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c" || b.Seeders != 3 {
		t.Errorf("second: %+v", b)
	}
	_, err = tz.Caps(ctx, Indexer{URL: srv.URL, APIKey: "wrong"})
	if err == nil || !strings.Contains(err.Error(), "Invalid API Key") {
		t.Errorf("bad key: %v", err)
	}
	if _, err := tz.Caps(ctx, Indexer{URL: "ftp://x"}); err == nil {
		t.Error("ftp url accepted")
	}
}

// ---------------------------------------------------------------- qBittorrent

type fakeQB struct {
	mu       sync.Mutex
	torrents map[string]map[string]any
	added    []string
	cats     []string
	logins   int
	v5       bool // qBittorrent 5.x answers: 204 / 401, cookie QBT_SID_<port>
}

func (f *fakeQB) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.URL.Path == "/api/v2/auth/login" {
			r.ParseForm()
			if r.Form.Get("username") != "admin" || r.Form.Get("password") != "pw" {
				if f.v5 {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				fmt.Fprint(w, "Fails.")
				return
			}
			f.logins++
			if f.v5 {
				http.SetCookie(w, &http.Cookie{Name: "QBT_SID_8080", Value: "s1", Path: "/"})
				w.WriteHeader(http.StatusNoContent)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "s1", Path: "/"})
			fmt.Fprint(w, "Ok.")
			return
		}
		name := "SID"
		if f.v5 {
			name = "QBT_SID_8080"
		}
		if c, err := r.Cookie(name); err != nil || c.Value != "s1" {
			if f.v5 {
				w.WriteHeader(http.StatusUnauthorized)
			} else {
				w.WriteHeader(http.StatusForbidden)
			}
			return
		}
		switch r.URL.Path {
		case "/api/v2/app/version":
			fmt.Fprint(w, "v5.0.4")
		case "/api/v2/torrents/createCategory":
			r.ParseForm()
			f.cats = append(f.cats, r.Form.Get("category"))
		case "/api/v2/torrents/add":
			r.ParseMultipartForm(1 << 20)
			if u := r.FormValue("urls"); u != "" {
				h, _ := MagnetHash(u)
				f.added = append(f.added, h)
			} else if fl, _, err := r.FormFile("torrents"); err == nil {
				b, _ := io.ReadAll(fl)
				h, _ := InfoHash(b)
				f.added = append(f.added, h)
			}
			fmt.Fprint(w, "Ok.")
		case "/api/v2/torrents/info":
			var out []map[string]any
			for _, h := range strings.Split(r.URL.Query().Get("hashes"), "|") {
				if t, ok := f.torrents[h]; ok {
					out = append(out, t)
				}
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, toJSON(out))
		case "/api/v2/torrents/delete":
			r.ParseForm()
			delete(f.torrents, r.Form.Get("hashes"))
		default:
			http.NotFound(w, r)
		}
	})
}

func toJSON(v []map[string]any) string {
	if v == nil {
		return "[]"
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func TestQBittorrent(t *testing.T) {
	for _, v5 := range []bool{false, true} {
		t.Run(fmt.Sprintf("v5=%v", v5), func(t *testing.T) { testQBittorrent(t, v5) })
	}
}

func testQBittorrent(t *testing.T, v5 bool) {
	f := &fakeQB{torrents: map[string]map[string]any{}, v5: v5}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	ctx := context.Background()
	bad, _ := NewQBittorrent(ClientConfig{URL: srv.URL, Username: "admin", Password: "nope"}, srv.Client())
	if _, err := bad.Test(ctx); err == nil || !strings.Contains(err.Error(), "jelszó") {
		t.Errorf("bad password: %v", err)
	}
	q, err := NewQBittorrent(ClientConfig{URL: srv.URL + "/", Username: "admin", Password: "pw", Category: "kinora"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	v, err := q.Test(ctx)
	if err != nil || v != "qBittorrent v5.0.4" {
		t.Fatalf("test: %q %v", v, err)
	}
	tor, hash := bencodeTorrent("x.mkv")
	if _, err := q.Add(ctx, AddRequest{Fetched: Fetched{Torrent: tor, InfoHash: hash}}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Add(ctx, AddRequest{Fetched: Fetched{Magnet: "magnet:?xt=urn:btih:dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c"}}); err != nil {
		t.Fatal(err)
	}
	if len(f.added) != 2 || f.added[0] != hash || len(f.cats) == 0 || f.cats[0] != "kinora" {
		t.Fatalf("added %v cats %v", f.added, f.cats)
	}
	f.torrents[hash] = map[string]any{"hash": strings.ToUpper(hash), "name": "x", "state": "downloading", "progress": 0.5, "size": 100, "dlspeed": 10, "eta": 8640000, "content_path": "/dl/x.mkv"}
	st, err := q.Status(ctx, []string{hash, "missing"})
	if err != nil {
		t.Fatal(err)
	}
	s := st[hash]
	if s.Progress != 0.5 || s.Done || s.ETA != -1 || s.ContentPath != "/dl/x.mkv" {
		t.Errorf("status: %+v", s)
	}
	if _, ok := st["missing"]; ok {
		t.Error("missing torrent reported")
	}
	f.torrents[hash]["state"], f.torrents[hash]["progress"] = "stoppedUP", 1
	st, _ = q.Status(ctx, []string{hash})
	if !st[hash].Done {
		t.Errorf("finished not done: %+v", st[hash])
	}
	// an expired session logs in again
	q.mu.Lock()
	q.hc.Jar.SetCookies(mustURL(srv.URL), []*http.Cookie{{Name: "SID", Value: "old", Path: "/"}, {Name: "QBT_SID_8080", Value: "old", Path: "/"}})
	q.mu.Unlock()
	if err := q.Remove(ctx, hash, false); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.torrents[hash]; ok || f.logins != 2 {
		t.Errorf("remove after relogin: logins %d", f.logins)
	}
}

// ---------------------------------------------------------------- import

func write(t *testing.T, p string, size int) {
	t.Helper()
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(strings.Repeat("v", size)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestImport(t *testing.T) {
	dir := t.TempDir()
	dl := filepath.Join(dir, "downloads", "The.Matrix.1999.1080p.BluRay.x264-GRP")
	write(t, filepath.Join(dl, "the.matrix.1999.1080p.bluray.x264-grp.mkv"), 5000)
	write(t, filepath.Join(dl, "Sample", "sample.mkv"), 9000)
	write(t, filepath.Join(dl, "the.matrix.1999.1080p.bluray.x264-grp.hun.srt"), 10)
	write(t, filepath.Join(dl, "Subs", "2_English.srt"), 10)
	write(t, filepath.Join(dl, "info.nfo"), 10)
	lib := filepath.Join(dir, "media", "Movies")
	os.MkdirAll(lib, 0o755)
	res, err := Import(ImportMovie{Source: dl, LibRoot: lib, Title: "The Matrix: Reloaded?", Year: 1999, IMDb: "tt0133093", Quality: "1080p BluRay", Mode: "hardlink"})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(lib, "The Matrix Reloaded (1999) {imdb-tt0133093}", "The Matrix Reloaded (1999) - 1080p BluRay.mkv")
	if res.Path != want {
		t.Fatalf("path %q, want %q", res.Path, want)
	}
	a, _ := os.Stat(res.Path)
	b, _ := os.Stat(filepath.Join(dl, "the.matrix.1999.1080p.bluray.x264-grp.mkv"))
	if a.Size() != 5000 || (res.Linked && !os.SameFile(a, b)) {
		t.Errorf("not the main file: %d linked=%v", a.Size(), res.Linked)
	}
	if !res.Linked {
		t.Log("hardlink not possible here: copied")
	}
	base := strings.TrimSuffix(res.Path, ".mkv")
	for _, s := range []string{base + ".hun.srt", base + ".2_English.srt"} {
		if _, err := os.Stat(s); err != nil {
			t.Errorf("subtitle missing: %s", s)
		}
	}
	// importing again changes nothing
	res2, err := Import(ImportMovie{Source: dl, LibRoot: lib, Title: "The Matrix: Reloaded?", Year: 1999, IMDb: "tt0133093", Quality: "1080p BluRay"})
	if err != nil || res2.Path != res.Path {
		t.Errorf("second import: %+v %v", res2, err)
	}
	// a single file download, copy mode
	single := filepath.Join(dir, "downloads", "Other.2020.720p.mp4")
	write(t, single, 3000)
	res3, err := Import(ImportMovie{Source: single, LibRoot: lib, Title: "Other", Year: 2020, Quality: "720p", Mode: "copy"})
	if err != nil || res3.Linked || !strings.HasSuffix(res3.Path, filepath.Join("Other (2020)", "Other (2020) - 720p.mp4")) {
		t.Errorf("single: %+v %v", res3, err)
	}
	if _, err := Import(ImportMovie{Source: filepath.Join(dir, "nope"), LibRoot: lib, Title: "X"}); err == nil {
		t.Error("missing source accepted")
	}
	empty := filepath.Join(dir, "downloads", "empty")
	write(t, filepath.Join(empty, "readme.txt"), 5)
	if _, err := Import(ImportMovie{Source: empty, LibRoot: lib, Title: "X"}); err == nil {
		t.Error("folder without video accepted")
	}
}

func TestMapPath(t *testing.T) {
	got := MapPath("/downloads/kinora/Movie/movie.mkv", "/downloads", "/data/torrents")
	if got != filepath.Join("/data/torrents", "kinora", "Movie", "movie.mkv") {
		t.Errorf("mapped: %q", got)
	}
	if MapPath("/elsewhere/x", "/downloads", "/data") != "/elsewhere/x" {
		t.Error("unrelated path changed")
	}
	if MapPath("/downloadsX/a", "/downloads", "/data") != "/downloadsX/a" {
		t.Error("prefix of a longer folder name mapped")
	}
}

// ---------------------------------------------------------------- the whole flow

type fakeMovies struct{}

func (fakeMovies) Search(ctx context.Context, kind, q string, year int) ([]meta.SearchResult, error) {
	return []meta.SearchResult{{ID: 603, Title: "Mátrix", Year: 1999}, {ID: 604, Title: "Mátrix – Újratöltve", Year: 2003}}, nil
}
func (fakeMovies) Movie(ctx context.Context, id int64) (meta.MovieInfo, error) {
	if id != 603 { // a film no indexer has
		return meta.MovieInfo{ID: id, IMDb: "tt9999999", Title: "Nincs Ilyen Film", OriginalTitle: "No Such Film", EnglishTitle: "No Such Film", Year: 2031}, nil
	}
	return meta.MovieInfo{ID: 603, IMDb: "tt0133093", Title: "Mátrix", OriginalTitle: "The Matrix", EnglishTitle: "The Matrix", Year: 1999}, nil
}
func (fakeMovies) TV(ctx context.Context, id int64) (meta.TVInfo, error) {
	return meta.TVInfo{ID: 1399, IMDb: "tt0944947", TVDB: 121361, Title: "Trónok harca", OriginalTitle: "Game of Thrones", EnglishTitle: "Game of Thrones",
		Year: 2011, Seasons: []meta.TVSeason{{Number: 0, EpisodeCount: 1}, {Number: 1, EpisodeCount: 3}, {Number: 2, EpisodeCount: 2}}}, nil
}
func (fakeMovies) TVSeasonEpisodes(ctx context.Context, id int64, season int) ([]meta.TVEpisode, error) {
	switch season {
	case 1:
		return []meta.TVEpisode{{Season: 1, Episode: 1, AirDate: "2011-04-17"}, {Season: 1, Episode: 2, AirDate: "2011-04-24"}, {Season: 1, Episode: 3, AirDate: "2011-05-01"}}, nil
	case 2:
		return []meta.TVEpisode{{Season: 2, Episode: 1, AirDate: "2012-04-01"}, {Season: 2, Episode: 2, AirDate: "2099-01-01"}}, nil
	}
	return nil, nil
}
func (fakeMovies) TVList(ctx context.Context, list string, page int) ([]meta.SearchResult, error) {
	return []meta.SearchResult{{ID: 1399, Title: "Trónok harca", Year: 2011}}, nil
}
func (fakeMovies) MovieList(ctx context.Context, list string, page int) ([]meta.SearchResult, error) {
	return []meta.SearchResult{{ID: 603, Title: "Mátrix", Year: 1999}, {ID: 777, Title: "Nincs Ilyen Film", Year: 2031}}, nil
}

func (fakeMovies) Browse(ctx context.Context, b meta.BrowseFilter) (meta.BrowsePage, error) {
	if b.Kind == "series" {
		return meta.BrowsePage{Results: []meta.SearchResult{{ID: 1399, Title: "Trónok harca", Year: 2011}}, Page: 1, TotalPages: 1, Total: 1}, nil
	}
	return meta.BrowsePage{Results: []meta.SearchResult{{ID: 603, Title: "Mátrix", Year: 1999}}, Page: b.Page, TotalPages: 3, Total: 60}, nil
}
func (fakeMovies) WatchProviders(ctx context.Context, kind string) ([]meta.WatchProvider, error) {
	return []meta.WatchProvider{{ID: 8, Name: "Netflix"}}, nil
}
func (fakeMovies) Genres(ctx context.Context, kind string) ([]meta.Genre, error) {
	return []meta.Genre{{ID: 878, Name: "Sci-Fi"}}, nil
}
func (fakeMovies) TitleProviders(ctx context.Context, kind string, id int64) (meta.WhereToWatch, error) {
	return meta.WhereToWatch{Region: "HU", Stream: []meta.WatchProvider{{ID: 8, Name: "Netflix"}}}, nil
}
func (fakeMovies) WatchRegion() string { return "HU" }

type memClient struct {
	mu   sync.Mutex
	st   map[string]TorrentStatus
	adds int
	reqs []AddRequest // category and folder of each download
}

func (c *memClient) Test(ctx context.Context) (string, error) { return "mem", nil }
func (c *memClient) Protocol() string                         { return "torrent" }
func (c *memClient) Add(ctx context.Context, r AddRequest) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.adds++
	c.reqs = append(c.reqs, r)
	c.st[r.InfoHash] = TorrentStatus{Hash: r.InfoHash, Progress: 0.1, State: "downloading", ETA: 60}
	return r.InfoHash, nil
}
func (c *memClient) Status(ctx context.Context, hs []string) (map[string]TorrentStatus, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[string]TorrentStatus{}
	for _, h := range hs {
		if s, ok := c.st[h]; ok {
			out[h] = s
		}
	}
	return out, nil
}
func (c *memClient) Remove(ctx context.Context, h string, files bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.st, h)
	return nil
}

func TestManagerFlow(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "k.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	idx := fakeIndexer(t, nil)
	defer idx.Close()
	lib := filepath.Join(dir, "Movies")
	os.MkdirAll(lib, 0o755)
	libRow, err := st.CreateLibrary("Filmek", lib, "movies")
	if err != nil {
		t.Fatal(err)
	}
	st.SaveAcqIndexer(&store.AcqIndexer{Name: "Fake", URL: idx.URL, APIKey: "secret", Enabled: true, Priority: 1})
	st.SaveAcqClient(&store.AcqClient{Name: "qB", URL: "http://qb", Enabled: true, PathRemote: "/downloads", PathLocal: filepath.Join(dir, "dl")})
	var scanned []int64
	m := New(st, fakeMovies{}, func(id int64) { scanned = append(scanned, id) }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	m.hc, m.tz = idx.Client(), &Torznab{HC: idx.Client()}
	mem := &memClient{st: map[string]TorrentStatus{}}
	m.NewClient = func(ClientConfig) (DownloadClient, error) { return mem, nil }
	ctx := context.Background()

	if _, err := m.Grab(ctx, GrabInput{TmdbID: 603, LibraryID: libRow.ID}); err != ErrDisabled {
		t.Fatalf("disabled: %v", err)
	}
	st.SetSetting("acq_enabled", "1")

	look, err := m.Lookup(ctx, "matrix", 0)
	if err != nil || len(look) != 2 || look[0].InLibrary {
		t.Fatalf("lookup: %+v %v", look, err)
	}
	rs, err := m.Releases(ctx, 603)
	if err != nil || len(rs.Results) != 2 || len(rs.Errors) != 0 {
		t.Fatalf("releases: %+v %v", rs, err)
	}
	// the Hungarian 720p one has too few seeders? no: min 1 → accepted, Hungarian bonus wins over 1080p
	if rs.Results[0].Rejected != "" || rs.Results[1].Rejected != "" {
		t.Fatalf("rejections: %+v", rs.Results)
	}
	if !strings.Contains(rs.Results[0].Title, "HUN") {
		t.Errorf("Hungarian release should rank first: %s", rs.Results[0].Title)
	}

	g, err := m.Grab(ctx, GrabInput{TmdbID: 603, LibraryID: libRow.ID, UserID: 0})
	if err != nil {
		t.Fatal(err)
	}
	if g.InfoHash != "dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c" || g.Title != "Mátrix" || mem.adds != 1 || mem.reqs[0].Category != "" {
		t.Fatalf("grab: %+v adds %d", g, mem.adds)
	}
	if g2, _ := m.Grab(ctx, GrabInput{TmdbID: 603, LibraryID: libRow.ID}); g2 == nil || g2.ID != g.ID || mem.adds != 1 {
		t.Error("the same torrent was added twice")
	}
	look, _ = m.Lookup(ctx, "matrix", 0)
	if look[0].Grab != store.GrabDownloading {
		t.Errorf("lookup grab state: %q", look[0].Grab)
	}

	m.Poll(ctx)
	g, _ = st.AcqGrab(g.ID)
	if g.Status != store.GrabDownloading || g.Progress != 0.1 || g.ETA != 60 {
		t.Fatalf("progress: %+v", g)
	}

	// finished: the client reports its own path, mapped to where Kinora sees it
	write(t, filepath.Join(dir, "dl", "The.Matrix.1999.720p.WEB.HUN.x264-OTHER", "matrix.mkv"), 4000)
	mem.st[g.InfoHash] = TorrentStatus{Hash: g.InfoHash, Progress: 1, Done: true, State: "stalledUP", ContentPath: "/downloads/The.Matrix.1999.720p.WEB.HUN.x264-OTHER"}
	m.Poll(ctx)
	g, _ = st.AcqGrab(g.ID)
	if g.Status != store.GrabDone || !strings.HasSuffix(g.TargetPath, filepath.Join("Mátrix (1999) {imdb-tt0133093}", "Mátrix (1999) - 720p WEBRip.mkv")) {
		t.Fatalf("import: %+v", g)
	}
	if len(scanned) != 1 || scanned[0] != libRow.ID {
		t.Errorf("scan not started: %v", scanned)
	}

	// a download that vanished from the client fails; retry brings it back to the follow list
	g3 := &store.AcqGrab{Kind: "movie", TmdbID: 604, Title: "X", ReleaseTitle: "X", InfoHash: "ffff", ClientID: g.ClientID, LibraryID: &libRow.ID}
	st.AddAcqGrab(g3)
	m.Poll(ctx)
	if g3, _ = st.AcqGrab(g3.ID); g3.Status != store.GrabDownloading {
		t.Fatalf("a just added download is not gone yet: %+v", g3)
	}
	st.DB.Exec(`UPDATE acq_grabs SET created_at=created_at-600 WHERE id=?`, g3.ID)
	m.Poll(ctx)
	g3, _ = st.AcqGrab(g3.ID)
	if g3.Status != store.GrabFailed || g3.Error == "" {
		t.Errorf("vanished: %+v", g3)
	}
	if err := m.Retry(ctx, g3.ID); err != nil {
		t.Error(err)
	}
	if err := m.Remove(ctx, g.ID, false); err != nil {
		t.Error(err)
	}
	if g, _ = st.AcqGrab(g.ID); g.Status != store.GrabRemoved {
		t.Errorf("removed: %+v", g)
	}
}

func mustURL(s string) *url.URL { u, _ := url.Parse(s); return u }

func TestRequests(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "k.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	idx := fakeIndexer(t, nil)
	defer idx.Close()
	lib := filepath.Join(dir, "Movies")
	os.MkdirAll(lib, 0o755)
	libRow, _ := st.CreateLibrary("Filmek", lib, "movies")
	st.SaveAcqIndexer(&store.AcqIndexer{Name: "Fake", URL: idx.URL, APIKey: "secret", Enabled: true, Priority: 1})
	st.SaveAcqClient(&store.AcqClient{Name: "qB", URL: "http://qb", Enabled: true, PathRemote: "/downloads", PathLocal: filepath.Join(dir, "dl")})
	m := New(st, fakeMovies{}, func(int64) {}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	m.hc, m.tz = idx.Client(), &Torznab{HC: idx.Client()}
	mem := &memClient{st: map[string]TorrentStatus{}}
	m.NewClient = func(ClientConfig) (DownloadClient, error) { return mem, nil }
	ctx := context.Background()
	st.SetSetting("acq_enabled", "1")
	mk := func(name string, admin bool) int64 {
		u, err := st.CreateUser(name, "secret123", admin)
		if err != nil {
			t.Fatal(err)
		}
		return u.ID
	}
	adminID, anna, bela, cili := mk("admin", true), mk("anna", false), mk("bela", false), mk("cili", false)
	user := Requester{UserID: anna}

	if _, err := m.Request(ctx, Requester{UserID: anna, Kids: true}, 603, 0); err != ErrKids {
		t.Errorf("kids: %v", err)
	}
	q, err := m.Request(ctx, user, 603, 0)
	if err != nil || q.Status != store.ReqPending || q.Title != "Mátrix" {
		t.Fatalf("request: %+v %v", q, err)
	}
	if q2, _ := m.Request(ctx, Requester{UserID: bela}, 603, 0); q2 == nil || q2.ID != q.ID {
		t.Error("a second request for the same film should return the open one")
	}
	st.SetSetting("acq_quota_week", "1")
	if _, err := m.Request(ctx, user, 777, 0); err != ErrQuota {
		t.Errorf("quota: %v", err)
	}
	if _, err := m.Request(ctx, Requester{UserID: adminID, IsAdmin: true}, 777, 0); err != nil {
		t.Errorf("admins have no quota: %v", err)
	}
	st.SetSetting("acq_quota_week", "0")
	ds, err := m.Discover(ctx, "trending")
	if err != nil || ds[0].Request != store.ReqPending || ds[1].Request != store.ReqApproved {
		t.Errorf("discover marks: %+v %v", ds, err)
	}

	// pending: nothing happens until an admin approves
	m.ProcessRequests(ctx)
	if mem.adds != 0 {
		t.Fatal("a pending request was downloaded")
	}
	if err := m.Decide(q.ID, adminID, true, ""); err != nil {
		t.Fatal(err)
	}
	m.ProcessRequests(ctx)
	q, _ = st.AcqRequest(q.ID)
	if q.Status != store.ReqDownloading || mem.adds != 1 {
		t.Fatalf("approved request not grabbed: %+v adds %d", q, mem.adds)
	}
	other, _ := st.OpenRequestFor("movie", 777)
	if other.Status != store.ReqWaiting || other.NextTryAt <= time.Now().Unix() || other.Note == "" {
		t.Errorf("no release → waiting: %+v", other)
	}
	grabs, _ := st.AcqGrabs(10)
	first := grabs[0]
	if first.RequestID == nil || *first.RequestID != q.ID || !strings.Contains(first.ReleaseTitle, "HUN") {
		t.Fatalf("grab: %+v", first)
	}

	// the client reports an error: blocklisted, the next best release goes
	mem.st[first.InfoHash] = TorrentStatus{Hash: first.InfoHash, State: "error", Failed: true}
	m.Poll(ctx)
	if !st.Blocklisted([]string{first.InfoHash})[first.InfoHash] {
		t.Error("failed release not blocklisted")
	}
	q, _ = st.AcqRequest(q.ID)
	if q.Status != store.ReqApproved || q.Attempts != 1 {
		t.Fatalf("after a failure: %+v", q)
	}
	m.ProcessRequests(ctx)
	grabs, _ = st.AcqGrabs(10)
	second := grabs[0]
	if second.ID == first.ID || second.InfoHash == first.InfoHash || !strings.Contains(second.ReleaseTitle, "1080p") {
		t.Fatalf("next release: %+v", second)
	}

	// finished: imported, the request is available and the requester is told once
	write(t, filepath.Join(dir, "dl", "M", "matrix.mkv"), 4000)
	mem.st[second.InfoHash] = TorrentStatus{Hash: second.InfoHash, Progress: 1, Done: true, State: "uploading", ContentPath: "/downloads/M"}
	m.Poll(ctx)
	q, _ = st.AcqRequest(q.ID)
	if q.Status != store.ReqAvailable {
		t.Fatalf("not available: %+v", q)
	}
	un, _ := st.UnseenAvailable(anna)
	if len(un) != 1 {
		t.Errorf("unseen: %d", len(un))
	}
	st.MarkRequestsSeen(anna)
	if un, _ = st.UnseenAvailable(anna); len(un) != 0 {
		t.Error("still unseen")
	}

	// cancel: only own, only before the download
	r3, _ := m.Request(ctx, Requester{UserID: cili}, 778, 0)
	if err := m.CancelRequest(r3.ID, Requester{UserID: anna}); err != ErrNotYours {
		t.Errorf("cancel someone else's: %v", err)
	}
	if err := m.CancelRequest(r3.ID, Requester{UserID: cili}); err != nil {
		t.Errorf("cancel: %v", err)
	}
	_ = libRow
}

func TestParseEpisodes(t *testing.T) {
	r := ParseRelease("Game.of.Thrones.S01E01E02.1080p.BluRay.x264-GRP")
	if r.Season != 1 || fmt.Sprint(r.Episodes) != "[1 2]" || r.Title != "Game of Thrones" {
		t.Errorf("multi: %+v", r)
	}
	if r := ParseRelease("Game.of.Thrones.S02.1080p.WEB-DL"); r.Season != 2 || r.Episode != -1 || len(r.Episodes) != 0 {
		t.Errorf("pack: %+v", r)
	}
}

// a fake TV indexer: a season 1 pack, single S02E01, and an RSS feed
func fakeTVIndexer(t *testing.T) *httptest.Server {
	item := func(title, magnetHash string, seeders int) string {
		return fmt.Sprintf(`<item><title>%s</title><guid>%s</guid><link>magnet:?xt=urn:btih:%s</link><size>%d</size>
<torznab:attr name="seeders" value="%d"/></item>`, title, magnetHash, magnetHash, int64(3)<<30, seeders)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		var items string
		switch {
		case q.Get("t") == "tvsearch" && q.Get("season") == "1" && q.Get("ep") == "":
			items = item("Game.of.Thrones.S01.1080p.BluRay.x264-PACK", "1111111111111111111111111111111111111111", 50) +
				item("Game.of.Thrones.S01E01.720p.HDTV.x264-ONE", "2222222222222222222222222222222222222222", 90)
		case q.Get("t") == "tvsearch" && q.Get("season") == "2" && q.Get("ep") == "1":
			items = item("Game.of.Thrones.S02E01.1080p.WEB.H264-EP", "3333333333333333333333333333333333333333", 40) +
				item("House.of.the.Dragon.S02E01.1080p.WEB.H264-NOPE", "4444444444444444444444444444444444444444", 99)
		}
		fmt.Fprintf(w, `<?xml version="1.0"?><rss xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel>%s</channel></rss>`, items)
	}))
}

func TestSeries(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "k.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	idx := fakeTVIndexer(t)
	defer idx.Close()
	shows := filepath.Join(dir, "Shows")
	os.MkdirAll(shows, 0o755)
	lib, _ := st.CreateLibrary("Sorozatok", shows, "shows")
	st.SaveAcqIndexer(&store.AcqIndexer{Name: "TV", URL: idx.URL, Enabled: true, Priority: 1})
	st.SaveAcqClient(&store.AcqClient{Name: "qB", URL: "http://qb", Enabled: true, Category: "kinora-movies", SavePath: "/downloads/movies",
		CategoryTV: "kinora-tv", SavePathTV: "/downloads/tv", PathRemote: "/downloads", PathLocal: filepath.Join(dir, "dl")})
	var scanned int
	m := New(st, fakeMovies{}, func(int64) { scanned++ }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	m.hc, m.tz = idx.Client(), &Torznab{HC: idx.Client()}
	mem := &memClient{st: map[string]TorrentStatus{}}
	m.NewClient = func(ClientConfig) (DownloadClient, error) { return mem, nil }
	m.lastRSS = time.Now() // no RSS in this test
	ctx := context.Background()
	st.SetSetting("acq_enabled", "1")
	admin, _ := st.CreateUser("admin", "secret123", true)

	x, err := m.AddSeries(ctx, SeriesInput{TmdbID: 1399, Monitor: MonitorAll, UserID: admin.ID})
	if err != nil {
		t.Fatal(err)
	}
	if x.Total != 5 || x.Wanted != 4 || *x.LibraryID != lib.ID || x.NextAir != "2099-01-01" {
		t.Fatalf("followed: %+v", x)
	}
	m.ProcessSeries(ctx)
	if mem.adds != 2 {
		t.Fatalf("expected the season 1 pack and S02E01, got %d downloads", mem.adds)
	}
	for _, r := range mem.reqs { // series go to their own category and folder, not the films'
		if r.Category != "kinora-tv" || r.SavePath != "/downloads/tv" {
			t.Errorf("series download sent to %q %q", r.Category, r.SavePath)
		}
	}
	grabs, _ := st.AcqGrabs(10)
	var pack, single *store.AcqGrab
	for _, g := range grabs {
		switch g.Kind {
		case "season":
			pack = g
		case "episode":
			single = g
		}
	}
	if pack == nil || pack.Season != 1 || pack.Episodes != "" || !strings.Contains(pack.ReleaseTitle, "PACK") {
		t.Fatalf("pack: %+v", pack)
	}
	if single == nil || single.Season != 2 || single.Episodes != "1" || !strings.Contains(single.ReleaseTitle, "-EP") {
		t.Fatalf("episode: %+v", single)
	}
	x, _ = st.AcqSeriesByID(x.ID)
	if x.Wanted != 0 || x.Downloading != 4 {
		t.Errorf("while downloading: %+v", x)
	}
	m.ProcessSeries(ctx) // nothing new to search
	if mem.adds != 2 {
		t.Errorf("searched again while downloading: %d", mem.adds)
	}

	// both finish: the pack holds three episodes (and a sample), the single one file
	pd := filepath.Join(dir, "dl", "Game.of.Thrones.S01.1080p.BluRay.x264-PACK")
	for i := 1; i <= 3; i++ {
		write(t, filepath.Join(pd, fmt.Sprintf("Game.of.Thrones.S01E%02d.1080p.BluRay.x264-PACK.mkv", i)), 2000+i)
	}
	write(t, filepath.Join(pd, "Game.of.Thrones.S01E01.1080p.BluRay.x264-PACK.hun.srt"), 10)
	write(t, filepath.Join(pd, "Sample", "sample.mkv"), 9000)
	write(t, filepath.Join(dir, "dl", "got.s02e01.mkv"), 3000)
	mem.st[pack.InfoHash] = TorrentStatus{Hash: pack.InfoHash, Progress: 1, Done: true, State: "uploading", ContentPath: "/downloads/Game.of.Thrones.S01.1080p.BluRay.x264-PACK"}
	mem.st[single.InfoHash] = TorrentStatus{Hash: single.InfoHash, Progress: 1, Done: true, State: "uploading", ContentPath: "/downloads/got.s02e01.mkv"}
	m.Poll(ctx)
	show := filepath.Join(shows, "Trónok harca (2011)")
	for _, f := range []string{
		filepath.Join(show, "Season 01", "Trónok harca (2011) - S01E01 - 1080p BluRay.mkv"),
		filepath.Join(show, "Season 01", "Trónok harca (2011) - S01E01 - 1080p BluRay.hun.srt"),
		filepath.Join(show, "Season 01", "Trónok harca (2011) - S01E03 - 1080p BluRay.mkv"),
		filepath.Join(show, "Season 02", "Trónok harca (2011) - S02E01 - 1080p WEBRip.mkv"),
	} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("missing %s", f)
		}
	}
	x, _ = st.AcqSeriesByID(x.ID)
	if x.Have != 4 || x.Wanted != 0 || x.Downloading != 0 || scanned != 2 {
		t.Fatalf("after import: %+v scans %d", x, scanned)
	}

	// a request for the series: followed already, everything aired is here → available
	st.SetSetting("acq_auto_approve", "all")
	anna, _ := st.CreateUser("anna", "secret123", false)
	q, err := m.RequestSeries(ctx, Requester{UserID: anna.ID}, 1399, "1", 0)
	if err != nil || q.Status != store.ReqApproved || q.Seasons != "1" || q.Kind != "series" {
		t.Fatalf("series request: %+v %v", q, err)
	}
	m.ProcessRequests(ctx)
	if q, _ = st.AcqRequest(q.ID); q.Status != store.ReqDownloading {
		t.Fatalf("request after follow: %+v", q)
	}
	x, _ = st.AcqSeriesByID(x.ID)
	if x.Monitor != MonitorAll {
		t.Errorf("a request must not narrow what is followed: %s", x.Monitor)
	}
	m.ProcessSeries(ctx)
	if q, _ = st.AcqRequest(q.ID); q.Status != store.ReqAvailable {
		t.Errorf("request not available: %+v", q)
	}
	if _, err := ValidSeasons("1,x"); err != nil {
		t.Errorf("1,x keeps the valid part: %v", err)
	}
	if _, err := ValidSeasons("0"); err == nil {
		t.Error("season 0 accepted")
	}
	// interactive search: a season lists packs and single episodes; one episode only matching releases
	_, rs, err := m.SeriesReleases(ctx, x.ID, 1, -1)
	if err != nil || len(rs) != 2 || rs[0].Rejected != "" || rs[1].Rejected != "" {
		t.Fatalf("season interactive: %+v %v", rs, err)
	}
	_, rs, _ = m.SeriesReleases(ctx, x.ID, 2, 1)
	var hod *Result
	for i := range rs {
		if strings.Contains(rs[i].Title, "House") {
			hod = &rs[i]
		}
	}
	if hod == nil || hod.Rejected != RejectTitle {
		t.Errorf("another series' release not rejected: %+v", hod)
	}
	before := mem.adds
	for _, r := range rs {
		if strings.Contains(r.Title, "-EP") {
			if err := m.GrabSeriesRelease(ctx, x.ID, 2, r); err != nil {
				t.Fatal(err)
			}
		}
	}
	if mem.adds != before+1 { // an admin may send a release again on purpose
		t.Errorf("interactive grab: adds %d → %d", before, mem.adds)
	}
	ds, _ := m.Discover(ctx, "tv_trending")
	if len(ds) != 1 || ds[0].Kind != "series" || !ds[0].Followed {
		t.Errorf("tv discover: %+v", ds)
	}
}

func TestUpgrade(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "k.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	idx := fakeIndexer(t, nil)
	defer idx.Close()
	lib := filepath.Join(dir, "Movies")
	libRow, _ := st.CreateLibrary("Filmek", lib, "movies")
	st.SaveAcqIndexer(&store.AcqIndexer{Name: "Fake", URL: idx.URL, APIKey: "secret", Enabled: true, Priority: 1})
	st.SaveAcqClient(&store.AcqClient{Name: "qB", URL: "http://qb", Enabled: true, PathRemote: "/downloads", PathLocal: filepath.Join(dir, "dl")})
	m := New(st, fakeMovies{}, func(int64) {}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	m.hc, m.tz = idx.Client(), &Torznab{HC: idx.Client()}
	mem := &memClient{st: map[string]TorrentStatus{}}
	m.NewClient = func(ClientConfig) (DownloadClient, error) { return mem, nil }
	ctx := context.Background()
	st.SetSetting("acq_enabled", "1")
	prof := Profile{Resolutions: []int{1080, 720}, MinSeeders: 1, Upgrade: true, CutoffRes: 1080, CutoffSource: "bluray"}
	pr := &store.AcqProfile{Name: "Upgrade", IsDefault: true, JSON: ProfileJSON(prof)}
	if err := st.SaveAcqProfile(pr); err != nil {
		t.Fatal(err)
	}
	if got := m.Profile(); !got.Upgrade || got.CutoffRank() != 108050 {
		t.Fatalf("default profile: %+v", got)
	}

	// a 720p WEB copy Kinora imported earlier
	old := filepath.Join(lib, "Mátrix (1999) {imdb-tt0133093}", "Mátrix (1999) - 720p WEBRip.mkv")
	write(t, old, 1000)
	write(t, strings.TrimSuffix(old, ".mkv")+".hun.srt", 10)
	g0 := &store.AcqGrab{Kind: "movie", TmdbID: 603, ImdbID: "tt0133093", Title: "Mátrix", Year: 1999, LibraryID: &libRow.ID,
		ReleaseTitle: "The.Matrix.1999.720p.WEB.HUN.x264-OTHER", InfoHash: "old", Status: store.GrabDone}
	st.AddAcqGrab(g0)
	st.SetAcqGrabStatus(g0.ID, store.GrabDone, "", old)

	m.ProcessUpgrades(ctx)
	grabs, _ := st.AcqGrabs(10)
	up := grabs[0]
	if mem.adds != 1 || up.ID == g0.ID || up.Replaces != old || !strings.Contains(up.ReleaseTitle, "1080p.BluRay") {
		t.Fatalf("upgrade grab: adds %d %+v", mem.adds, up)
	}
	write(t, filepath.Join(dir, "dl", "M1080", "matrix.1080p.mkv"), 5000)
	mem.st[up.InfoHash] = TorrentStatus{Hash: up.InfoHash, Progress: 1, Done: true, State: "uploading", ContentPath: "/downloads/M1080"}
	m.Poll(ctx)
	up, _ = st.AcqGrab(up.ID)
	if up.Status != store.GrabDone {
		t.Fatalf("upgrade import: %+v", up)
	}
	if _, err := os.Stat(old); err == nil {
		t.Error("the replaced file is still there")
	}
	if _, err := os.Stat(strings.TrimSuffix(old, ".mkv") + ".hun.srt"); err == nil {
		t.Error("the replaced file's subtitle is still there")
	}
	if _, err := os.Stat(up.TargetPath); err != nil {
		t.Errorf("new file missing: %s", up.TargetPath)
	}
	// at the cutoff now: no more searches
	st.DB.Exec(`UPDATE acq_grabs SET upgrade_checked_at=0`)
	m.ProcessUpgrades(ctx)
	if mem.adds != 1 {
		t.Errorf("searched again above the cutoff: %d", mem.adds)
	}
	// never deletes outside the library
	outside := filepath.Join(dir, "elsewhere.mkv")
	write(t, outside, 10)
	removeReplaced(lib, outside, "")
	if _, err := os.Stat(outside); err != nil {
		t.Error("a file outside the library was removed")
	}
}
