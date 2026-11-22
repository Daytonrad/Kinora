package party

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type tclient struct {
	t    *testing.T
	conn *websocket.Conn
	msgs chan map[string]any
	id   int
}

func dial(t *testing.T, url, name string) *tclient {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial(url+"?name="+name, nil)
	if err != nil {
		t.Fatal(err)
	}
	c := &tclient{t: t, conn: conn, msgs: make(chan map[string]any, 256)}
	go func() {
		for {
			_, b, err := conn.ReadMessage()
			if err != nil {
				close(c.msgs)
				return
			}
			var m map[string]any
			json.Unmarshal(b, &m)
			c.msgs <- m
		}
	}()
	hello := c.waitFor("hello", nil)
	c.id = int(hello["id"].(float64))
	t.Cleanup(func() { conn.Close() })
	return c
}

func (c *tclient) send(v map[string]any) {
	c.t.Helper()
	if err := c.conn.WriteJSON(v); err != nil {
		c.t.Fatal(err)
	}
}

// waitFor returns the first message of type t that satisfies pred.
func (c *tclient) waitFor(t string, pred func(map[string]any) bool) map[string]any {
	c.t.Helper()
	timeout := time.After(3 * time.Second)
	for {
		select {
		case m, ok := <-c.msgs:
			if !ok {
				c.t.Fatalf("connection closed waiting for %s", t)
			}
			if m["t"] == t && (pred == nil || pred(m)) {
				return m
			}
		case <-timeout:
			c.t.Fatalf("timeout waiting for %s", t)
		}
	}
}

func playing(v bool) func(map[string]any) bool {
	return func(m map[string]any) bool { return m["playing"] == v }
}

func setup(t *testing.T) (*Hub, *Room, string) {
	t.Helper()
	BufferTimeout = 400 * time.Millisecond
	h := NewHub(slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go h.Run(ctx)
	room := h.Create(42, false, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		h.Serve(w, r, room, Who{UserID: 1, ProfileID: int64(name[0]), Name: name, Color: "#fff"})
	}))
	t.Cleanup(srv.Close)
	return h, room, "ws" + strings.TrimPrefix(srv.URL, "http")
}

func TestPartySync(t *testing.T) {
	_, room, url := setup(t)
	a := dial(t, url, "Anna")
	b := dial(t, url, "Bela")
	st := b.waitFor("state", func(m map[string]any) bool { return len(m["members"].([]any)) == 2 })
	if st["item_id"].(float64) != 42 || st["code"] != room.Code {
		t.Fatalf("state: %v", st)
	}

	// clock sync
	b.send(map[string]any{"t": "ping", "c": 123.0})
	pong := b.waitFor("pong", nil)
	if pong["c"].(float64) != 123 || pong["s"].(float64) <= 0 {
		t.Fatalf("pong: %v", pong)
	}

	// only the host (first joiner) controls by default
	b.send(map[string]any{"t": "play"})
	b.waitFor("sys", func(m map[string]any) bool { return strings.Contains(m["text"].(string), "házigazda") })

	// play → both see playing, echo tagged with the sender id
	a.send(map[string]any{"t": "play"})
	sa := a.waitFor("state", playing(true))
	b.waitFor("state", playing(true))
	if int(sa["by"].(float64)) != a.id {
		t.Fatalf("by=%v want %d", sa["by"], a.id)
	}

	// B buffers → room pauses and waits for B
	b.send(map[string]any{"t": "buffering", "v": true})
	w := a.waitFor("state", playing(false))
	if wl := w["waiting"].([]any); len(wl) != 1 || wl[0] != "Bela" || w["intended"] != true {
		t.Fatalf("waiting: %v", w)
	}
	b.send(map[string]any{"t": "buffering", "v": false})
	a.waitFor("state", playing(true))

	// pause at position, seek, everyone sees the same position
	a.send(map[string]any{"t": "pause", "pos": 12.5})
	p := b.waitFor("state", func(m map[string]any) bool { return m["pos"].(float64) == 12.5 })
	if p["playing"] != false || p["intended"] != false {
		t.Fatalf("pause state %v", p)
	}
	a.send(map[string]any{"t": "seek", "pos": 30.0})
	s := b.waitFor("state", func(m map[string]any) bool { return m["pos"].(float64) == 30 })
	if s["playing"] != false {
		t.Fatal("seek must not resume a paused room")
	}
	// skip intro: everyone jumps, the room is told who skipped what
	a.send(map[string]any{"t": "skip", "pos": 95.5, "kind": "intro"})
	// the room announces first, then broadcasts the new state
	b.waitFor("sys", func(m map[string]any) bool { return strings.Contains(m["text"].(string), "átugrotta az intrót") })
	b.waitFor("state", func(m map[string]any) bool { return m["pos"].(float64) == 95.5 })
	// position advances while playing
	a.send(map[string]any{"t": "play"})
	b.waitFor("state", playing(true))
	time.Sleep(300 * time.Millisecond)
	a.send(map[string]any{"t": "chat", "text": "  szia  "})
	if c := b.waitFor("chat", nil); c["text"] != "szia" || c["name"] != "Anna" {
		t.Fatalf("chat %v", c)
	}
	a.send(map[string]any{"t": "react", "emoji": "🔥"})
	b.waitFor("react", func(m map[string]any) bool { return m["emoji"] == "🔥" })
	a.send(map[string]any{"t": "react", "emoji": "💩"}) // not allowed: dropped

	// buffering timeout: the room continues without the slow member
	b.send(map[string]any{"t": "buffering", "v": true})
	a.waitFor("state", playing(false))
	a.waitFor("sys", func(m map[string]any) bool { return strings.Contains(m["text"].(string), "nélküle") })
	a.waitFor("state", playing(true))

	// host hands control to everyone; then B can pause
	a.send(map[string]any{"t": "everyone", "v": true})
	b.waitFor("state", func(m map[string]any) bool { return m["everyone"] == true })
	b.send(map[string]any{"t": "pause"})
	a.waitFor("state", playing(false))

	// a late joiner gets the chat history
	c := dial(t, url, "Cili")
	c.waitFor("chat", func(m map[string]any) bool { return m["text"] == "szia" })

	// host leaves → a new host is chosen
	a.conn.Close()
	ns := b.waitFor("state", func(m map[string]any) bool { return len(m["members"].([]any)) == 2 })
	hosts := 0
	for _, m := range ns["members"].([]any) {
		if m.(map[string]any)["host"] == true {
			hosts++
		}
	}
	if hosts != 1 {
		t.Fatalf("expected exactly one new host: %v", ns["members"])
	}
}

