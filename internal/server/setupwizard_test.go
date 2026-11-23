package server

import (
	"net"
	"net/http"
	"net/http/cookiejar"
	"testing"

	"github.com/ScotyLucas/kinora/internal/store"
)

func TestSetupWizardStatus(t *testing.T) {
	c, st := newTestServer(t)
	var s setupStatus
	if code := c.do("GET", "/api/admin/setup/status", nil, &s); code != 200 {
		t.Fatalf("status %d", code)
	}
	// a fresh install: the wizard is pending; the test library is there; nothing else is set up
	if !s.Pending || s.Libraries != 2 || s.Movies != 3 || s.Series != 2 || s.Episodes == 0 {
		t.Fatalf("fresh: %+v", s)
	}
	if s.Metadata.TMDB || s.Downloads.Clients != 0 || s.Downloads.Indexers != 0 || s.Downloads.Enabled || s.Remote.PublicURL != "" {
		t.Fatalf("fresh extras: %+v", s)
	}
	if s.Encoder.Name == "" {
		t.Fatalf("no encoder picked: %+v", s.Encoder)
	}

	// what the wizard's steps save shows up in the summary
	if code := c.do("PATCH", "/api/admin/settings", map[string]string{"tmdb_key": "abcd1234abcd1234", "public_url": "https://kinora.example.org/"}, nil); code != 200 {
		t.Fatalf("settings %d", code)
	}
	if err := st.SaveAcqClient(&store.AcqClient{Kind: "qbittorrent", Name: "qb", URL: "http://qb:8080", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	c.do("GET", "/api/admin/setup/status", nil, &s)
	if !s.Metadata.TMDB || s.Remote.PublicURL != "https://kinora.example.org" || s.Downloads.Clients != 1 {
		t.Fatalf("after the steps: %+v", s)
	}

	// closing the wizard
	if code := c.do("POST", "/api/admin/setup/finish", map[string]any{}, nil); code != 200 {
		t.Fatalf("finish %d", code)
	}
	c.do("GET", "/api/admin/setup/status", nil, &s)
	if s.Pending {
		t.Fatal("still pending after finish")
	}

	// an install from before the wizard (no value) is not asked to run it
	st.DB.Exec(`DELETE FROM settings WHERE key=?`, settingWizard)
	c.do("GET", "/api/admin/setup/status", nil, &s)
	if s.Pending {
		t.Fatal("an older install is pending")
	}
}

func TestSetupWizardAdminOnly(t *testing.T) {
	c, st := newTestServer(t)
	if _, err := st.CreateUser("viewer", "secret123", false); err != nil {
		t.Fatal(err)
	}
	jar, _ := cookiejar.New(nil)
	vc := &client{t: t, base: c.base, hc: &http.Client{Jar: jar}}
	for _, r := range []struct{ method, path string }{{"GET", "/api/admin/setup/status"}, {"POST", "/api/admin/setup/finish"}} {
		if code := vc.do(r.method, r.path, map[string]any{}, nil); code != http.StatusUnauthorized {
			t.Fatalf("%s %s without login: %d", r.method, r.path, code)
		}
	}
	if code := vc.do("POST", "/api/login", map[string]string{"username": "viewer", "password": "secret123"}, nil); code != 200 {
		t.Fatalf("login %d", code)
	}
	for _, r := range []struct{ method, path string }{{"GET", "/api/admin/setup/status"}, {"POST", "/api/admin/setup/finish"}} {
		if code := vc.do(r.method, r.path, map[string]any{}, nil); code != http.StatusForbidden {
			t.Fatalf("%s %s as a non-admin: %d", r.method, r.path, code)
		}
	}
	if st.Setting(settingWizard) != "0" {
		t.Fatal("a non-admin changed the wizard state")
	}
}

func TestIsTailscaleIP(t *testing.T) {
	for ip, want := range map[string]bool{"100.64.0.1": true, "100.101.102.103": true, "100.127.255.254": true,
		"100.128.0.1": false, "100.63.255.255": false, "192.168.1.10": false, "fd7a:115c:a1e0::1": false} {
		if got := isTailscaleIP(net.ParseIP(ip)); got != want {
			t.Errorf("%s: %v", ip, got)
		}
	}
}
