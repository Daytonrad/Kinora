// Package jfcompat is Kinora's Jellyfin-compatible API: a subset of Jellyfin's public HTTP API, served on its
// own port, so native Jellyfin clients (Infuse, Swiftfin, Findroid, Kodi add-ons…) can browse and play a
// Kinora library. It is written from the public API's observable behaviour; no Jellyfin code is used.
//
// Every request runs as one Kinora profile (a Jellyfin "user" is a profile): the profile's rating limit, the
// account's library restriction and the transcode limits apply exactly as in Kinora's own apps.
// The feature is off unless KINORA_JELLYFIN_COMPAT=1 starts the listener and the admin setting is on.
package jfcompat

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ScotyLucas/kinora/internal/media"
	"github.com/ScotyLucas/kinora/internal/store"
)

// APIVersion is the Jellyfin API level the clients are told (they refuse servers older than 10.8 / 10.9).
// The product name stays "Kinora".
const APIVersion = "10.10.7"

// SettingEnabled is the admin setting that switches the API on (with the KINORA_JELLYFIN_COMPAT listener).
const SettingEnabled = "jellyfin_compat"

// Platform marks the device sessions this API creates (shown in Kinora's device lists).
const Platform = "jellyfin"

// QuickConnect is the part of Kinora's Quick Connect this API uses (implemented by the server package).
type QuickConnect interface {
	Enabled() bool
	Initiate(d store.DeviceInfo) (code, secret string, err error)
	// State: pending | approved | denied | expired
	State(secret string) string
	// Claim consumes an approved code: the account and the profile its approver chose.
	Claim(secret string) (userID, profileID int64, ok bool)
}

// Deps are the parts of Kinora the API works with.
type Deps struct {
	Ctx      context.Context
	Store    *store.Store
	Media    *media.Manager
	Encoders *media.Encoders
	Subs     *media.Subtitles
	ImgDir   string
	Log      *slog.Logger
	Version  string        // Kinora's version
	ServerID func() string // Kinora's server id
	QC       QuickConnect
}

// Server is the API.
type Server struct {
	Deps
	ids   *ids
	mux   *http.ServeMux
	locks *lockout
	plays *playSessions
}

// New makes the API; the id key is created in the database on first use and kept.
func New(d Deps) (*Server, error) {
	key, err := loadKey(d.Store.Setting, d.Store.SetSetting)
	if err != nil {
		return nil, err
	}
	x, err := newIDs(key)
	if err != nil {
		return nil, err
	}
	if d.Ctx == nil {
		d.Ctx = context.Background()
	}
	s := &Server{Deps: d, ids: x, mux: http.NewServeMux(), locks: newLockout(), plays: newPlaySessions()}
	s.routes()
	return s, nil
}

// Enabled reports the admin setting.
func (s *Server) Enabled() bool { return s.Store.Setting(SettingEnabled) == "1" }

// ServeHTTP: off → 503; paths are matched case-insensitively (Jellyfin's routing is), and the old
// "/emby" prefix some clients still send is dropped.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.Enabled() {
		http.Error(w, "the Jellyfin-compatible API is switched off on this Kinora server", http.StatusServiceUnavailable)
		return
	}
	p := strings.ToLower(r.URL.Path)
	for _, pre := range []string{"/emby/", "/mediabrowser/"} {
		if strings.HasPrefix(p, pre) {
			p = p[len(pre)-1:]
		}
	}
	r2 := r.Clone(r.Context())
	r2.URL.Path, r2.URL.RawPath = p, ""
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, X-Emby-Authorization, X-Emby-Token, X-MediaBrowser-Token, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	t := time.Now()
	sr := &statusWriter{ResponseWriter: w, status: 200}
	s.mux.ServeHTTP(sr, r2)
	lvl := slog.LevelDebug
	if sr.status >= 500 {
		lvl = slog.LevelWarn
	}
	s.Log.Log(r.Context(), lvl, "jellyfin-compat", "method", r.Method, "path", r.URL.Path, "status", sr.status, "ms", time.Since(t).Milliseconds())
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(c int) { sw.status = c; sw.ResponseWriter.WriteHeader(c) }
func (sw *statusWriter) Flush() {
	if f, ok := sw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
func (sw *statusWriter) Unwrap() http.ResponseWriter { return sw.ResponseWriter }

// Hijack lets the clients' WebSocket (/socket) upgrade pass through the logging wrapper; gorilla/websocket
// type-asserts http.Hijacker on the writer it is given.
func (sw *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := sw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("hijack not supported")
	}
	c, rw, err := hj.Hijack()
	if err == nil {
		sw.status = http.StatusSwitchingProtocols
	}
	return c, rw, err
}

