package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ScotyLucas/kinora/internal/store"
)

// An account limited to some libraries must not get artwork of other libraries, even before it chose a profile.
func TestImagesRespectLibrariesWithoutProfile(t *testing.T) {
	c, st := newTestServer(t)
	libs, _ := st.Libraries()
	var movies, shows int64
	for _, l := range libs {
		if l.Kind == "movies" {
			movies = l.ID
		} else {
			shows = l.ID
		}
	}
	u, err := st.CreateUser("limited", "secret123", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetUserLibraries(u.ID, []int64{shows}); err != nil {
		t.Fatal(err)
	}
	jar, _ := cookiejar.New(nil)
	lc := &client{t: t, base: c.base, hc: &http.Client{Jar: jar}}
	if code := lc.do("POST", "/api/login", map[string]string{"username": "limited", "password": "secret123"}, nil); code != 200 {
		t.Fatalf("login %d", code)
	}
	if code := lc.do("GET", "/api/libraries", nil, nil); code != http.StatusPreconditionRequired {
		t.Fatalf("the test needs a session without a profile, got %d", code)
	}
	var hidden, visible int64
	st.DB.QueryRow(`SELECT id FROM items WHERE library_id=? LIMIT 1`, movies).Scan(&hidden)
	st.DB.QueryRow(`SELECT id FROM items WHERE library_id=? LIMIT 1`, shows).Scan(&visible)

	var e map[string]string
	if code := lc.do("GET", "/api/images/"+itoa(hidden)+"/poster", nil, &e); code != 404 || e["error"] != "nem található" {
		t.Fatalf("artwork of a hidden library: %d %v", code, e)
	}
	e = nil
	if _, b := lc.raw("GET", "/api/images/"+itoa(visible)+"/poster", nil); strings.Contains(string(b), `"nem található"`) {
		t.Fatalf("artwork of an allowed library is refused: %s", b)
	}
}

// Internal errors are logged; the client gets a reference, never the error text.
func TestInternalErrorHidesDetails(t *testing.T) {
	_, _, srv := newTestServerFull(t)
	rec := httptest.NewRecorder()
	srv.internalErr(rec, httptest.NewRequest("GET", "/api/x", nil), errors.New("open /srv/media/secret.mkv: SQL near SELECT"))
	if rec.Code != 500 || strings.Contains(rec.Body.String(), "secret") || strings.Contains(rec.Body.String(), "SELECT") {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	var out map[string]string
	json.Unmarshal(rec.Body.Bytes(), &out)
	if out["ref"] == "" || !strings.Contains(out["error"], out["ref"]) {
		t.Fatalf("no reference: %v", out)
	}
	rec = httptest.NewRecorder()
	srv.internalErr(rec, httptest.NewRequest("GET", "/api/x", nil), store.ErrNotFound)
	if rec.Code != 404 {
		t.Fatalf("not found: %d", rec.Code)
	}
}
