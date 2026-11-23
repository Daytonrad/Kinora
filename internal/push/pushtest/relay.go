// Package pushtest is an in-memory push relay for tests: it checks the request
// signatures the way the real relay does and records what was sent.
package pushtest

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Sent is one accepted /v1/send.
type Sent struct {
	Handle, CollapseID string
	Payload            []byte
}

// Relay is a fake relay.
type Relay struct {
	*httptest.Server
	mu            sync.Mutex
	secrets       map[string]string // server_id -> secret
	Registrations int
	Sent          []Sent
	Deleted       []string
	Gone          map[string]bool // handles answered with 410
	Status        int             // when set, every send is answered with it
}

// New starts a fake relay, closed with the test.
func New(t *testing.T) *Relay {
	r := &Relay{secrets: map[string]string{}, Gone: map[string]bool{}}
	r.Server = httptest.NewServer(http.HandlerFunc(r.serve))
	t.Cleanup(r.Close)
	return r
}

// Snapshot returns copies of what was recorded.
func (r *Relay) Snapshot() (sent []Sent, deleted []string, regs int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Sent(nil), r.Sent...), append([]string(nil), r.Deleted...), r.Registrations
}

// SetGone makes sends to a handle answer 410.
func (r *Relay) SetGone(h string) { r.mu.Lock(); r.Gone[h] = true; r.mu.Unlock() }

// Fail makes every send answer with status (0 = back to normal).
func (r *Relay) Fail(status int) { r.mu.Lock(); r.Status = status; r.mu.Unlock() }

// Forget drops every server registration (as if the relay's secret key changed).
func (r *Relay) Forget() { r.mu.Lock(); r.secrets = map[string]string{}; r.mu.Unlock() }

func fail(w http.ResponseWriter, code int, e string) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": e})
}

func (r *Relay) serve(w http.ResponseWriter, q *http.Request) {
	body, _ := io.ReadAll(q.Body)
	r.mu.Lock()
	defer r.mu.Unlock()
	if q.Method == http.MethodPost && q.URL.Path == "/v1/servers" {
		r.Registrations++
		id := "srv_" + strconv.Itoa(r.Registrations)
		r.secrets[id] = "secret-" + id
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"server_id": id, "server_secret": r.secrets[id]})
		return
	}
	secret, known := r.secrets[q.Header.Get("x-kinora-server")]
	ts, _ := strconv.ParseInt(q.Header.Get("x-kinora-timestamp"), 10, 64)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(q.Header.Get("x-kinora-timestamp") + "\n" + q.Method + "\n" + q.URL.Path + "\n"))
	mac.Write(body)
	sig, _ := hex.DecodeString(q.Header.Get("x-kinora-signature"))
	if !known || !hmac.Equal(sig, mac.Sum(nil)) || time.Since(time.Unix(ts, 0)).Abs() > 300*time.Second {
		fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	switch {
	case q.Method == http.MethodPost && q.URL.Path == "/v1/send":
		var in struct{ Handle, Payload, Collapse_id string }
		json.Unmarshal(body, &in)
		p, err := base64.RawURLEncoding.DecodeString(in.Payload)
		switch {
		case err != nil || len(p) < 28:
			fail(w, http.StatusBadRequest, "bad_payload")
		case len(p) > 2048:
			fail(w, http.StatusRequestEntityTooLarge, "payload_too_large")
		case r.Gone[in.Handle]:
			fail(w, http.StatusGone, "gone")
		case r.Status != 0:
			fail(w, r.Status, "test")
		default:
			r.Sent = append(r.Sent, Sent{Handle: in.Handle, CollapseID: in.Collapse_id, Payload: p})
			w.Write([]byte(`{"ok":true}`))
		}
	case q.Method == http.MethodDelete && strings.HasPrefix(q.URL.Path, "/v1/devices/"):
		r.Deleted = append(r.Deleted, strings.TrimPrefix(q.URL.Path, "/v1/devices/"))
		w.Write([]byte(`{"ok":true}`))
	default:
		fail(w, http.StatusNotFound, "not_found")
	}
}

// Open decrypts a payload the way the iOS app does (CryptoKit AES.GCM combined).
func Open(key, sealed []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, sealed[:12], sealed[12:], nil)
}
