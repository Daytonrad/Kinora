package server

import (
	"context"
	"encoding/json"
	"io"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"

	"github.com/ScotyLucas/kinora/internal/store"
)

// appClient is a cookie-less client that authenticates like a native app.
func appClient(c *client, token, origin string) *client {
	h := map[string]string{}
	if token != "" {
		h["Authorization"] = "Bearer " + token
	}
	if origin != "" {
		h["Origin"] = origin
	}
	return &client{t: c.t, base: c.base, hc: &http.Client{}, hdr: h}
}

type loginResp struct {
	Token    string     `json:"token"`
	DeviceID string     `json:"device_id"`
	User     store.User `json:"user"`
}

func appLogin(t *testing.T, c *client, platform string) (loginResp, *client) {
	t.Helper()
	anon := appClient(c, "", "capacitor://localhost")
	var lr loginResp
	resp, _ := anon.raw("POST", "/api/login", map[string]string{"username": "admin", "password": "secret123",
		"device_name": "Nappali TV", "platform": platform, "app_version": "1.0.0"})
	if resp.StatusCode != 200 {
		t.Fatalf("app login %d", resp.StatusCode)
	}
	if len(resp.Cookies()) != 0 {
		t.Fatal("app login must not set the session cookie")
	}
	_, b := anon.raw("POST", "/api/login", map[string]string{"username": "admin", "password": "secret123",
		"device_name": "Nappali TV", "platform": platform, "app_version": "1.0.0"})
	json.Unmarshal(b, &lr)
	if lr.Token == "" || lr.DeviceID == "" {
		t.Fatalf("login response: %s", b)
	}
	return lr, appClient(c, lr.Token, "capacitor://localhost")
}

func selectFirstProfile(t *testing.T, c *client) store.Profile {
	t.Helper()
	var me struct {
		Profiles []store.Profile `json:"profiles"`
	}
	c.do("GET", "/api/me", nil, &me)
	if code := c.do("POST", "/api/profiles/"+itoa(me.Profiles[0].ID)+"/select", map[string]string{}, nil); code != 200 {
		t.Fatalf("select profile %d", code)
	}
	return me.Profiles[0]
}

func itemID(st *store.Store, title string) int64 {
	var i int64
	st.DB.QueryRow(`SELECT id FROM items WHERE title=?`, title).Scan(&i)
	return i
}

func TestDevicesListAndRevoke(t *testing.T) {
	c, _ := newTestServer(t)
	lr, app := appLogin(t, c, "tizen")
	var list struct {
		Devices []store.Device `json:"devices"`
	}
	if code := app.do("GET", "/api/devices", nil, &list); code != 200 {
		t.Fatalf("devices %d", code)
	}
	var mine *store.Device
	for i, d := range list.Devices {
		if d.ID == lr.DeviceID {
			mine = &list.Devices[i]
		}
	}
	if mine == nil || !mine.Current || mine.Platform != "tizen" || mine.Name != "Nappali TV" || mine.AppVersion != "1.0.0" || mine.IP == "" {
		t.Fatalf("device entry: %+v", list.Devices)
	}
	if len(list.Devices) < 2 {
		t.Fatal("the web session should be listed too")
	}
	// admin sees all devices
	var all struct {
		Devices []store.Device `json:"devices"`
	}
	if code := c.do("GET", "/api/admin/devices", nil, &all); code != 200 || len(all.Devices) < 2 || all.Devices[0].Username == "" {
		t.Fatalf("admin devices %d %+v", code, all)
	}
	// revoke the app from the web session
	if code := c.do("DELETE", "/api/devices/"+lr.DeviceID, nil, nil); code != 200 {
		t.Fatalf("revoke %d", code)
	}
	if code := app.do("GET", "/api/me", nil, nil); code != 401 {
		t.Fatalf("revoked token still works: %d", code)
	}
	// "log out everywhere else" keeps the caller
	_, app2 := appLogin(t, c, "android")
	var res struct {
		Removed int `json:"removed"`
	}
	c.do("DELETE", "/api/devices", nil, &res)
	if res.Removed < 1 {
		t.Fatalf("removed %d", res.Removed)
	}
	if code := app2.do("GET", "/api/me", nil, nil); code != 401 {
		t.Fatal("other device not logged out")
	}
	if code := c.do("GET", "/api/me", nil, nil); code != 200 {
		t.Fatal("caller must stay logged in")
	}
}

