// Package push delivers notifications to the official iOS app through the push
// relay (docs/PUSH_RELAY.md). The relay holds the APNs key; this server only knows
// per-device handles and encrypts every payload with a key shared with the device.
package push

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ScotyLucas/kinora/internal/store"
)

// Settings the relay credentials are kept under (not shown in the admin settings).
const (
	keyServerID = "push_server_id"
	keySecret   = "push_server_secret"
	keyRelayFor = "push_server_relay" // relay URL the credentials belong to
)

var (
	// ErrGone means the relay forgot the device (APNs said the token is dead).
	ErrGone = errors.New("push: a készülék már nincs regisztrálva")
	// ErrDisabled means push is off or no relay is configured.
	ErrDisabled = errors.New("push: nincs bekapcsolva")
)

// RelayError is any other non-2xx answer of the relay.
type RelayError struct {
	Status int
	Code   string
}

func (e *RelayError) Error() string { return fmt.Sprintf("push relay %d: %s", e.Status, e.Code) }

// Relay talks to the push relay with the credentials stored in the settings.
type Relay struct {
	st  *store.Store
	hc  *http.Client
	mu  sync.Mutex // serialises registration
	now func() time.Time
}

// NewRelay makes a relay client.
func NewRelay(st *store.Store) *Relay {
	return &Relay{st: st, hc: &http.Client{Timeout: 15 * time.Second}, now: time.Now}
}

// URL is the configured relay base URL ("" = none).
func (r *Relay) URL() string { return strings.TrimRight(strings.TrimSpace(r.st.Setting("push_relay_url")), "/") }

// Enabled reports whether push is switched on and a relay is configured.
func (r *Relay) Enabled() bool { return r.st.Setting("push_enabled") == "1" && r.URL() != "" }

// ServerID returns this server's relay id, registering with the relay on first use
// or after the relay URL changed.
func (r *Relay) ServerID(ctx context.Context) (string, error) {
	id, _, err := r.creds(ctx)
	return id, err
}

func (r *Relay) creds(ctx context.Context) (id, secret string, err error) {
	if !r.Enabled() {
		return "", "", ErrDisabled
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	url := r.URL()
	id, secret = r.st.Setting(keyServerID), r.st.Setting(keySecret)
	if id != "" && secret != "" && r.st.Setting(keyRelayFor) == url {
		return id, secret, nil
	}
	var out struct {
		ServerID string `json:"server_id"`
		Secret   string `json:"server_secret"`
	}
	name, _ := json.Marshal(map[string]string{"name": truncate(r.st.Setting("server_name"), 64)})
	if err := r.do(ctx, http.MethodPost, url+"/v1/servers", name, nil, &out); err != nil {
		return "", "", err
	}
	if out.ServerID == "" || out.Secret == "" {
		return "", "", errors.New("push relay: hiányos regisztrációs válasz")
	}
	for k, v := range map[string]string{keyServerID: out.ServerID, keySecret: out.Secret, keyRelayFor: url} {
		if err := r.st.SetSetting(k, v); err != nil {
			return "", "", err
		}
	}
	return out.ServerID, out.Secret, nil
}

// Send delivers an encrypted payload to one handle.
func (r *Relay) Send(ctx context.Context, handle string, payload []byte, collapseID string) error {
	body := map[string]string{"handle": handle, "payload": b64url(payload)}
	if collapseID != "" {
		body["collapse_id"] = truncate(collapseID, 64)
	}
	raw, _ := json.Marshal(body)
	return r.signed(ctx, http.MethodPost, "/v1/send", raw)
}

// Delete tells the relay to forget a handle.
func (r *Relay) Delete(ctx context.Context, handle string) error {
	return r.signed(ctx, http.MethodDelete, "/v1/devices/"+handle, nil)
}

func (r *Relay) signed(ctx context.Context, method, path string, body []byte) error {
	id, secret, err := r.creds(ctx)
	if err != nil {
		return err
	}
	ts := strconv.FormatInt(r.now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "\n" + method + "\n" + path + "\n"))
	mac.Write(body)
	h := map[string]string{"x-kinora-server": id, "x-kinora-timestamp": ts, "x-kinora-signature": hex.EncodeToString(mac.Sum(nil))}
	err = r.do(ctx, method, r.URL()+path, body, h, nil)
	var re *RelayError
	if errors.As(err, &re) && re.Status == http.StatusUnauthorized && re.Code == "unauthorized" {
		// the relay does not know us (its database or secret key was reset): register again next time
		r.st.SetSetting(keyServerID, "")
	}
	return err
}

func (r *Relay) do(ctx context.Context, method, url string, body []byte, headers map[string]string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Kinora")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := r.hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if res.StatusCode == http.StatusGone {
		return ErrGone
	}
	if res.StatusCode/100 != 2 {
		var e struct {
			Error string `json:"error"`
		}
		json.Unmarshal(data, &e)
		return &RelayError{Status: res.StatusCode, Code: e.Error}
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

// Seal encrypts with AES-256-GCM in CryptoKit's combined layout: nonce(12) || ciphertext || tag(16).
func Seal(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
