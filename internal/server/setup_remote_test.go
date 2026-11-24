package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ScotyLucas/kinora/internal/media"
	"github.com/ScotyLucas/kinora/internal/meta"
	"github.com/ScotyLucas/kinora/internal/party"
	"github.com/ScotyLucas/kinora/internal/scanner"
	"github.com/ScotyLucas/kinora/internal/store"
)

// newFreshServer: a server before its first setup (no media needed).
func newFreshServer(t *testing.T, allowRemote bool) (http.Handler, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "k.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	enc := media.NewEncoders()
	srv := New(Config{Addr: ":0", DataDir: dir, CacheDir: dir, AllowRemoteSetup: allowRemote}, st, log, Deps{
		Ctx: context.Background(), Scanner: scanner.New(st, filepath.Join(dir, "images"), log), ImgDir: filepath.Join(dir, "images"),
		Media: media.NewManager(dir, enc, log), Encoders: enc, Subs: media.NewSubtitles(dir), Downloads: media.NewDownloader(dir, enc, log),
		Party: party.NewHub(log), Meta: meta.New(st, filepath.Join(dir, "images"), log)})
	t.Cleanup(func() { srv.Shutdown(context.Background()) })
	return srv.Handler(), st
}

// as sends a request as if it came from peer (the TCP address), with an optional X-Forwarded-For.
func as(h http.Handler, method, path, peer, xff string, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = net.JoinHostPort(peer, "40000")
	req.Header.Set("Content-Type", "application/json")
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestSetupOnlyFromLocalAddresses(t *testing.T) {
	const body = `{"server_name":"Otthon","username":"admin","password":"secret123"}`
	for _, tc := range []struct {
		name        string
		peer, xff   string
		allowRemote bool
		want        int
	}{
		{"loopback", "127.0.0.1", "", false, 200},
		{"home network", "192.168.1.20", "", false, 200},
		{"IPv6 unique local", "fd00::5", "", false, 200},
		{"Tailscale", "100.101.102.103", "", false, 200},
		{"local proxy, client on the home network", "127.0.0.1", "192.168.1.30", false, 200},
		{"directly from the internet", "203.0.113.5", "", false, 403},
		{"local proxy, client on the internet", "127.0.0.1", "203.0.113.5", false, 403},
		{"forged header from the internet", "203.0.113.5", "192.168.1.2", false, 403},
		{"forged leftmost entry through a proxy", "127.0.0.1", "192.168.1.2, 203.0.113.5", false, 403},
		{"KINORA_ALLOW_REMOTE_SETUP=1", "203.0.113.5", "", true, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, st := newFreshServer(t, tc.allowRemote)
			var status struct {
				SetupNeeded  bool `json:"setup_needed"`
				SetupBlocked bool `json:"setup_blocked"`
			}
			json.Unmarshal(as(h, "GET", "/api/status", tc.peer, tc.xff, "").Body.Bytes(), &status)
			if !status.SetupNeeded || status.SetupBlocked != (tc.want == 403) {
				t.Fatalf("status before setup: %+v", status)
			}
			w := as(h, "POST", "/api/setup", tc.peer, tc.xff, body)
			if w.Code != tc.want {
				t.Fatalf("setup: %d %s", w.Code, w.Body)
			}
			n, _ := st.CountUsers()
			if (n == 1) != (tc.want == 200) {
				t.Fatalf("users after setup: %d", n)
			}
			if tc.want == 403 && !strings.Contains(w.Body.String(), "helyi hálózatról") {
				t.Fatalf("message: %s", w.Body)
			}
		})
	}
}

// Once set up, the address check does not matter: the answer is the usual "already set up".
func TestSetupDoneAnswersConflictFromAnywhere(t *testing.T) {
	h, _ := newFreshServer(t, false)
	if w := as(h, "POST", "/api/setup", "192.168.1.20", "", `{"username":"admin","password":"secret123"}`); w.Code != 200 {
		t.Fatalf("setup: %d", w.Code)
	}
	if w := as(h, "POST", "/api/setup", "203.0.113.5", "", `{"username":"x","password":"secret123"}`); w.Code != http.StatusConflict {
		t.Fatalf("second setup from outside: %d", w.Code)
	}
	var status struct {
		SetupBlocked bool `json:"setup_blocked"`
	}
	json.Unmarshal(as(h, "GET", "/api/status", "203.0.113.5", "", "").Body.Bytes(), &status)
	if status.SetupBlocked {
		t.Fatal("setup_blocked on a server that is set up")
	}
}