func TestCORS(t *testing.T) {
	c, _ := newTestServer(t)
	lr, _ := appLogin(t, c, "android")
	pre := func(origin string) *http.Response {
		req, _ := http.NewRequest("OPTIONS", c.base+"/api/me", nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", "GET")
		req.Header.Set("Access-Control-Request-Headers", "authorization")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp
	}
	for _, o := range []string{"capacitor://localhost", "http://localhost", "https://localhost", "null"} {
		r := pre(o)
		if r.StatusCode != 204 || r.Header.Get("Access-Control-Allow-Origin") != o ||
			!strings.Contains(r.Header.Get("Access-Control-Allow-Headers"), "Authorization") {
			t.Fatalf("preflight %s: %d %v", o, r.StatusCode, r.Header)
		}
		if r.Header.Get("Access-Control-Allow-Credentials") != "" {
			t.Fatal("credentials must never be allowed")
		}
	}
	if r := pre("https://evil.example"); r.StatusCode != 403 || r.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("foreign preflight: %d", r.StatusCode)
	}
	// bearer request from an app origin: allowed, with CORS headers
	app := appClient(c, lr.Token, "null")
	resp, _ := app.raw("GET", "/api/me", nil)
	if resp.StatusCode != 200 || resp.Header.Get("Access-Control-Allow-Origin") != "null" {
		t.Fatalf("bearer cors: %d %v", resp.StatusCode, resp.Header)
	}
	// cookie auth from an app origin is ignored
	cookieApp := &client{t: t, base: c.base, hc: c.hc, hdr: map[string]string{"Origin": "capacitor://localhost"}}
	if code := cookieApp.do("GET", "/api/me", nil, nil); code != 401 {
		t.Fatalf("cookie must not authenticate cross-origin requests: %d", code)
	}
	// unknown origin gets no CORS headers
	evil := &client{t: t, base: c.base, hc: &http.Client{}, hdr: map[string]string{"Origin": "https://evil.example", "Authorization": "Bearer " + lr.Token}}
	resp, _ = evil.raw("GET", "/api/me", nil)
	if resp.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("evil origin got CORS")
	}
}

type playResp struct {
	Mode        string `json:"mode"`
	URL         string `json:"url"`
	Session     string `json:"session"`
	StreamToken string `json:"stream_token"`
	Subtitles   []struct {
		URL string `json:"url"`
	} `json:"subtitles"`
}