// ------------------------------------------------------------------ request context

type reqCtx struct {
	User    *store.User
	Profile *store.Profile
	Token   string
	Auth    authHeader
}

type handler func(w http.ResponseWriter, r *http.Request, c *reqCtx)

// authHeader is the MediaBrowser authorization header: Client, Device, DeviceId, Version, Token.
type authHeader struct {
	Client, Device, DeviceID, Version, Token string
}

func parseAuth(r *http.Request) authHeader {
	var a authHeader
	h := r.Header.Get("Authorization")
	if h == "" {
		h = r.Header.Get("X-Emby-Authorization")
	}
	if i := strings.IndexByte(h, ' '); i > 0 {
		scheme := strings.ToLower(h[:i])
		if scheme == "mediabrowser" || scheme == "emby" {
			for _, part := range splitParams(h[i+1:]) {
				k, v, ok := strings.Cut(part, "=")
				if !ok {
					continue
				}
				v = strings.Trim(strings.TrimSpace(v), `"`)
				if u, err := url.QueryUnescape(v); err == nil {
					v = u // clients URL-encode the device name
				}
				switch strings.ToLower(strings.TrimSpace(k)) {
				case "client":
					a.Client = v
				case "device":
					a.Device = v
				case "deviceid":
					a.DeviceID = v
				case "version":
					a.Version = v
				case "token":
					a.Token = v
				}
			}
		}
	}
	for _, alt := range []string{r.Header.Get("X-Emby-Token"), r.Header.Get("X-MediaBrowser-Token"), qv(r, "api_key"), qv(r, "ApiKey")} {
		if a.Token == "" && alt != "" {
			a.Token = alt
		}
	}
	if a.Client == "" {
		a.Client = r.Header.Get("X-Emby-Client")
	}
	if a.Device == "" {
		a.Device = r.Header.Get("X-Emby-Device-Name")
	}
	return a
}

