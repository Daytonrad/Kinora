package jfcompat

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestSocket is the regression test for the WebSocket upgrade: the logging wrapper must pass http.Hijacker
// through (Kodi's JellyCon got HTTP 500 on /socket before).
func TestSocket(t *testing.T) {
	e := newEnv(t)
	tok := e.login("anna", "secret123")
	base := "ws" + strings.TrimPrefix(e.ts.URL, "http") + "/socket?deviceId=dev1"

	if _, resp, err := websocket.DefaultDialer.Dial(base, nil); err == nil {
		t.Fatal("socket without a token was accepted")
	} else if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("socket without a token: %v %v", resp, err)
	}

	conn, resp, err := websocket.DefaultDialer.Dial(base+"&api_key="+tok, nil)
	if err != nil {
		t.Fatalf("dial: %v (%v)", err, resp)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var msg struct{ MessageType string }
	if err := conn.ReadJSON(&msg); err != nil || msg.MessageType != "ForceKeepAlive" {
		t.Fatalf("first message: %+v %v", msg, err)
	}
	if err := conn.WriteJSON(map[string]string{"MessageType": "KeepAlive"}); err != nil {
		t.Fatal(err)
	}
	if err := conn.ReadJSON(&msg); err != nil || msg.MessageType != "KeepAlive" {
		t.Fatalf("keep-alive reply: %+v %v", msg, err)
	}
}
