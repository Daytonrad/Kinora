package jfcompat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/ScotyLucas/kinora/internal/media"
	"github.com/ScotyLucas/kinora/internal/scanner"
	"github.com/ScotyLucas/kinora/internal/store"
)

type env struct {
	t    *testing.T
	st   *store.Store
	srv  *Server
	ts   *httptest.Server
	user *store.User
	kid  *store.Profile
}

func newEnv(t *testing.T) *env {
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
	for _, l := range []struct{ name, path, kind string }{{"Filmek", "Movies", "movies"}, {"Sorozatok", "Shows", "shows"}} {
		lib, _ := st.CreateLibrary(l.name, filepath.Join(mediaDir, l.path), l.kind)
		if _, err := sc.ScanNow(context.Background(), lib.ID); err != nil {
			t.Fatal(err)
		}
	}
	u, err := st.CreateFirstAdmin("anna", "secret123")
	if err != nil {
		t.Fatal(err)
	}
	two, name, yes, pin := 2, "Kicsi", true, "1234"
	kid, err := st.CreateProfile(u.ID, store.ProfileInput{Name: &name, IsKids: &yes, MaxRatingLevel: &two, PIN: &pin})
	if err != nil {
		t.Fatal(err)
	}
	enc := media.NewEncoders()
	enc.SelfTest(context.Background())
	mgr := media.NewManager(dir, enc, log)
	t.Cleanup(mgr.StopAll)
	srv, err := New(Deps{Store: st, Media: mgr, Encoders: enc, Subs: media.NewSubtitles(dir), ImgDir: filepath.Join(dir, "images"), Log: log})
	if err != nil {
		t.Fatal(err)
	}
	st.SetSetting(SettingEnabled, "1")
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return &env{t: t, st: st, srv: srv, ts: ts, user: u, kid: kid}
}

// call does a request like a client: the MediaBrowser header carries the token.
func (e *env) call(method, path, token string, body any, out any) (int, []byte) {
	e.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.ts.URL+path, r)
	req.Header.Set("Content-Type", "application/json")
	auth := `MediaBrowser Client="Infuse", Device="Apple%20TV", DeviceId="dev1", Version="8.0"`
	if token != "" {
		auth += `, Token="` + token + `"`
	}
	req.Header.Set("Authorization", auth)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if out != nil {
		json.Unmarshal(b, out)
	}
	return resp.StatusCode, b
}

func (e *env) login(user, pw string) string {
	e.t.Helper()
	var res struct {
		AccessToken string
		User        struct{ Name, Id string }
	}
	if code, b := e.call("POST", "/Users/AuthenticateByName", "", map[string]string{"Username": user, "Pw": pw}, &res); code != 200 {
		e.t.Fatalf("login %s: %d %s", user, code, b)
	}
	return res.AccessToken
}

type itemList struct {
	Items []struct {
		Id, Name, Type, SeriesId, ParentId string
		IndexNumber                        int
		UserData                           struct {
			Played                bool
			IsFavorite            bool
			PlaybackPositionTicks int64
		}
		MediaSources []map[string]any
		ImageTags    map[string]string
	}
	TotalRecordCount int
}

func (l itemList) find(name string) string {
	for _, it := range l.Items {
		if it.Name == name {
			return it.Id
		}
	}
	return ""
}

func TestIDs(t *testing.T) {
	x, _ := newIDs(bytes.Repeat([]byte{7}, 16))
	id := x.encode(nsItem, 42)
	if len(id) != 32 {
		t.Fatalf("id %q", id)
	}
	if n, ok := x.decode(nsItem, id); !ok || n != 42 {
		t.Fatal("round trip")
	}
	if _, ok := x.decode(nsLibrary, id); ok {
		t.Fatal("namespace must be checked")
	}
	if n, ok := x.decode(nsItem, strings.ToUpper(id[:8])+"-"+id[8:12]+"-"+id[12:16]+"-"+id[16:20]+"-"+id[20:]); !ok || n != 42 {
		t.Fatal("GUID form")
	}
	for i := int64(1); i < 2000; i++ { // a neighbour of a valid id is not a valid id
		b := []byte(id)
		b[31] = "0123456789abcdef"[i%16]
		if string(b) != id {
			if _, ok := x.decode(nsItem, string(b)); ok {
				t.Fatal("a tampered id decoded")
			}
		}
	}
	y, _ := newIDs(bytes.Repeat([]byte{8}, 16))
	if _, ok := y.decode(nsItem, id); ok {
		t.Fatal("another key must not decode the id")
	}
}

