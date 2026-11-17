package server

import (
	"net/http"
	"strings"
)

// Native apps load their UI from a local origin and call the API cross-origin:
// Capacitor (capacitor://localhost on iOS, http(s)://localhost on Android) and
// the TV web apps (file:// pages send "Origin: null"). These origins get CORS
// for token (Bearer) requests only: no Access-Control-Allow-Credentials is ever
// sent and the cookie is stripped, so the browser session cookie can never be
// used cross-origin.
var appOrigins = map[string]bool{
	"capacitor://localhost": true,
	"ionic://localhost":     true,
	"http://localhost":      true,
	"https://localhost":     true,
	"null":                  true,
	"file://":               true,
}

func parseOrigins(list string) map[string]bool {
	out := map[string]bool{}
	for _, o := range strings.Split(list, ",") {
		if o = strings.TrimRight(strings.TrimSpace(o), "/"); o != "" {
			out[o] = true
		}
	}
	return out
}

func (s *Server) isAppOrigin(o string) bool {
	if appOrigins[o] || s.extraOrigins[o] {
		return true
	}
	// development servers (vite) on any localhost port
	for _, p := range []string{"http://localhost:", "http://127.0.0.1:"} {
		if strings.HasPrefix(o, p) && o != p && strings.Trim(o[len(p):], "0123456789") == "" {
			return true
		}
	}
	return false
}

func sameOrigin(r *http.Request, origin string) bool {
	host := strings.TrimPrefix(strings.TrimPrefix(origin, "http://"), "https://")
	if host == origin {
		return false
	}
	if strings.EqualFold(host, r.Host) {
		return true
	}
	// a reverse proxy that rewrites Host usually reports the original one
	for _, fh := range strings.Split(r.Header.Get("X-Forwarded-Host"), ",") {
		if fh = strings.TrimSpace(fh); fh != "" && strings.EqualFold(host, fh) {
			return true
		}
	}
	return false
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" || !strings.HasPrefix(r.URL.Path, "/api/") || sameOrigin(r, origin) {
			next.ServeHTTP(w, r)
			return
		}
		preflight := r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != ""
		if !s.isAppOrigin(origin) {
			if preflight {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r) // no CORS headers: the browser will not expose the response
			return
		}
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", origin)
		h.Add("Vary", "Origin")
		if preflight {
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Range")
			h.Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.Set("Access-Control-Expose-Headers", "Content-Length, Content-Range, Accept-Ranges")
		r.Header.Del("Cookie") // token auth only
		next.ServeHTTP(w, r)
	})
}