func TestStreamTokens(t *testing.T) {
	c, st := newTestServer(t)
	lr, app := appLogin(t, c, "webos")
	selectFirstProfile(t, app)
	caps := map[string]any{"video": []string{"h264"}, "audio": []string{"aac", "mp3"}, "containers": []string{"mp4"}}
	movie, ep, other := itemID(st, "Big Test Movie"), itemID(st, "Pilot Part 1"), itemID(st, "Space Drama")

	var d playResp
	app.do("POST", "/api/items/"+itoa(movie)+"/playback", map[string]any{"caps": caps}, &d)
	if d.Mode != "direct" || !strings.Contains(d.URL, "?st=") || d.StreamToken == "" || len(d.Subtitles) == 0 || !strings.Contains(d.Subtitles[0].URL, "st=") {
		t.Fatalf("direct response: %+v", d)
	}
	anon := &client{t: t, base: c.base, hc: &http.Client{}}
	if resp, _ := anon.raw("GET", d.URL, nil); resp.StatusCode != 200 {
		t.Fatalf("stream with token: %d", resp.StatusCode)
	}
	if resp, _ := anon.raw("GET", d.Subtitles[0].URL, nil); resp.StatusCode != 200 {
		t.Fatalf("subtitle with token: %d", resp.StatusCode)
	}
	if code := anon.do("GET", fmt.Sprintf("/api/stream/%d", movie), nil, nil); code != 401 {
		t.Fatalf("stream without token: %d", code)
	}
	// the token is scoped to its item
	if code := anon.do("GET", fmt.Sprintf("/api/stream/%d?st=%s", other, d.StreamToken), nil, nil); code != 403 {
		t.Fatalf("token on another item: %d", code)
	}
	// tampering breaks the signature
	bad := d.StreamToken[:len(d.StreamToken)-2] + "xx"
	if code := anon.do("GET", fmt.Sprintf("/api/stream/%d?st=%s", movie, bad), nil, nil); code != 401 {
		t.Fatalf("tampered token: %d", code)
	}
	// a forged payload (other user / item) without the key is rejected
	if code := anon.do("GET", fmt.Sprintf("/api/stream/%d?st=MXwyfDF8MXxhYmN8OTk5OTk5OTk5OQ.AAAA", movie), nil, nil); code != 401 {
		t.Fatalf("forged token: %d", code)
	}

	// HLS: playlist segments carry the token, and segments load without headers
	app.do("POST", "/api/items/"+itoa(ep)+"/playback", map[string]any{"caps": caps}, &d)
	if d.Mode != "remux" || d.Session == "" {
		t.Fatalf("remux: %+v", d)
	}
	var pl []byte
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		var resp *http.Response
		resp, pl = anon.raw("GET", d.URL, nil)
		if resp.StatusCode == 200 && strings.Contains(string(pl), ".ts") {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	var seg string
	for _, l := range strings.Split(string(pl), "\n") {
		if strings.HasPrefix(l, "seg") {
			seg = l
			break
		}
	}
	if !strings.Contains(seg, "?st=") {
		t.Fatalf("segment URL without token: %q\n%s", seg, pl)
	}
	segURL := "/api/hls/" + d.Session + "/" + seg
	for time.Now().Before(deadline) {
		if resp, _ := anon.raw("GET", segURL, nil); resp.StatusCode == 200 {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if resp, _ := anon.raw("GET", segURL, nil); resp.StatusCode != 200 {
		t.Fatalf("segment with token: %d", resp.StatusCode)
	}
	// the movie's token cannot open the episode's HLS session
	movieTok := d.StreamToken
	app.do("POST", "/api/items/"+itoa(movie)+"/playback", map[string]any{"caps": caps}, &d)
	if code := anon.do("GET", "/api/hls/"+strings.Split(strings.TrimPrefix(segURL, "/api/hls/"), "/")[0]+"/index.m3u8?st="+d.StreamToken, nil, nil); code != 404 {
		t.Fatalf("other item's token on HLS session: %d", code)
	}

	// another user's token cannot reach this user's HLS session
	c.do("POST", "/api/admin/users", map[string]any{"username": "bob", "password": "secret456"}, nil)
	bob := appClient(c, "", "null")
	var bl loginResp
	_, b := bob.raw("POST", "/api/login", map[string]string{"username": "bob", "password": "secret456", "platform": "android"})
	json.Unmarshal(b, &bl)
	bob = appClient(c, bl.Token, "null")
	selectFirstProfile(t, bob)
	var bd playResp
	bob.do("POST", "/api/items/"+itoa(ep)+"/playback", map[string]any{"caps": caps}, &bd)
	sess := strings.Split(strings.TrimPrefix(segURL, "/api/hls/"), "/")[0]
	if code := anon.do("GET", "/api/hls/"+sess+"/index.m3u8?st="+bd.StreamToken, nil, nil); code != 404 {
		t.Fatalf("bob's token on admin's session: %d", code)
	}

	// artwork token from /api/me works for images only
	var me struct {
		MediaToken string `json:"media_token"`
	}
	app.do("GET", "/api/me", nil, &me)
	if resp, _ := anon.raw("GET", fmt.Sprintf("/api/images/%d/thumb?st=%s", movie, me.MediaToken), nil); resp.StatusCode != 200 && resp.StatusCode != 404 {
		t.Fatalf("image with media token: %d", resp.StatusCode)
	}
	if code := anon.do("GET", fmt.Sprintf("/api/stream/%d?st=%s", movie, me.MediaToken), nil, nil); code != 403 {
		t.Fatalf("media token must not stream: %d", code)
	}

	// logging the device out kills its stream tokens
	c.do("DELETE", "/api/devices/"+lr.DeviceID, nil, nil)
	if code := anon.do("GET", fmt.Sprintf("/api/stream/%d?st=%s", ep, movieTok), nil, nil); code != 401 {
		t.Fatalf("token after logout: %d", code)
	}
}

func TestStreamTokenKidsProfile(t *testing.T) {
	c, st := newTestServer(t)
	_, app := appLogin(t, c, "android")
	var kid store.Profile
	app.do("POST", "/api/profiles", map[string]any{"name": "Kicsi", "is_kids": true, "max_rating_level": 2}, &kid)
	app.do("POST", "/api/profiles/"+itoa(kid.ID)+"/select", map[string]string{}, nil)
	var me struct {
		MediaToken string `json:"media_token"`
	}
	app.do("GET", "/api/me", nil, &me)
	anon := &client{t: t, base: c.base, hc: &http.Client{}}
	if code := anon.do("GET", fmt.Sprintf("/api/images/%d/thumb?st=%s", itemID(st, "Space Drama"), me.MediaToken), nil, nil); code != 404 {
		t.Fatalf("kids media token reaches R artwork: %d", code)
	}
}

type qcInit struct {
	Code   string `json:"code"`
	Secret string `json:"secret"`
}

type qcStatus struct {
	Status        string     `json:"status"`
	Token         string     `json:"token"`
	ProfileID     int64      `json:"profile_id"`
	NeedsPIN      bool       `json:"needs_pin"`
	ProfileLocked bool       `json:"profile_locked"`
	User          store.User `json:"user"`
}

func TestQuickConnectFlow(t *testing.T) {
	c, _ := newTestServer(t)
	tv := appClient(c, "", "null")
	var ini qcInit
	if code := tv.do("POST", "/api/quickconnect/initiate", map[string]string{"device_name": "Samsung TV", "platform": "tizen"}, &ini); code != 200 {
		t.Fatalf("initiate %d", code)
	}
	if len(ini.Code) != 6 || strings.Trim(ini.Code, "0123456789") != "" || len(ini.Secret) < 32 {
		t.Fatalf("code/secret: %+v", ini)
	}
	var s qcStatus
	tv.do("GET", "/api/quickconnect/status?secret="+ini.Secret, nil, &s)
	if s.Status != "pending" {
		t.Fatalf("pending: %+v", s)
	}
	// the approver sees the device, picks a profile and approves
	var look struct {
		DeviceName string `json:"device_name"`
		Platform   string `json:"platform"`
	}
	if code := c.do("POST", "/api/quickconnect/lookup", map[string]string{"code": ini.Code}, &look); code != 200 || look.DeviceName != "Samsung TV" || look.Platform != "tizen" {
		t.Fatalf("lookup %d %+v", code, look)
	}
	p := selectFirstProfile(t, c)
	if code := c.do("POST", "/api/quickconnect/approve", map[string]any{"code": ini.Code, "profile_id": p.ID}, nil); code != 200 {
		t.Fatalf("approve %d", code)
	}
	// approving twice fails (single use)
	if code := c.do("POST", "/api/quickconnect/approve", map[string]any{"code": ini.Code, "profile_id": p.ID}, nil); code != 404 {
		t.Fatalf("second approve %d", code)
	}
	tv.do("GET", "/api/quickconnect/status?secret="+ini.Secret, nil, &s)
	if s.Status != "approved" || s.Token == "" || s.ProfileID != p.ID || s.User.Username != "admin" {
		t.Fatalf("approved: %+v", s)
	}
	// the token works and the profile is preselected
	dev := appClient(c, s.Token, "null")
	var me struct {
		Profile *store.Profile `json:"profile"`
	}
	if code := dev.do("GET", "/api/me", nil, &me); code != 200 || me.Profile == nil || me.Profile.ID != p.ID {
		t.Fatalf("me with qc token: %d %+v", code, me)
	}
	if code := dev.do("GET", "/api/home", nil, nil); code != 200 {
		t.Fatalf("home with qc token: %d", code)
	}
	var list struct {
		Devices []store.Device `json:"devices"`
	}
	c.do("GET", "/api/devices", nil, &list)
	foundTV := false
	for _, d := range list.Devices {
		foundTV = foundTV || (d.Name == "Samsung TV" && d.Platform == "tizen")
	}
	if !foundTV {
		t.Fatal("quick connect must register a device")
	}
	// a lost answer can be fetched again for a short grace period – the same token, no new device
	var again qcStatus
	if code := tv.do("GET", "/api/quickconnect/status?secret="+ini.Secret, nil, &again); code != 200 || again.Token != s.Token {
		t.Fatalf("re-delivery %d %+v", code, again)
	}
	var after struct {
		Devices []store.Device `json:"devices"`
	}
	c.do("GET", "/api/devices", nil, &after)
	if len(after.Devices) != len(list.Devices) {
		t.Fatal("re-delivery must not create another device")
	}
	// the code itself is spent
	if code := c.do("POST", "/api/quickconnect/lookup", map[string]string{"code": ini.Code}, nil); code != 404 {
		t.Fatalf("spent code lookup %d", code)
	}
}

// The web login page signs in with a code: the approved poll sets the session cookie
// for a browser (no foreign origin), but not for a native app.
func TestQuickConnectWebLogin(t *testing.T) {
	c, _ := newTestServer(t)
	jar, _ := cookiejar.New(nil)
	web := &client{t: t, base: c.base, hc: &http.Client{Jar: jar}, hdr: map[string]string{"User-Agent": "Mozilla/5.0 (Macintosh) Chrome/130.0 Safari/537.36"}}
	var ini qcInit
	if code := web.do("POST", "/api/quickconnect/initiate", map[string]string{"platform": "web"}, &ini); code != 200 {
		t.Fatalf("initiate %d", code)
	}
	var look struct {
		DeviceName string `json:"device_name"`
	}
	if code := c.do("POST", "/api/quickconnect/lookup", map[string]string{"code": ini.Code}, &look); code != 200 || look.DeviceName == "" {
		t.Fatalf("lookup %d %+v (a browser must get a name)", code, look)
	}
	if code := web.do("GET", "/api/me", nil, nil); code != 401 {
		t.Fatalf("me before approval %d", code)
	}
	p := selectFirstProfile(t, c)
	if code := c.do("POST", "/api/quickconnect/approve", map[string]any{"code": ini.Code, "profile_id": p.ID}, nil); code != 200 {
		t.Fatalf("approve %d", code)
	}
	var s qcStatus
	if web.do("GET", "/api/quickconnect/status?secret="+ini.Secret, nil, &s); s.Status != "approved" {
		t.Fatalf("approved: %+v", s)
	}
	var me struct {
		Profile *store.Profile `json:"profile"`
	}
	if code := web.do("GET", "/api/me", nil, &me); code != 200 || me.Profile == nil || me.Profile.ID != p.ID {
		t.Fatalf("me with the qc cookie: %d %+v", code, me)
	}
	// a native app (own origin) gets no cookie
	appJar, _ := cookiejar.New(nil)
	tv := &client{t: t, base: c.base, hc: &http.Client{Jar: appJar}, hdr: map[string]string{"Origin": "null"}}
	tv.do("POST", "/api/quickconnect/initiate", map[string]string{"device_name": "TV", "platform": "tizen"}, &ini)
	c.do("POST", "/api/quickconnect/approve", map[string]any{"code": ini.Code, "profile_id": p.ID}, nil)
	if tv.do("GET", "/api/quickconnect/status?secret="+ini.Secret, nil, &s); s.Status != "approved" || s.Token == "" {
		t.Fatalf("app approved: %+v", s)
	}
	tv.hdr = nil
	if code := tv.do("GET", "/api/me", nil, nil); code != 401 {
		t.Fatalf("an app must not get a session cookie: %d", code)
	}
}

func TestQuickConnectGraceExpires(t *testing.T) {
	c, _, srv := newTestServerFull(t)
	tv := appClient(c, "", "null")
	var ini qcInit
	tv.do("POST", "/api/quickconnect/initiate", map[string]string{"platform": "tizen"}, &ini)
	c.do("POST", "/api/quickconnect/approve", map[string]any{"code": ini.Code}, nil)
	tv.do("GET", "/api/quickconnect/status?secret="+ini.Secret, nil, nil)
	srv.qc.mu.Lock()
	srv.qc.bySecret[sha(ini.Secret)].Expires = time.Now().Add(-time.Second)
	srv.qc.mu.Unlock()
	if code := tv.do("GET", "/api/quickconnect/status?secret="+ini.Secret, nil, nil); code != 410 {
		t.Fatalf("after grace %d", code)
	}
}

func TestQuickConnectApprovalsDoNotUseAttemptBudget(t *testing.T) {
	c, _ := newTestServer(t)
	tv := appClient(c, "", "null")
	for i := 0; i < qcAttempts+2; i++ {
		var ini qcInit
		tv.do("POST", "/api/quickconnect/initiate", map[string]string{"platform": "webos"}, &ini)
		if code := c.do("POST", "/api/quickconnect/lookup", map[string]string{"code": ini.Code}, nil); code != 200 {
			t.Fatalf("lookup %d: %d", i, code)
		}
		if code := c.do("POST", "/api/quickconnect/approve", map[string]any{"code": ini.Code}, nil); code != 200 {
			t.Fatalf("approve %d: %d", i, code)
		}
	}
}

func TestLockedKidsDeviceBeforePIN(t *testing.T) {
	c, st := newTestServer(t)
	var kid store.Profile
	c.do("POST", "/api/profiles", map[string]any{"name": "Kicsi", "is_kids": true, "max_rating_level": 2, "pin": "4321"}, &kid)
	tv := appClient(c, "", "null")
	var ini qcInit
	tv.do("POST", "/api/quickconnect/initiate", map[string]string{"platform": "tizen"}, &ini)
	selectFirstProfile(t, c)
	c.do("POST", "/api/quickconnect/approve", map[string]any{"code": ini.Code, "profile_id": kid.ID}, nil)
	var s qcStatus
	tv.do("GET", "/api/quickconnect/status?secret="+ini.Secret, nil, &s)
	if !s.NeedsPIN || !s.ProfileLocked {
		t.Fatalf("status %+v", s)
	}
	dev := appClient(c, s.Token, "null")
	var me struct {
		MediaToken string `json:"media_token"`
	}
	dev.do("GET", "/api/me", nil, &me)
	r := itoa(itemID(st, "Space Drama"))
	if code := dev.do("GET", "/api/images/"+r+"/thumb", nil, nil); code != 404 {
		t.Fatalf("locked device (session) sees R artwork: %d", code)
	}
	anon := &client{t: t, base: c.base, hc: &http.Client{}}
	if code := anon.do("GET", "/api/images/"+r+"/thumb?st="+me.MediaToken, nil, nil); code != 404 {
		t.Fatalf("locked device (token) sees R artwork: %d", code)
	}
}

func TestQuickConnectDenyExpiryAndRules(t *testing.T) {
	c, _, srv := newTestServerFull(t)
	tv := appClient(c, "", "null")
	start := func() qcInit {
		var ini qcInit
		tv.do("POST", "/api/quickconnect/initiate", map[string]string{"device_name": "LG TV", "platform": "webos"}, &ini)
		return ini
	}
	// deny
	ini := start()
	c.do("POST", "/api/quickconnect/approve", map[string]any{"code": ini.Code, "action": "deny"}, nil)
	var s qcStatus
	tv.do("GET", "/api/quickconnect/status?secret="+ini.Secret, nil, &s)
	if s.Status != "denied" || s.Token != "" {
		t.Fatalf("denied: %+v", s)
	}
	// expiry
	ini = start()
	srv.qc.mu.Lock()
	srv.qc.byCode[ini.Code].Expires = time.Now().Add(-time.Second)
	srv.qc.mu.Unlock()
	if code := c.do("POST", "/api/quickconnect/approve", map[string]any{"code": ini.Code}, nil); code != 404 {
		t.Fatalf("expired approve %d", code)
	}
	if code := tv.do("GET", "/api/quickconnect/status?secret="+ini.Secret, nil, nil); code != 410 {
		t.Fatalf("expired status %d", code)
	}
	// a profile of another account cannot be chosen
	var bob store.User
	c.do("POST", "/api/admin/users", map[string]any{"username": "bob", "password": "secret456"}, &bob)
	var bobProfile int64
	srv.st.DB.QueryRow(`SELECT id FROM profiles WHERE user_id=?`, bob.ID).Scan(&bobProfile)
	ini = start()
	if code := c.do("POST", "/api/quickconnect/approve", map[string]any{"code": ini.Code, "profile_id": bobProfile}, nil); code != 404 {
		t.Fatalf("foreign profile %d", code)
	}
	// kids profiles cannot approve
	var kid store.Profile
	c.do("POST", "/api/profiles", map[string]any{"name": "Kicsi", "is_kids": true}, &kid)
	c.do("POST", "/api/profiles/"+itoa(kid.ID)+"/select", map[string]string{}, nil)
	if code := c.do("POST", "/api/quickconnect/approve", map[string]any{"code": ini.Code}, nil); code != 403 {
		t.Fatalf("kids approve %d", code)
	}
	c.do("POST", "/api/profiles/leave", nil, nil)
	// a kids device is pinned to its profile
	if code := c.do("POST", "/api/quickconnect/approve", map[string]any{"code": ini.Code, "profile_id": kid.ID}, nil); code != 200 {
		t.Fatalf("approve kid device %d", code)
	}
	tv.do("GET", "/api/quickconnect/status?secret="+ini.Secret, nil, &s)
	if !s.ProfileLocked {
		t.Fatalf("kid device must be locked: %+v", s)
	}
	kidTV := appClient(c, s.Token, "null")
	var adult int64
	srv.st.DB.QueryRow(`SELECT id FROM profiles WHERE user_id=1 AND is_kids=0 ORDER BY id LIMIT 1`).Scan(&adult)
	if code := kidTV.do("POST", "/api/profiles/"+itoa(adult)+"/select", map[string]string{}, nil); code != 404 {
		t.Fatalf("locked device switched profile: %d", code)
	}
	if code := kidTV.do("POST", "/api/profiles/leave", nil, nil); code != 403 {
		t.Fatalf("locked device left profile: %d", code)
	}
	if code := kidTV.do("GET", "/api/admin/users", nil, nil); code != 403 {
		t.Fatalf("locked device reached admin: %d", code)
	}
	var me struct {
		Profiles []store.Profile `json:"profiles"`
	}
	kidTV.do("GET", "/api/me", nil, &me)
	if len(me.Profiles) != 1 || me.Profiles[0].ID != kid.ID {
		t.Fatalf("locked device sees other profiles: %+v", me.Profiles)
	}
	// PIN profile: approved without preselection, PIN entered on the device
	pin := "1234"
	adultP, _ := srv.st.Profile(adult)
	srv.st.UpdateProfile(adultP.ID, store.ProfileInput{PIN: &pin})
	ini = start()
	c.do("POST", "/api/quickconnect/approve", map[string]any{"code": ini.Code, "profile_id": adult}, nil)
	tv.do("GET", "/api/quickconnect/status?secret="+ini.Secret, nil, &s)
	if !s.NeedsPIN || s.ProfileID != adult {
		t.Fatalf("pin profile: %+v", s)
	}
	pinTV := appClient(c, s.Token, "null")
	var me2 struct {
		Profile *store.Profile `json:"profile"`
	}
	pinTV.do("GET", "/api/me", nil, &me2)
	if me2.Profile != nil {
		t.Fatal("PIN profile must not be preselected")
	}
	// disabled by the admin
	c.do("PATCH", "/api/admin/settings", map[string]string{"quickconnect_enabled": "0"}, nil)
	if code := tv.do("POST", "/api/quickconnect/initiate", map[string]string{}, nil); code != 403 {
		t.Fatalf("disabled initiate %d", code)
	}
	var stat struct {
		QC bool `json:"quickconnect_enabled"`
	}
	tv.do("GET", "/api/status", nil, &stat)
	if stat.QC {
		t.Fatal("status should report quick connect off")
	}
}

func TestQuickConnectRateLimit(t *testing.T) {
	c, _ := newTestServer(t)
	for i := 0; i < qcAttempts; i++ {
		if code := c.do("POST", "/api/quickconnect/lookup", map[string]string{"code": fmt.Sprintf("%06d", i)}, nil); code != 404 {
			t.Fatalf("attempt %d: %d", i, code)
		}
	}
	if code := c.do("POST", "/api/quickconnect/lookup", map[string]string{"code": "000000"}, nil); code != 429 {
		t.Fatalf("rate limit: %d", code)
	}
	if code := c.do("POST", "/api/quickconnect/approve", map[string]string{"code": "000000"}, nil); code != 429 {
		t.Fatalf("rate limit approve: %d", code)
	}
}

func TestQuickConnectCodesUnique(t *testing.T) {
	q := newQuickConnect()
	seen := map[string]bool{}
	for i := 0; i < 1500; i++ {
		e, _, err := q.initiate(store.DeviceInfo{})
		if err != nil {
			t.Fatal(err)
		}
		if seen[e.Code] {
			t.Fatalf("duplicate active code %s", e.Code)
		}
		seen[e.Code] = true
	}
}

func TestStatusAndBranding(t *testing.T) {
	c, _ := newTestServer(t)
	anon := appClient(c, "", "http://localhost")
	var st map[string]any
	if code := anon.do("GET", "/api/status", nil, &st); code != 200 || st["product"] != "kinora" || st["api_version"] == nil || st["server_id"] == "" {
		t.Fatalf("status %d %v", code, st)
	}
	var br map[string]any
	if code := anon.do("GET", "/api/branding", nil, &br); code != 200 || br["server_name"] != "Kinora" {
		t.Fatalf("branding %d %v", code, br)
	}
	if code := anon.do("GET", "/api/branding/../k.db", nil, nil); code == 200 {
		t.Fatal("branding traversal")
	}
}

func TestDiscovery(t *testing.T) {
	c, _, srv := newTestServerFull(t)
	_ = c
	l, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Skip("no UDP")
	}
	port := l.LocalAddr().(*net.UDPAddr).Port
	l.Close()
	srv.cfg.DiscoveryPort = port
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.RunDiscovery(ctx)
	time.Sleep(200 * time.Millisecond)
	conn, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.Write([]byte("who is KinoraServer?"))
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	var rep discoveryReply
	json.Unmarshal(buf[:n], &rep)
	if rep.Product != "kinora" || rep.Name != "Kinora" || !strings.HasPrefix(rep.Address, "http://127.0.0.1:") {
		t.Fatalf("reply %+v", rep)
	}
	// disabled by the admin: silence
	srv.st.SetSetting("discovery_enabled", "0")
	conn.Write([]byte("who is KinoraServer?"))
	conn.SetReadDeadline(time.Now().Add(700 * time.Millisecond))
	if _, err := conn.Read(buf); err == nil {
		t.Fatal("discovery answered while disabled")
	}
}

func jsonUnmarshal(b []byte, v any) { json.Unmarshal(b, v) }

func jsonDecode(r io.Reader, v any) { json.NewDecoder(r).Decode(v) }

func TestSearchGroupsEpisodesOfOneSeries(t *testing.T) {
	c, _ := newTestServer(t)
	selectFirstProfile(t, c)
	var flat, grouped []store.Item
	c.do("GET", "/api/search?q=Pilot", nil, &flat)
	c.do("GET", "/api/search?q=Pilot&group=1", nil, &grouped)
	eps := 0
	for _, it := range flat {
		if it.Kind == "episode" {
			eps++
		}
	}
	if eps < 2 {
		t.Fatalf("expected several Pilot episodes ungrouped: %d", eps)
	}
	if len(grouped) != 1 || grouped[0].Kind != "series" || grouped[0].Title != "Test Show" {
		t.Fatalf("grouped: %+v", grouped)
	}
}