func TestKeyIsKept(t *testing.T) {
	e := newEnv(t)
	a := e.srv.itemID(5)
	srv2, err := New(Deps{Store: e.st, Log: e.srv.Log})
	if err != nil || srv2.itemID(5) != a {
		t.Fatal("ids must survive a restart")
	}
}

func TestDisabled(t *testing.T) {
	e := newEnv(t)
	e.st.SetSetting(SettingEnabled, "0")
	if code, _ := e.call("GET", "/System/Info/Public", "", nil, nil); code != http.StatusServiceUnavailable {
		t.Fatalf("switched off: %d", code)
	}
}

func TestSystemInfoAndLogin(t *testing.T) {
	e := newEnv(t)
	var info map[string]any
	e.call("GET", "/system/info/public", "", nil, &info) // paths are case-insensitive
	if info["ProductName"] != "Kinora" || info["Version"] != APIVersion || info["StartupWizardCompleted"] != true {
		t.Fatalf("info: %v", info)
	}
	if code, _ := e.call("GET", "/Users/Me", "", nil, nil); code != 401 {
		t.Fatal("no token must be refused")
	}
	tok := e.login("ANNA", "secret123") // account names are case-insensitive
	var me map[string]any
	e.call("GET", "/emby/Users/Me", tok, nil, &me) // the old /emby prefix works
	if me["Name"] != "anna" {
		t.Fatalf("me: %v", me["Name"])
	}
	// the kids profile: account/profile, PIN after '#'
	for _, pw := range []string{"secret123", "secret123#0000", "wrong#1234"} {
		if code, _ := e.call("POST", "/Users/AuthenticateByName", "", map[string]string{"Username": "anna/Kicsi", "Pw": pw}, nil); code != 401 {
			t.Fatalf("kids login with %q: %d", pw, code)
		}
	}
	kidTok := e.login("anna/kicsi", "secret123#1234")
	e.call("GET", "/Users/Me", kidTok, nil, &me)
	if me["Name"] != "anna/Kicsi" {
		t.Fatalf("kid: %v", me["Name"])
	}
	// the session is a Kinora device, pinned to the profile
	var plat string
	var lock int64
	e.st.DB.QueryRow(`SELECT platform, COALESCE(profile_lock,0) FROM sessions ORDER BY created_at DESC, rowid DESC LIMIT 1`).Scan(&plat, &lock)
	if plat != Platform || lock != e.kid.ID {
		t.Fatalf("session: %s %d", plat, lock)
	}
}

func TestLockout(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < maxFails; i++ {
		e.call("POST", "/Users/AuthenticateByName", "", map[string]string{"Username": "anna/Kicsi", "Pw": fmt.Sprintf("secret123#%04d", i)}, nil)
	}
	// locked: even the right PIN is refused now
	if code, _ := e.call("POST", "/Users/AuthenticateByName", "", map[string]string{"Username": "anna/Kicsi", "Pw": "secret123#1234"}, nil); code != http.StatusTooManyRequests {
		t.Fatalf("after %d failures: %d", maxFails, code)
	}
	// the lock is per account and IP: another account from the same address still works
	e.st.CreateUser("bela", "secret456", false)
	if code, _ := e.call("POST", "/Users/AuthenticateByName", "", map[string]string{"Username": "bela", "Pw": "secret456"}, nil); code != 200 {
		t.Fatalf("other account: %d", code)
	}
}

