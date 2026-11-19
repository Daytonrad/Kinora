package server

import (
	"strings"
	"testing"
)

func TestLowerJSRemovesNewSyntax(t *testing.T) {
	out, err := lowerJS([]byte("const f = o => o?.a?.b ?? 1;\ntry { f(); } catch { f(null); }\n"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, bad := range []string{"?.", "??", "catch {"} {
		if strings.Contains(s, bad) {
			t.Errorf("lowered code still has %q:\n%s", bad, s)
		}
	}
}

// The real web client must lower cleanly, and the result is served with its polyfills.
func TestLegacyAppJS(t *testing.T) {
	c, _ := newTestServer(t)
	resp, b := c.raw("GET", "/app.legacy.js", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, b)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/javascript") {
		t.Fatalf("content type %q", ct)
	}
	s := string(b)
	if !strings.Contains(s, "fromEntries") || !strings.Contains(s, "function route(") {
		t.Fatal("app.legacy.js lacks the polyfills or the client")
	}
	// lowering it again finds nothing left to change in the syntax old engines lack
	if strings.Contains(s, "?.[") || strings.Contains(s, "?.(") {
		t.Error("optional chaining left in app.legacy.js")
	}
}

// Launchers on another origin (Titan OS's hosted page) may read the public status, nothing else.
func TestStatusCORSForLaunchers(t *testing.T) {
	c, _ := newTestServer(t)
	other := appClient(c, "", "https://launcher.example")
	resp, _ := other.raw("GET", "/api/status", nil)
	if resp.StatusCode != 200 || resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("status from another origin: %d, ACAO %q", resp.StatusCode, resp.Header.Get("Access-Control-Allow-Origin"))
	}
	resp, _ = other.raw("GET", "/api/me", nil)
	if resp.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("only /api/status is open to other origins")
	}
}
