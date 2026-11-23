package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ScotyLucas/kinora/internal/jfcompat"
	"github.com/ScotyLucas/kinora/internal/store"
)

// A Jellyfin-compatible client signs in with Quick Connect: the code is approved in Kinora with a profile
// (and that profile's PIN, which the client itself cannot ask for).
func TestJellyfinQuickConnect(t *testing.T) {
	c, st, srv := newTestServerFull(t)
	jf, err := jfcompat.New(jfcompat.Deps{Ctx: context.Background(), Store: st, Media: srv.Media, Encoders: srv.Encoders, Subs: srv.Subs,
		ImgDir: srv.ImgDir, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), QC: srv.JFCompatQC()})
	if err != nil {
		t.Fatal(err)
	}
	st.SetSetting(jfcompat.SettingEnabled, "1")
	jts := httptest.NewServer(jf)
	defer jts.Close()
	call := func(method, path string, body, out any) int {
		var r io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			r = bytes.NewReader(b)
		}
		req, _ := http.NewRequest(method, jts.URL+path, r)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", `MediaBrowser Client="Swiftfin", Device="iPhone", DeviceId="x", Version="1.0"`)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if out != nil {
			json.Unmarshal(b, out)
		}
		return resp.StatusCode
	}

	var me struct {
		User     store.User      `json:"user"`
		Profiles []store.Profile `json:"profiles"`
	}
	c.do("GET", "/api/me", nil, &me)
	var parent store.Profile
	if code := c.do("POST", "/api/profiles", map[string]any{"name": "Szülő", "pin": "4321"}, &parent); code != 201 {
		t.Fatalf("profile: %d", code)
	}

	var qc struct {
		Secret, Code  string
		Authenticated bool
	}
	if code := call("POST", "/QuickConnect/Initiate", nil, &qc); code != 200 || len(qc.Code) != 6 {
		t.Fatalf("initiate: %d %+v", code, qc)
	}
	// Kinora shows the device as a Jellyfin-compatible client
	var info map[string]any
	c.do("POST", "/api/quickconnect/lookup", map[string]string{"code": qc.Code}, &info)
	if info["platform"] != jfcompat.Platform {
		t.Fatalf("lookup: %v", info)
	}
	// no profile chosen, or a wrong PIN: refused, the code stays pending
	if code := c.do("POST", "/api/quickconnect/approve", map[string]any{"code": qc.Code}, nil); code != 400 {
		t.Fatalf("approve without a profile: %d", code)
	}
	if code := c.do("POST", "/api/quickconnect/approve", map[string]any{"code": qc.Code, "profile_id": parent.ID, "pin": "0000"}, nil); code != 403 {
		t.Fatalf("wrong PIN: %d", code)
	}
	var state struct{ Authenticated bool }
	call("GET", "/QuickConnect/Connect?Secret="+qc.Secret, nil, &state)
	if state.Authenticated {
		t.Fatal("not approved yet")
	}
	if code := c.do("POST", "/api/quickconnect/approve", map[string]any{"code": qc.Code, "profile_id": parent.ID, "pin": "4321"}, nil); code != 200 {
		t.Fatalf("approve: %d", code)
	}
	call("GET", "/QuickConnect/Connect?secret="+qc.Secret, nil, &state)
	if !state.Authenticated {
		t.Fatal("approved code not reported")
	}
	var auth struct {
		AccessToken string
		User        struct{ Name string }
	}
	if code := call("POST", "/Users/AuthenticateWithQuickConnect", map[string]string{"Secret": qc.Secret}, &auth); code != 200 || auth.AccessToken == "" {
		t.Fatalf("authenticate: %d", code)
	}
	if auth.User.Name != me.User.Username+"/Szülő" {
		t.Fatalf("signed in as %q", auth.User.Name)
	}
	// a code works once
	if code := call("POST", "/Users/AuthenticateWithQuickConnect", map[string]string{"Secret": qc.Secret}, nil); code != 401 {
		t.Fatalf("second use: %d", code)
	}
}