func TestBrowseAndKidsFilter(t *testing.T) {
	e := newEnv(t)
	tok := e.login("anna", "secret123")
	var views itemList
	e.call("GET", "/UserViews", tok, nil, &views)
	if len(views.Items) != 2 {
		t.Fatalf("views: %+v", views)
	}
	movies := views.find("Filmek")
	var films itemList
	e.call("GET", "/Items?ParentId="+movies+"&Recursive=true&IncludeItemTypes=Movie&SortBy=SortName&Fields=MediaSources", tok, nil, &films)
	if films.TotalRecordCount != 3 || films.find("Space Drama") == "" || len(films.Items[0].MediaSources) != 1 {
		t.Fatalf("films: %d %+v", films.TotalRecordCount, films.Items)
	}
	// paging
	var page itemList
	e.call("GET", "/Items?ParentId="+movies+"&IncludeItemTypes=Movie&StartIndex=1&Limit=1", tok, nil, &page)
	if len(page.Items) != 1 || page.TotalRecordCount != 3 {
		t.Fatalf("page: %+v", page)
	}

	// series → seasons → episodes
	var shows itemList
	e.call("GET", "/Users/x/Items?ParentId="+views.find("Sorozatok")+"&IncludeItemTypes=Series", tok, nil, &shows)
	series := shows.find("Test Show")
	var seasons, eps itemList
	e.call("GET", "/Shows/"+series+"/Seasons", tok, nil, &seasons)
	if len(seasons.Items) != 2 {
		t.Fatalf("seasons: %+v", seasons)
	}
	e.call("GET", "/Shows/"+series+"/Episodes?SeasonId="+seasons.Items[0].Id, tok, nil, &eps)
	if len(eps.Items) != 3 || eps.Items[0].IndexNumber != 1 || eps.Items[0].SeriesId != series {
		t.Fatalf("episodes: %+v", eps)
	}
	var hints struct{ SearchHints []struct{ Name string } }
	e.call("GET", "/Search/Hints?SearchTerm=space", tok, nil, &hints)
	if len(hints.SearchHints) != 1 {
		t.Fatalf("search: %+v", hints)
	}

	// the kids profile sees neither the R film nor the unrated one, anywhere
	kidTok := e.login("anna/Kicsi", "secret123#1234")
	var kid itemList
	e.call("GET", "/Items?Recursive=true&IncludeItemTypes=Movie", kidTok, nil, &kid)
	if kid.find("Space Drama") != "" || kid.find("Kids Cartoon") != "" {
		t.Fatalf("kids list: %+v", kid.Items)
	}
	space := films.find("Space Drama")
	for _, p := range []string{"/Items/" + space, "/Items?Ids=" + space, "/Videos/" + space + "/stream?static=true", "/Items/" + space + "/PlaybackInfo"} {
		method := "GET"
		if strings.HasSuffix(p, "PlaybackInfo") {
			method = "POST"
		}
		code, b := e.call(method, p, kidTok, nil, nil)
		if code == 200 && !strings.Contains(string(b), `"TotalRecordCount":0`) {
			t.Fatalf("kids access to %s: %d %s", p, code, b)
		}
	}
	e.call("GET", "/Search/Hints?SearchTerm=space", kidTok, nil, &hints)
	if len(hints.SearchHints) != 0 {
		t.Fatal("kids search leak")
	}
	// a guessed id fails
	if code, _ := e.call("GET", "/Items/0123456789abcdef0123456789abcdef", tok, nil, nil); code != 404 {
		t.Fatalf("guessed id: %d", code)
	}
}

