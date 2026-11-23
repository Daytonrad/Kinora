package server

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ScotyLucas/kinora/internal/push"
	"github.com/ScotyLucas/kinora/internal/push/pushtest"
)

func TestPushIOSPairing(t *testing.T) {
	c, st := newTestServer(t)
	var info map[string]any
	if code := c.do("GET", "/api/push/ios", nil, &info); code != 200 || info["enabled"] != false {
		t.Fatalf("off: %d %v", code, info)
	}
	handle := strings.Repeat("h", 43)
	key := make([]byte, 32)
	key[0] = 9
	pairBody := map[string]string{"handle": handle, "key": base64.RawURLEncoding.EncodeToString(key)}
	if code := c.do("POST", "/api/push/ios", pairBody, nil); code != 409 {
		t.Fatalf("pair while off: %d", code)
	}

	rl := pushtest.New(t)
	if code := c.do("PATCH", "/api/admin/settings", map[string]string{"push_enabled": "1", "push_relay_url": rl.URL + "/"}, nil); code != 200 {
		t.Fatalf("settings %d", code)
	}
	if code := c.do("PATCH", "/api/admin/settings", map[string]string{"push_relay_url": "ftp://x"}, nil); code != 400 {
		t.Fatalf("bad url accepted: %d", code)
	}
	if code := c.do("GET", "/api/push/ios", nil, &info); code != 200 || info["enabled"] != true || info["server_id"] != "srv_1" || info["relay_url"] != rl.URL || info["paired"] != false {
		t.Fatalf("on: %d %v", code, info)
	}
	if code := c.do("POST", "/api/push/ios", map[string]string{"handle": "rövid", "key": pairBody["key"]}, nil); code != 400 {
		t.Fatalf("bad handle: %d", code)
	}
	if code := c.do("POST", "/api/push/ios", map[string]string{"handle": handle, "key": "AAAA"}, nil); code != 400 {
		t.Fatalf("short key: %d", code)
	}
	if code := c.do("POST", "/api/push/ios", pairBody, nil); code != 200 {
		t.Fatalf("pair %d", code)
	}
	if code := c.do("POST", "/api/push/ios/test", nil, nil); code != 200 {
		t.Fatalf("test push %d", code)
	}
	sent, _, _ := rl.Snapshot()
	if len(sent) != 1 || sent[0].Handle != handle {
		t.Fatalf("sent %v", sent)
	}
	plain, err := pushtest.Open(key, sent[0].Payload)
	var m push.Message
	if err != nil || json.Unmarshal(plain, &m) != nil || m.Type != "test" {
		t.Fatalf("payload %q %v", plain, err)
	}

	// re-pairing with a new handle forgets the old one at the relay
	newHandle := strings.Repeat("n", 43)
	if code := c.do("POST", "/api/push/ios", map[string]string{"handle": newHandle, "key": base64.StdEncoding.EncodeToString(key)}, nil); code != 200 {
		t.Fatalf("re-pair %d", code)
	}
	waitDeleted(t, rl, handle)
	if code := c.do("DELETE", "/api/push/ios", nil, nil); code != 200 || st.CountPushIOS() != 0 {
		t.Fatalf("unpair %d", code)
	}
	waitDeleted(t, rl, newHandle)
	if code := c.do("POST", "/api/push/ios/test", nil, nil); code != 404 {
		t.Fatalf("test after unpair %d", code)
	}

	// a gone device answers 410 and is dropped
	c.do("POST", "/api/push/ios", pairBody, nil)
	rl.SetGone(handle)
	if code := c.do("POST", "/api/push/ios/test", nil, nil); code != 410 || st.CountPushIOS() != 0 {
		t.Fatalf("gone %d", code)
	}

	// logging out removes the pairing with the session
	c.do("POST", "/api/push/ios", map[string]string{"handle": newHandle, "key": pairBody["key"]}, nil)
	c.do("POST", "/api/logout", nil, nil)
	if st.CountPushIOS() != 0 {
		t.Fatal("pairing outlived the session")
	}
}

func waitDeleted(t *testing.T, rl *pushtest.Relay, h string) {
	t.Helper()
	for range 100 {
		if _, del, _ := rl.Snapshot(); len(del) > 0 && del[len(del)-1] == h {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("relay was not told to delete %s", h)
}
