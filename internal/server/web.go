package server

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/ScotyLucas/kinora/web"
)

// webHandler serves the single page app (embedded, or from --web for development).
func (s *Server) webHandler() http.Handler {
	var root fs.FS
	if s.cfg.WebDir != "" {
		root = os.DirFS(s.cfg.WebDir)
		s.log.Info("serving web client from disk", "dir", s.cfg.WebDir)
	} else {
		sub, err := fs.Sub(web.Static, "static")
		if err != nil {
			panic(err)
		}
		root = sub
	}
	files := http.FileServer(http.FS(root))
	dev := s.cfg.WebDir != ""
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" || p == "." {
			p = "index.html"
		}
		if _, err := fs.Stat(root, p); err != nil {
			// SPA uses hash routing; unknown paths get the shell.
			p = "index.html"
			r.URL.Path = "/"
		}
		switch {
		case dev || p == "index.html":
			w.Header().Set("Cache-Control", "no-cache")
		case strings.HasPrefix(p, "vendor/"):
			w.Header().Set("Cache-Control", "public, max-age=604800")
		default:
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}
