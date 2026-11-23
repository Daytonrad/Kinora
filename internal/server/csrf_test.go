package server

import "testing"

// A page on another port of the same host (or a sibling subdomain) is same-site: the browser sends the
// session cookie with its form posts. Such a request must not act as the signed-in admin.
func TestCookieIgnoredOnSameSiteStateChange(t *testing.T) {
	c, _ := newTestServer(t)
	body := map[string]any{"username": "intruder", "password": "secret123", "is_admin": true}

	for _, site := range []string{"same-site", "cross-site"} {
		evil := &client{t: t, base: c.base, hc: c.hc, hdr: map[string]string{"Sec-Fetch-Site": site, "Origin": "http://192.168.1.10:8080"}}
		if code := evil.do("POST", "/api/admin/users", body, nil); code != 401 {
			t.Fatalf("%s POST with the session cookie: %d, want 401", site, code)
		}
		// reads are not state changes and keep working (the browser cannot read a cross-origin answer anyway)
		if code := evil.do("GET", "/api/me", nil, nil); code != 200 {
			t.Fatalf("%s GET: %d", site, code)
		}
	}

	// the web client itself: same-origin
	own := &client{t: t, base: c.base, hc: c.hc, hdr: map[string]string{"Sec-Fetch-Site": "same-origin"}}
	var users []map[string]any
	if code := own.do("POST", "/api/admin/users", map[string]any{"username": "family", "password": "secret123"}, nil); code != 201 {
		t.Fatalf("same-origin POST: %d", code)
	}
	if code := own.do("GET", "/api/admin/users", nil, &users); code != 200 {
		t.Fatalf("users: %d", code)
	}
	for _, u := range users {
		if u["username"] == "intruder" {
			t.Fatal("the same-site request created a user")
		}
	}
}
