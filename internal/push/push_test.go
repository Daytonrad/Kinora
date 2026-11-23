package push_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ScotyLucas/kinora/internal/push"
	"github.com/ScotyLucas/kinora/internal/push/pushtest"
	"github.com/ScotyLucas/kinora/internal/store"
)

func setup(t *testing.T) (*store.Store, *pushtest.Relay, *push.Notifier) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "k.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	rl := pushtest.New(t)
	st.SetSetting("push_enabled", "1")
	st.SetSetting("push_relay_url", rl.URL+"/")
	return st, rl, push.NewNotifier(st, push.NewRelay(st), slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func pair(t *testing.T, st *store.Store, userID int64, handle string) []byte {
	t.Helper()
	_, dev, err := st.CreateDeviceSession(userID, store.DeviceInfo{Platform: "ios"}, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte{byte(len(handle))}, 32)
	key[0] = handle[0]
	if err := st.SetPushIOS(dev, userID, handle, key); err != nil {
		t.Fatal(err)
	}
	return key
}

func request(t *testing.T, st *store.Store, userID int64, updated int64) int64 {
	t.Helper()
	res, err := st.DB.Exec(`INSERT INTO acq_requests(kind,tmdb_id,title,user_id,status,created_at,updated_at) VALUES('movie',603,'Mátrix',?,'available',?,?)`, userID, updated, updated)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

func TestSealMatchesCryptoKitLayout(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	sealed, err := push.Seal(key, []byte("szia"))
	if err != nil {
		t.Fatal(err)
	}
	if len(sealed) != 12+4+16 {
		t.Fatalf("len %d", len(sealed))
	}
	plain, err := pushtest.Open(key, sealed)
	if err != nil || string(plain) != "szia" {
		t.Fatalf("%q %v", plain, err)
	}
}

func TestRegistersOnceAndAgainWhenRelayChanges(t *testing.T) {
	st, rl, n := setup(t)
	ctx := context.Background()
	id1, err := n.Relay.ServerID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if id2, _ := n.Relay.ServerID(ctx); id2 != id1 {
		t.Fatalf("re-registered: %s %s", id1, id2)
	}
	if _, _, regs := rl.Snapshot(); regs != 1 {
		t.Fatalf("registrations %d", regs)
	}
	other := pushtest.New(t)
	st.SetSetting("push_relay_url", other.URL)
	if _, err := n.Relay.ServerID(ctx); err != nil {
		t.Fatal(err)
	}
	if _, _, regs := other.Snapshot(); regs != 1 {
		t.Fatal("new relay not registered with")
	}
	st.SetSetting("push_enabled", "0")
	if _, err := n.Relay.ServerID(ctx); err != push.ErrDisabled {
		t.Fatalf("disabled: %v", err)
	}
}

func TestReRegistersWhenRelayForgetsServer(t *testing.T) {
	st, rl, n := setup(t)
	u, _ := st.CreateUser("anna", "secret123", false)
	pair(t, st, u.ID, strings.Repeat("a", 43))
	ctx := context.Background()
	if err := n.SendUser(ctx, u.ID, push.Message{V: 1, Title: "x"}); err != nil {
		t.Fatal(err)
	}
	rl.Forget()
	if err := n.SendUser(ctx, u.ID, push.Message{V: 1, Title: "x"}); err == nil {
		t.Fatal("expected unauthorized")
	}
	if err := n.SendUser(ctx, u.ID, push.Message{V: 1, Title: "x"}); err != nil {
		t.Fatalf("after re-registration: %v", err)
	}
	if sent, _, regs := rl.Snapshot(); regs != 2 || len(sent) != 2 {
		t.Fatalf("regs %d sent %d", regs, len(sent))
	}
}

func TestTickPushesFulfilledRequestOnce(t *testing.T) {
	st, rl, n := setup(t)
	ctx := context.Background()
	anna, _ := st.CreateUser("anna", "secret123", false)
	bela, _ := st.CreateUser("bela", "secret123", false)
	h1, h2, hb := strings.Repeat("a", 43), strings.Repeat("b", 43), strings.Repeat("c", 43)
	k1 := pair(t, st, anna.ID, h1)
	pair(t, st, anna.ID, h2)
	pair(t, st, bela.ID, hb)
	rid := request(t, st, anna.ID, time.Now().Unix())
	old := request(t, st, anna.ID, time.Now().Add(-48*time.Hour).Unix()) // arrived while push was off

	n.Tick(ctx)
	n.Tick(ctx)
	sent, _, _ := rl.Snapshot()
	if len(sent) != 2 {
		t.Fatalf("sent %d, want one per paired device of the requester", len(sent))
	}
	for _, s := range sent {
		if s.Handle == hb {
			t.Fatal("pushed to another user")
		}
		if s.CollapseID != "req-"+itoa(rid) {
			t.Fatalf("collapse %q", s.CollapseID)
		}
	}
	var s pushtest.Sent
	for _, x := range sent {
		if x.Handle == h1 {
			s = x
		}
	}
	plain, err := pushtest.Open(k1, s.Payload)
	if err != nil {
		t.Fatal(err)
	}
	var m push.Message
	json.Unmarshal(plain, &m)
	if m.V != 1 || m.Type != "request_available" || m.Title != "Mátrix" || !strings.Contains(m.Body, "film") {
		t.Fatalf("%+v", m)
	}
	var pushed int
	st.DB.QueryRow(`SELECT pushed FROM acq_requests WHERE id=?`, old).Scan(&pushed)
	if pushed != 1 {
		t.Fatal("old request not marked")
	}
}

func TestTickKeepsPendingWhileRelayIsDown(t *testing.T) {
	st, rl, n := setup(t)
	u, _ := st.CreateUser("anna", "secret123", false)
	pair(t, st, u.ID, strings.Repeat("a", 43))
	rid := request(t, st, u.ID, time.Now().Unix())
	rl.Fail(502)
	n.Tick(context.Background())
	var pushed int
	st.DB.QueryRow(`SELECT pushed FROM acq_requests WHERE id=?`, rid).Scan(&pushed)
	if pushed != 0 {
		t.Fatal("marked although the relay failed")
	}
	rl.Fail(0)
	n.Tick(context.Background())
	if sent, _, _ := rl.Snapshot(); len(sent) != 1 {
		t.Fatalf("retry sent %d", len(sent))
	}
}

func TestGoneHandleIsDropped(t *testing.T) {
	st, rl, n := setup(t)
	u, _ := st.CreateUser("anna", "secret123", false)
	h := strings.Repeat("a", 43)
	pair(t, st, u.ID, h)
	rl.SetGone(h)
	if err := n.SendUser(context.Background(), u.ID, push.Message{V: 1, Title: "x"}); err != nil {
		t.Fatal(err)
	}
	if st.CountPushIOS() != 0 {
		t.Fatal("gone handle kept")
	}
}

func TestLongMessageIsShortened(t *testing.T) {
	st, rl, n := setup(t)
	u, _ := st.CreateUser("anna", "secret123", false)
	pair(t, st, u.ID, strings.Repeat("a", 43))
	if err := n.SendUser(context.Background(), u.ID, push.Message{V: 1, Title: strings.Repeat("ő", 900), Body: strings.Repeat("é", 2000)}); err != nil {
		t.Fatal(err)
	}
	if sent, _, _ := rl.Snapshot(); len(sent) != 1 || len(sent[0].Payload) > 2048 {
		t.Fatal("not sent within the limit")
	}
}

func itoa(n int64) string { return fmtInt(n) }

func fmtInt(n int64) string { b, _ := json.Marshal(n); return string(b) }
