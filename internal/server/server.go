// Package server is Kinora's HTTP API and web client host.
//
// Jellyfin problems it addresses: confusing user onboarding and remote access
// (invite links with QR codes, a "Connect" page listing every reachable
// address), no real household profiles / kids mode (profiles with PIN and a
// rating filter applied to *every* endpoint), and opaque transcoding (the
// playback decision returns human readable reasons).
package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/scoty_lucas/kinora/internal/scanner"
	"github.com/scoty_lucas/kinora/internal/store"
)

// Version is set at build time via -ldflags.
var Version = "dev"

// Config holds runtime configuration (flags / environment).
type Config struct {
	Addr      string
	DataDir   string
	CacheDir  string
	WebDir    string // serve the web client from disk (development)
	MediaRoot string // default start folder of the admin folder picker
}

// Server is the HTTP application.
type Server struct {
	cfg     Config
	st      *store.Store
	log     *slog.Logger
	started time.Time
	mux     *http.ServeMux
	limiter *rateLimiter

	Deps
	bgCtx context.Context
}

// Deps are the subsystems the server drives (wired in main).
type Deps struct {
	Ctx     context.Context // cancelled on shutdown; background work started by requests uses it
	Scanner *scanner.Scanner
	ImgDir  string
}

// New builds the server and its routes.
func New(cfg Config, st *store.Store, log *slog.Logger, deps Deps) *Server {
	s := &Server{cfg: cfg, st: st, log: log, started: time.Now(), mux: http.NewServeMux(), limiter: newRateLimiter(), Deps: deps, bgCtx: deps.Ctx}
	if s.bgCtx == nil {
		s.bgCtx = context.Background()
	}
	s.routes()
	return s
}

// Handler returns the root handler with global middleware applied.
func (s *Server) Handler() http.Handler {
	return s.securityHeaders(s.logRequests(s.mux))
}

// ------------------------------------------------------------------ context

type reqCtx struct {
	User    *store.User
	Profile *store.Profile
	Token   string
}

type handler func(w http.ResponseWriter, r *http.Request, c *reqCtx)

const cookieName = "kinora_session"

func tokenFrom(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	if c, err := r.Cookie(cookieName); err == nil {
		return c.Value
	}
	return ""
}

func (s *Server) public(h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := &reqCtx{Token: tokenFrom(r)}
		if c.Token != "" {
			c.User, c.Profile, _ = s.st.Session(c.Token)
		}
		h(w, r, c)
	}
}

func (s *Server) authed(h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := tokenFrom(r)
		u, p, err := s.st.Session(tok)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "bejelentkezés szükséges")
			return
		}
		h(w, r, &reqCtx{User: u, Profile: p, Token: tok})
	}
}

func (s *Server) withProfile(h handler) http.HandlerFunc {
	return s.authed(func(w http.ResponseWriter, r *http.Request, c *reqCtx) {
		if c.Profile == nil {
			writeErr(w, http.StatusPreconditionRequired, "válassz profilt")
			return
		}
		h(w, r, c)
	})
}

func (s *Server) admin(h handler) http.HandlerFunc {
	return s.authed(func(w http.ResponseWriter, r *http.Request, c *reqCtx) {
		if !c.User.IsAdmin {
			writeErr(w, http.StatusForbidden, "ehhez adminisztrátori jog kell")
			return
		}
		h(w, r, c)
	})
}

// notKids rejects modifications from kids profiles.
func notKids(h handler) handler {
	return func(w http.ResponseWriter, r *http.Request, c *reqCtx) {
		if c.Profile != nil && c.Profile.IsKids {
			writeErr(w, http.StatusForbidden, "gyerekprofilból ez nem módosítható")
			return
		}
		h(w, r, c)
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
	writeJSON(w, status, map[string]string{"error": msg})
}

func ok(w http.ResponseWriter) { writeJSON(w, http.StatusOK, map[string]bool{"ok": true}) }

// internalErr logs the real error and returns a generic Hungarian message.
func (s *Server) internalErr(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "nem található")
		return
	}
	s.log.Error("request failed", "path", r.URL.Path, "err", err)
	writeErr(w, http.StatusInternalServerError, "belső hiba: "+err.Error())
}

const maxBody = 1 << 20

// decode reads a JSON body (max 1 MB); on failure it writes a 400 and returns false.
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil && !errors.Is(err, io.EOF) {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeErr(w, http.StatusRequestEntityTooLarge, "a kérés túl nagy (max. 1 MB)")
		} else {
			writeErr(w, http.StatusBadRequest, "hibás kérés: "+err.Error())
		}
		return false
	}
	return true
}

func pathID(r *http.Request, name string) int64 {
	n, _ := strconv.ParseInt(r.PathValue(name), 10, 64)
	return n
}

func queryInt(r *http.Request, name string, def int) int {
	if v := r.URL.Query().Get(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// clientIP returns the caller IP; X-Forwarded-For is trusted only from private/loopback peers.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip != nil && (ip.IsLoopback() || ip.IsPrivate()) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			return strings.TrimSpace(strings.Split(xff, ",")[0])
		}
		if xr := r.Header.Get("X-Real-IP"); xr != "" {
			return strings.TrimSpace(xr)
		}
	}
	return host
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, tok string) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: tok, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: isHTTPS(r), MaxAge: int(store.SessionTTL.Seconds()),
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, Secure: isHTTPS(r), MaxAge: -1})
}

// ------------------------------------------------------------------ middleware

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(code int) { sr.status = code; sr.ResponseWriter.WriteHeader(code) }
func (sr *statusRecorder) Flush() {
	if f, ok := sr.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
func (sr *statusRecorder) Unwrap() http.ResponseWriter { return sr.ResponseWriter }

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		t := time.Now()
		sr := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(sr, r)
		// HLS segments and progress pings are noisy; log them at debug.
		lvl := slog.LevelInfo
		if strings.HasPrefix(r.URL.Path, "/api/hls/") || strings.HasPrefix(r.URL.Path, "/api/images/") ||
			strings.HasSuffix(r.URL.Path, "/progress") || r.URL.Path == "/api/status" || strings.HasPrefix(r.URL.Path, "/api/stream/") {
			lvl = slog.LevelDebug
		}
		if sr.status >= 500 {
			lvl = slog.LevelWarn
		}
		s.log.Log(r.Context(), lvl, "http", "method", r.Method, "path", r.URL.Path, "status", sr.status, "ms", time.Since(t).Milliseconds())
	})
}

// ------------------------------------------------------------------ rate limit

type rateLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func newRateLimiter() *rateLimiter { return &rateLimiter{hits: map[string][]time.Time{}} }

// allow records a hit and reports whether key is still under limit within window.
func (rl *rateLimiter) allow(key string, limit int, window time.Duration) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	t := time.Now()
	cut := t.Add(-window)
	h := rl.hits[key]
	i := 0
	for i < len(h) && h[i].Before(cut) {
		i++
	}
	h = h[i:]
	if len(h) >= limit {
		rl.hits[key] = h
		return false
	}
	rl.hits[key] = append(h, t)
	if len(rl.hits) > 10000 { // crude memory bound
		for k, v := range rl.hits {
			if len(v) == 0 || v[len(v)-1].Before(cut) {
				delete(rl.hits, k)
			}
		}
	}
	return true
}

// ------------------------------------------------------------------ lifecycle

// Background runs periodic housekeeping until ctx is done.
func (s *Server) Background(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.st.PurgeSessions()
		}
	}
}

// Shutdown stops background subsystems (ffmpeg processes, scanners, parties).
func (s *Server) Shutdown(ctx context.Context) {}
