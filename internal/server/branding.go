package server

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"github.com/ScotyLucas/kinora/internal/store"
)

// APIVersion is bumped when the API changes in a way clients must know about.
// MinClientAPI is the oldest client API level this server still serves.
const (
	APIVersion   = 2
	MinClientAPI = 1
)

var reHexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// serverID is a stable random identifier (lets apps recognise a server whose address changed).
func (s *Server) serverID() string {
	id := s.st.Setting("server_id")
	if len(id) != 16 {
		id = store.RandomToken(8)
		s.st.SetSetting("server_id", id)
	}
	return id
}

// brandingFiles are the logo variants the server may hold in <data>/branding.
var brandingFiles = map[string]string{
	"logo-light.png": "image/png", "logo-dark.png": "image/png", "favicon.png": "image/png",
	"icon-192.png": "image/png", "icon-512.png": "image/png", "logo.svg": "image/svg+xml",
}

func (s *Server) brandingDir() string { return filepath.Join(s.cfg.DataDir, "branding") }

func (s *Server) brandingFile(name string) string {
	if _, ok := brandingFiles[name]; !ok {
		return ""
	}
	p := filepath.Join(s.brandingDir(), name)
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

func (s *Server) brandURL(name string) string {
	if s.brandingFile(name) == "" {
		return ""
	}
	return "/api/branding/" + name
}

func (s *Server) logoURL() string {
	if u := s.brandURL("logo-dark.png"); u != "" {
		return u
	}
	return s.brandURL("logo-light.png")
}

// GET /api/branding – public: name, accent, logos (used by the web client and the apps).
func (s *Server) handleBranding(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	writeJSON(w, http.StatusOK, map[string]any{
		"server_name": s.st.Setting("server_name"),
		"accent":      s.st.Setting("brand_accent"),
		"logo_light":  s.brandURL("logo-light.png"),
		"logo_dark":   s.brandURL("logo-dark.png"),
		"favicon":     s.brandURL("favicon.png"),
		"icon_192":    s.brandURL("icon-192.png"),
		"icon_512":    s.brandURL("icon-512.png"),
	})
}

// GET /api/branding/{file} – public logo files. SVG is only ever served as an
// image with a CSP that forbids scripts, and never sniffed.
func (s *Server) handleBrandingFile(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	name := r.PathValue("file")
	p := s.brandingFile(name)
	if p == "" {
		writeErr(w, http.StatusNotFound, "nincs ilyen fájl")
		return
	}
	h := w.Header()
	h.Set("Content-Type", brandingFiles[name])
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	h.Set("Cache-Control", "public, max-age=300")
	http.ServeFile(w, r, p)
}
