package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ScotyLucas/kinora/internal/media"
)

const maxLogoBytes = 2 << 20

// sniffLogo returns the logo format from its content (never from the file name).
func sniffLogo(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return "png"
	case bytes.HasPrefix(b, []byte{0xFF, 0xD8, 0xFF}):
		return "jpg"
	case len(b) > 12 && bytes.Equal(b[:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")):
		return "webp"
	}
	t := bytes.TrimLeft(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf")), " \t\r\n")
	if bytes.HasPrefix(t, []byte("<svg")) || (bytes.HasPrefix(t, []byte("<?xml")) && bytes.Contains(t, []byte("<svg"))) {
		return "svg"
	}
	return ""
}

// unsafeSVG lists what an uploaded SVG may not contain: scripts, event handlers,
// external references, embedded documents, entities. A violating SVG is rejected
// (rewriting would be guesswork); the logo is also only ever served as an image
// with a script-free CSP.
var unsafeSVG = []*regexp.Regexp{
	regexp.MustCompile(`(?i)<\s*script`),
	regexp.MustCompile(`(?i)\son[a-z]+\s*=`),
	regexp.MustCompile(`(?i)javascript\s*:`),
	regexp.MustCompile(`(?i)<\s*(foreignobject|iframe|embed|object|audio|video|use)\b`),
	regexp.MustCompile(`(?i)<!\s*(entity|doctype)`),
	regexp.MustCompile(`(?i)<\?xml-stylesheet`),
	regexp.MustCompile(`(?i)(href|src)\s*=\s*["']?\s*(https?:|//)`),
	regexp.MustCompile(`(?i)url\(\s*["']?\s*(https?:|//)`),
	regexp.MustCompile(`(?i)@import`),
}

// reDataRef finds data: references; only embedded raster images are allowed.
var (
	reDataRef  = regexp.MustCompile(`(?i)(href|src)\s*=\s*["']?\s*data:([a-z0-9.+/-]*)`)
	safeDataCT = map[string]bool{"image/png": true, "image/jpeg": true, "image/webp": true, "image/gif": true}
)

func checkSVG(b []byte) error {
	bad := errors.New("az SVG nem biztonságos (szkriptet, eseménykezelőt vagy külső hivatkozást tartalmaz) – tölts fel PNG-t vagy egyszerű SVG-t")
	for _, re := range unsafeSVG {
		if re.Match(b) {
			return bad
		}
	}
	for _, m := range reDataRef.FindAllSubmatch(b, -1) {
		if !safeDataCT[strings.ToLower(string(m[2]))] {
			return bad
		}
	}
	return nil
}

// ffmpegImage renders src into a PNG of the given box (contain, centred on bg; bg ""
// keeps transparency and only bounds the size).
func ffmpegImage(src, dst string, w, h int, bg string) error {
	vf := fmt.Sprintf("scale='min(%d,iw)':'min(%d,ih)':force_original_aspect_ratio=decrease", w, h)
	if bg != "" {
		vf = fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=%s", w, h, w, h, bg)
	}
	out, err := exec.Command(media.FFmpeg, "-hide_banner", "-loglevel", "error", "-y", "-i", src, "-vf", vf, "-frames:v", "1", dst).CombinedOutput()
	if err != nil {
		return fmt.Errorf("a kép nem alakítható át: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// installLogo validates and stores a logo variant (light|dark) and regenerates the
// favicon and app icons from it.
func (s *Server) installLogo(b []byte, variant string) error {
	if variant != "light" && variant != "dark" {
		return errors.New("a változat light vagy dark lehet")
	}
	if len(b) > maxLogoBytes {
		return errors.New("a logó legfeljebb 2 MB lehet")
	}
	format := sniffLogo(b)
	if format == "" {
		return errors.New("csak PNG, JPG, WebP vagy SVG logó tölthető fel")
	}
	if format == "svg" {
		if err := checkSVG(b); err != nil {
			return err
		}
	}
	dir := s.brandingDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	src := filepath.Join(dir, "source-"+variant+"."+format)
	for _, ext := range []string{"png", "jpg", "webp", "svg"} {
		os.Remove(filepath.Join(dir, "source-"+variant+"."+ext))
	}
	if err := os.WriteFile(src, b, 0o644); err != nil {
		return err
	}
	out := filepath.Join(dir, "logo-"+variant+".png")
	if err := ffmpegImage(src, out, 1024, 1024, ""); err != nil {
		if format != "svg" {
			return err
		}
		// ffmpeg without SVG support: serve the (checked) SVG itself as an image
		os.Remove(out)
		return os.WriteFile(filepath.Join(dir, "logo.svg"), b, 0o644)
	}
	return s.renderIcons()
}

// renderIcons makes favicon + PWA icons from the dark logo (else the light one).
func (s *Server) renderIcons() error {
	dir := s.brandingDir()
	src := filepath.Join(dir, "logo-dark.png")
	if !fileExists(src) {
		src = filepath.Join(dir, "logo-light.png")
	}
	if !fileExists(src) {
		for _, f := range []string{"favicon.png", "icon-192.png", "icon-512.png"} {
			os.Remove(filepath.Join(dir, f))
		}
		return nil
	}
	for name, size := range map[string]int{"favicon.png": 64, "icon-192.png": 192, "icon-512.png": 512} {
		if err := ffmpegImage(src, filepath.Join(dir, name), size, size, "0x141210"); err != nil {
			return err
		}
	}
	return nil
}

func readUpload(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxLogoBytes+64<<10)
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(maxLogoBytes + 64<<10); err != nil {
			return nil, errors.New("a logó legfeljebb 2 MB lehet")
		}
		var f multipart.File
		f, _, err := r.FormFile("file")
		if err != nil {
			return nil, errors.New("hiányzik a fájl (file mező)")
		}
		defer f.Close()
		return io.ReadAll(io.LimitReader(f, maxLogoBytes+1))
	}
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, errors.New("a logó legfeljebb 2 MB lehet")
	}
	return b, nil
}

// POST /api/admin/branding/logo?variant=light|dark – raw body or multipart "file".
func (s *Server) handleLogoUpload(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	b, err := readUpload(w, r)
	if err == nil {
		err = s.installLogo(b, r.URL.Query().Get("variant"))
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.handleBranding(w, r, c)
}

// DELETE /api/admin/branding/logo?variant=light|dark
func (s *Server) handleLogoDelete(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	v := r.URL.Query().Get("variant")
	if v != "light" && v != "dark" {
		writeErr(w, http.StatusBadRequest, "a változat light vagy dark lehet")
		return
	}
	dir := s.brandingDir()
	os.Remove(filepath.Join(dir, "logo-"+v+".png"))
	for _, ext := range []string{"png", "jpg", "webp", "svg"} {
		os.Remove(filepath.Join(dir, "source-"+v+"."+ext))
	}
	if !fileExists(filepath.Join(dir, "logo-light.png")) && !fileExists(filepath.Join(dir, "logo-dark.png")) {
		os.Remove(filepath.Join(dir, "logo.svg"))
	}
	s.renderIcons()
	s.handleBranding(w, r, c)
}

// ApplyStartupBranding imports environment-provided branding:
//   - KINORA_LOGO (Docker): a logo file that is (re)imported whenever it changes;
//   - KINORA_INITIAL_SERVER_NAME / KINORA_INITIAL_LOGO / KINORA_INITIAL_MEDIA (installers):
//     applied once, on the first start, and editable in the admin UI afterwards.
func (s *Server) ApplyStartupBranding() {
	if p := os.Getenv("KINORA_LOGO"); p != "" {
		if b, err := os.ReadFile(p); err != nil {
			s.log.Warn("KINORA_LOGO not readable", "path", p, "err", err)
		} else {
			sum := sha256.Sum256(b)
			h := hex.EncodeToString(sum[:])
			if s.st.Setting("logo_env_sha") != h {
				err1 := s.installLogo(b, "dark")
				err2 := s.installLogo(b, "light")
				if err := errors.Join(err1, err2); err != nil {
					s.log.Warn("KINORA_LOGO rejected", "path", p, "err", err)
				} else {
					s.st.SetSetting("logo_env_sha", h)
					s.log.Info("logo imported", "from", p)
				}
			}
		}
	}
	if s.st.Setting("initial_applied") == "1" {
		return
	}
	defer s.st.SetSetting("initial_applied", "1")
	if n := strings.TrimSpace(os.Getenv("KINORA_INITIAL_SERVER_NAME")); n != "" && !s.st.Overridden("server_name") {
		s.st.SetSetting("server_name", n)
	}
	if p := os.Getenv("KINORA_INITIAL_LOGO"); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			if err := errors.Join(s.installLogo(b, "dark"), s.installLogo(b, "light")); err != nil {
				s.log.Warn("installer logo rejected", "err", err)
			}
		}
	}
	if m := strings.TrimSpace(os.Getenv("KINORA_INITIAL_MEDIA")); m != "" {
		if libs, err := s.st.Libraries(); err == nil && len(libs) == 0 {
			for _, spec := range strings.Split(m, ";") {
				name, path := initialMedia(spec)
				if path == "" || !isDir(path) {
					continue
				}
				if lib, err := s.st.CreateLibrary(name, path, guessKind(name+" "+path)); err == nil {
					s.log.Info("initial library", "name", lib.Name, "path", lib.Path, "kind", lib.Kind)
				}
			}
		}
	}
}

// initialMedia parses "Név=/út" or "/út".
func initialMedia(spec string) (name, path string) {
	spec = strings.TrimSpace(spec)
	if i := strings.Index(spec, "="); i > 0 && !strings.HasPrefix(spec, "/") {
		name, path = strings.TrimSpace(spec[:i]), strings.TrimSpace(spec[i+1:])
	} else {
		path = spec
	}
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return "", ""
	}
	if name == "" {
		name = filepath.Base(path)
	}
	return name, path
}