func TestPartyItemVisibility(t *testing.T) {
	h, _, url := setup(t)
	h.CanSee = func(profileID, itemID int64) bool { return !(profileID == int64('C') && itemID == 666) } // "Cili" cannot see 666
	a := dial(t, url, "Anna")
	dial(t, url, "Cili")
	a.send(map[string]any{"t": "item", "item_id": 666})
	a.waitFor("sys", func(m map[string]any) bool { return strings.Contains(m["text"].(string), "nem mindenki") })
	a.send(map[string]any{"t": "item", "item_id": 43, "pos": 0})
	a.waitFor("state", func(m map[string]any) bool { return m["item_id"].(float64) == 43 })
}

func TestOriginCheck(t *testing.T) {
	r := httptest.NewRequest("GET", "http://kinora.local:8096/api/party/X/ws", nil)
	r.Header.Set("Origin", "http://evil.example")
	if sameOrigin(r) {
		t.Fatal("cross-origin accepted")
	}
	r.Header.Set("Origin", "http://kinora.local:8096")
	if !sameOrigin(r) {
		t.Fatal("same origin rejected")
	}
}

func TestPartyReport(t *testing.T) {
	h, room, url := setup(t)
	got := make(chan Report, 1)
	h.Report = func(r Report) error { got <- r; return nil }
	a := dial(t, url, "Anna")
	b := dial(t, url, "Bela")

	a.send(map[string]any{"t": "chat", "text": "csúnya szó"})
	line := b.waitFor("chat", nil)
	id := line["id"].(float64)
	if id < 1 || line["from"] != "1.65" { // ProfileID is the name's first byte: 'A'
		t.Fatalf("chat line: %v", line)
	}

	// the sender cannot report their own line
	a.send(map[string]any{"t": "report", "id": id})
	a.waitFor("sys", func(m map[string]any) bool { return strings.Contains(m["text"].(string), "nem lehet") })

	b.send(map[string]any{"t": "report", "id": id, "reason": " sértő "})
	b.waitFor("sys", func(m map[string]any) bool { return strings.Contains(m["text"].(string), "Jelentés elküldve") })
	select {
	case r := <-got:
		if r.Room != room.Code || r.Sender.Name != "Anna" || r.Reporter.Name != "Bela" || r.Text != "csúnya szó" || r.Reason != "sértő" {
			t.Fatalf("report: %+v", r)
		}
	case <-time.After(time.Second):
		t.Fatal("report not stored")
	}

	// an unknown line is refused
	b.send(map[string]any{"t": "report", "id": 999})
	b.waitFor("sys", func(m map[string]any) bool { return strings.Contains(m["text"].(string), "nem lehet") })
}