// splitParams splits `a="x, y", b="z"` on the commas outside quotes.
func splitParams(s string) []string {
	var out []string
	in, start := false, 0
	for i, ch := range s {
		switch ch {
		case '"':
			in = !in
		case ',':
			if !in {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}

func clientIP(r *http.Request) string {
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return h
}

func (s *Server) public(h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { h(w, r, &reqCtx{Auth: parseAuth(r)}) }
}

// authed resolves the token to a Kinora session; a Jellyfin session always runs as one profile.
func (s *Server) authed(h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a := parseAuth(r)
		ss, err := s.Store.SessionFull(a.Token, clientIP(r))
		if err != nil || ss.Profile == nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		h(w, r, &reqCtx{User: ss.User, Profile: ss.Profile, Token: a.Token, Auth: a})
	}
}

// streamAuthed authenticates the /Videos/{item}/… requests. Some clients (Kodi's JellyCon) hand the bare
// stream URL to their player, without a token: only the PlaySessionId that PlaybackInfo gave them. Such a
// request is let through as the profile that asked PlaybackInfo, when the play session exists, is for this
// very item, and the token it was issued under is still a valid session of that profile (so signing the
// device out ends it too). Play session ids are 16 random bytes, kept in memory for at most 12 hours.
func (s *Server) streamAuthed(h handler) http.HandlerFunc {
	tokenAuth := s.authed(h)
	return func(w http.ResponseWriter, r *http.Request) {
		if parseAuth(r).Token != "" {
			tokenAuth(w, r)
			return
		}
		ps := s.plays.get(qv(r, "PlaySessionId"))
		item, ok := s.ids.decode(nsItem, r.PathValue("item"))
		if ps == nil || ps.Token == "" || !ok || item != ps.ItemID {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		ss, err := s.Store.SessionFull(ps.Token, clientIP(r))
		if err != nil || ss.Profile == nil || ss.Profile.ID != ps.ProfileID || ss.User.ID != ps.UserID {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		h(w, r, &reqCtx{User: ss.User, Profile: ss.Profile, Token: ps.Token, Auth: parseAuth(r)})
	}
}

// ------------------------------------------------------------------ helpers

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	w.Write([]byte(msg))
}

// qv reads a query parameter case-insensitively (clients send both "UserId" and "userId").
func qv(r *http.Request, name string) string {
	q := r.URL.Query()
	if v := q.Get(name); v != "" {
		return v
	}
	for k, v := range q {
		if strings.EqualFold(k, name) && len(v) > 0 {
			return v[0]
		}
	}
	return ""
}

func qInt(r *http.Request, name string, def int) int {
	if n, err := strconv.Atoi(qv(r, name)); err == nil {
		return n
	}
	return def
}

func qBool(r *http.Request, name string) (bool, bool) {
	v := qv(r, name)
	if v == "" {
		return false, false
	}
	b, err := strconv.ParseBool(v)
	return b, err == nil
}

// qList splits a comma (or pipe) separated parameter.
func qList(r *http.Request, name string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(qv(r, name), func(r rune) bool { return r == ',' || r == '|' }) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func decode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	err := json.NewDecoder(r.Body).Decode(v)
	if err != nil && err.Error() == "EOF" {
		return nil
	}
	return err
}

// ticks converts seconds to Jellyfin's 100 ns ticks.
func ticks(sec float64) int64 { return int64(sec * 1e7) }

func isoTime(unix int64) string {
	if unix <= 0 {
		return ""
	}
	return time.Unix(unix, 0).UTC().Format("2006-01-02T15:04:05.0000000Z")
}

// ------------------------------------------------------------------ login lockout

// lockout counts failed logins per account and IP: after maxFails the pair is refused for lockFor, so a
// four-digit PIN cannot be tried through.
const (
	maxFails = 5
	lockFor  = 15 * time.Minute
	failTTL  = 15 * time.Minute
	// a looser cap per IP alone, against trying many account names
	maxIPFails = 30
)

type lockout struct {
	mu    sync.Mutex
	fails map[string]*failRec
}

type failRec struct {
	n     int
	last  time.Time
	until time.Time
}

func newLockout() *lockout { return &lockout{fails: map[string]*failRec{}} }

func (l *lockout) locked(keys ...string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	for _, k := range keys {
		if f := l.fails[k]; f != nil && now.Before(f.until) {
			return true
		}
	}
	return false
}

func (l *lockout) fail(key string, max int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	f := l.fails[key]
	if f == nil || now.Sub(f.last) > failTTL {
		f = &failRec{}
		l.fails[key] = f
	}
	f.n++
	f.last = now
	if f.n >= max {
		f.until, f.n = now.Add(lockFor), 0
	}
	if len(l.fails) > 10000 {
		for k, v := range l.fails {
			if now.Sub(v.last) > failTTL && now.After(v.until) {
				delete(l.fails, k)
			}
		}
	}
}

func (l *lockout) reset(key string) {
	l.mu.Lock()
	delete(l.fails, key)
	l.mu.Unlock()
}