var (
	reShows  = regexp.MustCompile(`(?i)sorozat|series|shows|\btv\b|anime`)
	reMovies = regexp.MustCompile(`(?i)film|movie|mozi`)
)

func guessKind(s string) string {
	switch {
	case reShows.MatchString(s):
		return "shows"
	case reMovies.MatchString(s):
		return "movies"
	}
	return "mixed"
}

// GET /manifest.json – the PWA manifest with the server's name and icons (the web
// client is installable on iPhones/Android from the browser).
func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	name := s.st.Setting("server_name")
	icons := []map[string]string{
		{"src": "icons/icon-192.png", "sizes": "192x192", "type": "image/png", "purpose": "any"},
		{"src": "icons/icon-512.png", "sizes": "512x512", "type": "image/png", "purpose": "any"},
		{"src": "icons/maskable-512.png", "sizes": "512x512", "type": "image/png", "purpose": "maskable"},
	}
	if u := s.brandURL("icon-192.png"); u != "" {
		icons = []map[string]string{
			{"src": strings.TrimPrefix(u, "/"), "sizes": "192x192", "type": "image/png", "purpose": "any"},
			{"src": strings.TrimPrefix(s.brandURL("icon-512.png"), "/"), "sizes": "512x512", "type": "image/png", "purpose": "any maskable"},
		}
	}
	w.Header().Set("Content-Type", "application/manifest+json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.Encode(map[string]any{
		"name": name, "short_name": name, "description": "Saját médiaszerver filmekhez és sorozatokhoz", "lang": "hu",
		"start_url": "./#/home", "scope": "./", "display": "standalone", "orientation": "any",
		"background_color": "#141210", "theme_color": "#141210", "icons": icons,
	})
}
