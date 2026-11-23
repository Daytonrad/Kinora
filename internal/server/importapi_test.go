package server

import (
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
)

// A minimal Jellyfin: one user who watched "Big Test Movie" (from the test library) and a film Kinora lacks.
func fakeJF(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Emby-Token") != "k3y" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/System/Info":
			w.Write([]byte(`{"ServerName":"JF","Version":"10.10.7","Id":"x"}`))
		case r.URL.Path == "/Users":
			w.Write([]byte(`[{"Id":"u1","Name":"Kati","Policy":{"IsAdministrator":false,"IsDisabled":false}}]`))
		case r.URL.Path == "/Users/u1/Items":
			w.Write([]byte(`{"TotalRecordCount":2,"Items":[
				{"Id":"a","Name":"Big Test Movie","Type":"Movie","ProductionYear":2019,"UserData":{"Played":true,"PlayCount":1,"LastPlayedDate":"2025-01-02T03:04:05Z"}},
				{"Id":"b","Name":"=cmd|' /C calc'!A0","Type":"Movie","ProductionYear":1900,"UserData":{"IsFavorite":true}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestJellyfinImportAPI(t *testing.T) {
	c, st := newTestServer(t)
	jf := fakeJF(t)
	defer jf.Close()

	var bad map[string]string
	if code := c.do("POST", "/api/admin/import/jellyfin/preview", map[string]any{"url": jf.URL, "api_key": "nope"}, &bad); code != 502 || bad["code"] != "unauthorized" {
		t.Fatalf("wrong key: %d %v", code, bad)
	}
	var plan struct {
		ID    string `json:"id"`
		Users []struct {
			Action  string `json:"action"`
			Entries []struct {
				Match  *struct{ ID int64 } `json:"match"`
				Reason string              `json:"reason"`
			} `json:"entries"`
		} `json:"users"`
		Matched, Unmatched int
	}
	if code := c.do("POST", "/api/admin/import/jellyfin/preview", map[string]any{"url": jf.URL, "api_key": "k3y"}, &plan); code != 200 {
		t.Fatalf("preview %d", code)
	}
	if plan.Matched != 1 || plan.Unmatched != 1 || plan.Users[0].Action != "create" {
		t.Fatalf("plan: %+v", plan)
	}
	if _, err := st.UserByName("Kati"); err == nil {
		t.Fatal("the preview created the user")
	}

	// the CSV lists both entries; a title that looks like a formula is defused
	resp, body := c.raw("GET", "/api/admin/import/jellyfin/"+plan.ID+"/export.csv", nil)
	if resp.StatusCode != 200 || !strings.Contains(string(body), "Big Test Movie") || !strings.Contains(string(body), "'=cmd") {
		t.Fatalf("csv %d:\n%s", resp.StatusCode, body)
	}

	var rep struct {
		Users []struct {
			Created      bool   `json:"created"`
			PasswordCode string `json:"password_code"`
			Changed      int    `json:"changed"`
		} `json:"users"`
	}
	if code := c.do("POST", "/api/admin/import/jellyfin/apply", map[string]any{"plan_id": plan.ID}, &rep); code != 200 {
		t.Fatalf("apply %d", code)
	}
	if len(rep.Users) != 1 || !rep.Users[0].Created || rep.Users[0].PasswordCode == "" || rep.Users[0].Changed != 1 {
		t.Fatalf("report: %+v", rep)
	}
	if code := c.do("POST", "/api/admin/import/jellyfin/apply", map[string]any{"plan_id": "nope"}, nil); code != 404 {
		t.Fatalf("unknown plan: %d", code)
	}

	// the admin user list marks the account as waiting for its password
	var users []map[string]any
	c.do("GET", "/api/admin/users", nil, &users)
	waiting := false
	for _, u := range users {
		if u["username"] == "Kati" {
			waiting = u["needs_password"] == true
		}
	}
	if !waiting {
		t.Fatal("needs_password not set")
	}

	// Kati sets her password with the link and is signed in; she sees the imported state
	jar, _ := cookiejar.New(nil)
	kati := &client{t: t, base: c.base, hc: &http.Client{Jar: jar}}
	code := rep.Users[0].PasswordCode
	var info map[string]any
	if s := kati.do("GET", "/api/password-links/"+code, nil, &info); s != 200 || info["username"] != "Kati" {
		t.Fatalf("link info %d %v", s, info)
	}
	if s := kati.do("POST", "/api/password-links/"+code, map[string]string{"password": "123"}, nil); s != 400 {
		t.Fatalf("short password: %d", s)
	}
	if s := kati.do("POST", "/api/password-links/"+code, map[string]string{"password": "katijelszo"}, nil); s != 200 {
		t.Fatalf("set password: %d", s)
	}
	if s := kati.do("POST", "/api/password-links/"+code, map[string]string{"password": "masik-jelszo"}, nil); s != 404 {
		t.Fatalf("a used link: %d", s)
	}
	var me struct {
		Profiles []struct{ ID int64 } `json:"profiles"`
	}
	kati.do("GET", "/api/me", nil, &me)
	kati.do("POST", "/api/profiles/"+itoa(me.Profiles[0].ID)+"/select", map[string]string{}, nil)
	var id int64
	st.DB.QueryRow(`SELECT id FROM items WHERE title='Big Test Movie'`).Scan(&id)
	raw, b := kati.raw("GET", "/api/items/"+itoa(id), nil)
	if raw.StatusCode != 200 || !strings.Contains(string(b), `"played":true`) {
		t.Fatalf("imported state not visible: %d %s", raw.StatusCode, b)
	}

	// only admins import
	if s := kati.do("POST", "/api/admin/import/jellyfin/preview", map[string]any{"url": jf.URL, "api_key": "k3y"}, nil); s != 403 {
		t.Fatalf("non-admin preview: %d", s)
	}
	// the admin can issue a new link for any account
	ku, _ := st.UserByName("Kati")
	var link map[string]any
	if s := c.do("POST", "/api/admin/users/"+itoa(ku.ID)+"/password-link", nil, &link); s != 200 || link["code"] == "" {
		t.Fatalf("password link: %d %v", s, link)
	}
	if s := c.do("POST", "/api/admin/users/99999/password-link", nil, nil); s != 404 {
		t.Fatalf("unknown user: %d", s)
	}
}
