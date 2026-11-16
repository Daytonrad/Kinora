package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/scoty_lucas/kinora/internal/media"
	"github.com/scoty_lucas/kinora/internal/meta"
	"github.com/scoty_lucas/kinora/internal/party"
	"github.com/scoty_lucas/kinora/internal/scanner"
	"github.com/scoty_lucas/kinora/internal/store"
)

type client struct {
	t    *testing.T
	base string
	hc   *http.Client
}

func (c *client) do(method, path string, body any, out any) int {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, r)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if out != nil {
		json.Unmarshal(b, out)
	}
	return resp.StatusCode
}

// newTestServer scans the generated test library and returns a logged-in admin client.
func newTestServer(t *testing.T) (*client, *store.Store) {
	t.Helper()
	mediaDir, _ := filepath.Abs("../../testmedia")
	if _, err := os.Stat(filepath.Join(mediaDir, ".complete")); err != nil {
		t.Skip("testmedia missing – run scripts/mkmedia.sh testmedia")
	}
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "k.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	sc := scanner.New(st, filepath.Join(dir, "images"), log)
	enc := media.NewEncoders()
	srv := New(Config{Addr: ":0", DataDir: dir, CacheDir: dir}, st, log, Deps{
		Ctx: context.Background(), Scanner: sc, ImgDir: filepath.Join(dir, "images"),
		Media: media.NewManager(dir, enc, log), Encoders: enc, Subs: media.NewSubtitles(dir), Downloads: media.NewDownloader(dir, enc, log), Party: party.NewHub(log), Meta: meta.New(st, filepath.Join(dir, "images"), log)})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	t.Cleanup(func() { srv.Shutdown(context.Background()) })
	jar, _ := cookiejar.New(nil)
	c := &client{t: t, base: ts.URL, hc: &http.Client{Jar: jar}}
	if code := c.do("POST", "/api/setup", map[string]string{"username": "admin", "password": "secret123"}, nil); code != 200 {
		t.Fatalf("setup %d", code)
	}
	for _, l := range []struct{ name, path, kind string }{{"Filmek", "Movies", "movies"}, {"Sorozatok", "Shows", "shows"}} {
		lib, _ := st.CreateLibrary(l.name, filepath.Join(mediaDir, l.path), l.kind)
		if _, err := sc.ScanNow(context.Background(), lib.ID); err != nil {
			t.Fatal(err)
		}
	}
	return c, st
}

func TestKidsProfileCannotSeeRFilm(t *testing.T) {
	c, st := newTestServer(t)
	var rID int64
	st.DB.QueryRow(`SELECT id FROM items WHERE title='Space Drama'`).Scan(&rID)
	var kid store.Profile
	if code := c.do("POST", "/api/profiles", map[string]any{"name": "Kicsi", "is_kids": true, "max_rating_level": 3}, &kid); code != 201 {
		t.Fatalf("create kid %d", code)
	}
	// even with max level 3 on a kids profile, use a level-2 limit to hide R explicitly
	two := 2
	st.UpdateProfile(kid.ID, store.ProfileInput{MaxRatingLevel: &two})
	if code := c.do("POST", "/api/profiles/"+itoa(kid.ID)+"/select", map[string]string{}, nil); code != 200 {
		t.Fatal("select kid")
	}
	var list struct {
		Items []store.Item `json:"items"`
	}
	c.do("GET", "/api/items?limit=100", nil, &list)
	for _, it := range list.Items {
		if it.ID == rID {
			t.Fatal("R film in kid list")
		}
		if it.Title == "Kids Cartoon" {
			t.Fatal("unrated film in kid list")
		}
	}
	if len(list.Items) == 0 {
		t.Fatal("kid should see the PG-13 movie and TV-PG show")
	}
	if code := c.do("GET", "/api/items/"+itoa(rID), nil, nil); code != 404 {
		t.Fatalf("details: %d", code)
	}
	if code := c.do("GET", "/api/stream/"+itoa(rID), nil, nil); code != 404 {
		t.Fatalf("stream: %d", code)
	}
	if code := c.do("POST", "/api/items/"+itoa(rID)+"/playback", map[string]any{"caps": map[string]any{}}, nil); code != 404 {
		t.Fatalf("playback: %d", code)
	}
	if code := c.do("GET", "/api/items/"+itoa(rID)+"/subtitles/0", nil, nil); code != 404 {
		t.Fatalf("subtitle: %d", code)
	}
	if code := c.do("POST", "/api/items/"+itoa(rID)+"/download", map[string]string{"quality": "360p"}, nil); code != 404 {
		t.Fatalf("download: %d", code)
	}
	if code := c.do("GET", "/api/images/"+itoa(rID)+"/thumb", nil, nil); code != 404 {
		t.Fatalf("image: %d", code)
	}
	var found []store.Item
	c.do("GET", "/api/search?q=space", nil, &found)
	if len(found) != 0 {
		t.Fatal("search leak")
	}
	// kids cannot create collections or tags
	if code := c.do("POST", "/api/collections", map[string]any{"name": "x"}, nil); code != 403 {
		t.Fatalf("kid collection create: %d", code)
	}
	// and the adult profile does see it
	c.do("POST", "/api/profiles/leave", nil, nil)
	var me struct {
		Profiles []store.Profile `json:"profiles"`
	}
	c.do("GET", "/api/me", nil, &me)
	c.do("POST", "/api/profiles/"+itoa(me.Profiles[0].ID)+"/select", map[string]string{}, nil)
	if code := c.do("GET", "/api/items/"+itoa(rID), nil, nil); code != 200 {
		t.Fatalf("adult details: %d", code)
	}
}

func TestPlaybackAPI(t *testing.T) {
	c, st := newTestServer(t)
	var me struct {
		Profiles []store.Profile `json:"profiles"`
	}
	c.do("GET", "/api/me", nil, &me)
	c.do("POST", "/api/profiles/"+itoa(me.Profiles[0].ID)+"/select", map[string]string{}, nil)
	caps := map[string]any{"video": []string{"h264"}, "audio": []string{"aac", "mp3"}, "containers": []string{"mp4"}}
	id := func(title string) string {
		var i int64
		st.DB.QueryRow(`SELECT id FROM items WHERE title=?`, title).Scan(&i)
		return itoa(i)
	}
	var d struct {
		Mode    string   `json:"mode"`
		URL     string   `json:"url"`
		Session string   `json:"session"`
		Reasons []string `json:"reasons"`
		Error   string   `json:"error"`
	}
	c.do("POST", "/api/items/"+id("Big Test Movie")+"/playback", map[string]any{"caps": caps}, &d)
	if d.Mode != "direct" {
		t.Fatalf("direct: %+v", d)
	}
	c.do("POST", "/api/items/"+id("Pilot Part 1")+"/playback", map[string]any{"caps": caps}, &d)
	if d.Mode != "remux" || d.Session == "" {
		t.Fatalf("remux: %+v", d)
	}
	if code := c.do("GET", d.URL, nil, nil); code != 200 {
		t.Fatalf("playlist %d", code)
	}
	if code := c.do("GET", "/api/hls/"+d.Session+"/..%2fk.db", nil, nil); code == 200 {
		t.Fatal("path traversal")
	}
	c.do("POST", "/api/items/"+id("Space Drama")+"/playback", map[string]any{"caps": caps, "prev_session": d.Session}, &d)
	if d.Mode != "transcode" || len(d.Reasons) < 2 {
		t.Fatalf("transcode: %+v", d)
	}
}

func itoa(i int64) string { return json.Number(jsonInt(i)).String() }

func jsonInt(i int64) string { b, _ := json.Marshal(i); return string(b) }