func TestPlaybackAndState(t *testing.T) {
	e := newEnv(t)
	tok := e.login("anna", "secret123")
	var films itemList
	e.call("GET", "/Items?Recursive=true&IncludeItemTypes=Movie", tok, nil, &films)
	big, space := films.find("Big Test Movie"), films.find("Space Drama")

	// no device profile (Infuse, Kodi): the original file
	var pi struct {
		MediaSources []struct {
			SupportsDirectPlay bool
			TranscodingUrl     string
			MediaStreams       []struct {
				Index                int
				Type                 string
				IsExternal           bool
				IsTextSubtitleStream bool
				DeliveryUrl          string
			}
		}
		PlaySessionId string
	}
	e.call("POST", "/Items/"+space+"/PlaybackInfo", tok, map[string]any{}, &pi)
	if !pi.MediaSources[0].SupportsDirectPlay || pi.MediaSources[0].TranscodingUrl != "" {
		t.Fatalf("direct: %+v", pi)
	}
	var sub string
	for _, ms := range pi.MediaSources[0].MediaStreams {
		if ms.Type == "Subtitle" && ms.IsTextSubtitleStream {
			sub = ms.DeliveryUrl
		}
	}
	if sub == "" {
		t.Fatalf("subtitle stream: %+v", pi.MediaSources[0].MediaStreams)
	}
	code, b := e.call("GET", sub, "", nil, nil) // the delivery URL carries the token itself
	if code != 200 || !strings.HasPrefix(string(b), "WEBVTT") {
		t.Fatalf("vtt %d: %s", code, b)
	}
	code, b = e.call("GET", strings.Replace(sub, "Stream.vtt", "Stream.srt", 1), "", nil, nil)
	// the MKV starts a few milliseconds late, which the times keep
	if code != 200 || !regexp.MustCompile(`^1\n00:00:01,\d{3} --> 00:00:04,\d{3}\nHouston`).Match(b) {
		t.Fatalf("srt %d: %q", code, b)
	}

	// direct stream with a range, token in the query
	req, _ := http.NewRequest("GET", e.ts.URL+"/Videos/"+big+"/stream?static=true&api_key="+tok, nil)
	req.Header.Set("Range", "bytes=0-99")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusPartialContent || resp.ContentLength != 100 {
		t.Fatalf("range: %v %+v", err, resp)
	}
	resp.Body.Close()

	// a client that plays only H.264 in MP4: HEVC is transcoded through HLS
	profile := map[string]any{"DirectPlayProfiles": []map[string]string{{"Container": "mp4", "Type": "Video", "VideoCodec": "h264", "AudioCodec": "aac"}}}
	e.call("POST", "/Items/"+space+"/PlaybackInfo", tok, map[string]any{"DeviceProfile": profile}, &pi)
	src := pi.MediaSources[0]
	if src.SupportsDirectPlay || !strings.Contains(src.TranscodingUrl, "master.m3u8") {
		t.Fatalf("transcode: %+v", src)
	}
	code, master := e.call("GET", src.TranscodingUrl, "", nil, nil)
	if code != 200 || !strings.Contains(string(master), "main.m3u8") {
		t.Fatalf("master %d: %s", code, master)
	}
	mainURL := "/Videos/" + space + "/" + strings.TrimSpace(strings.Split(string(master), "\n")[2])
	code, pl := e.call("GET", mainURL, "", nil, nil)
	if code != 200 || !strings.Contains(string(pl), "hls1/") {
		t.Fatalf("main %d: %s", code, pl)
	}
	var segURL string
	for _, l := range strings.Split(string(pl), "\n") {
		if strings.HasPrefix(l, "hls1/") {
			segURL = "/Videos/" + space + "/" + l
			break
		}
	}
	if code, seg := e.call("GET", segURL, "", nil, nil); code != 200 || len(seg) < 1000 {
		t.Fatalf("segment %d (%d bytes)", code, len(seg))
	}
	// stopping ends the Kinora transcode
	e.call("POST", "/Sessions/Playing/Stopped", tok, map[string]any{"ItemId": space, "PositionTicks": 150_000_000, "PlaySessionId": pi.PlaySessionId}, nil)
	if n := len(e.srv.Media.Sessions()); n != 0 {
		t.Fatalf("transcodes left: %d", n)
	}

	// progress, played, favourite end up in Kinora's user data
	var item map[string]any
	e.call("GET", "/Items/"+space, tok, nil, &item)
	if pos := item["UserData"].(map[string]any)["PlaybackPositionTicks"].(float64); pos != 150_000_000 {
		t.Fatalf("resume position: %v", pos)
	}
	var resume itemList
	e.call("GET", "/UserItems/Resume", tok, nil, &resume)
	if resume.find("Space Drama") == "" {
		t.Fatal("continue watching")
	}
	e.call("POST", "/UserPlayedItems/"+big, tok, nil, nil)
	e.call("POST", "/Users/x/FavoriteItems/"+big, tok, nil, nil)
	var bigItem itemList
	e.call("GET", "/Items?Ids="+big, tok, nil, &bigItem)
	if !bigItem.Items[0].UserData.Played || !bigItem.Items[0].UserData.IsFavorite {
		t.Fatalf("user data: %+v", bigItem.Items[0].UserData)
	}
	// the kids profile's data is its own
	kidTok := e.login("anna/Kicsi", "secret123#1234")
	e.call("GET", "/Items?Ids="+big, kidTok, nil, &bigItem)
	if len(bigItem.Items) != 1 || bigItem.Items[0].UserData.Played {
		t.Fatalf("kid sees the adult's state: %+v", bigItem.Items)
	}
}

func TestImagesWithoutToken(t *testing.T) {
	e := newEnv(t)
	tok := e.login("anna", "secret123")
	var films itemList
	e.call("GET", "/Items?Recursive=true&IncludeItemTypes=Movie", tok, nil, &films)
	for _, it := range films.Items {
		if tag := it.ImageTags["Primary"]; tag != "" {
			if code, _ := e.call("GET", "/Items/"+it.Id+"/Images/Primary?tag="+tag+"&maxWidth=300", "", nil, nil); code != 200 {
				t.Fatalf("image of %s: %d", it.Name, code)
			}
			return
		}
	}
	t.Skip("no artwork in the test library")
}
