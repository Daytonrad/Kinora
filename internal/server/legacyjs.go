package server

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"io/fs"
	"net/http"
	"sync"

	"github.com/evanw/esbuild/pkg/api"
)

// legacyEngine is the oldest browser engine the web client still runs on: Chromium 63
// (Samsung Tizen 5.0, 2019 TVs). app.js itself needs Chromium 80 (?. and ??).
const legacyEngine = "63"

// legacyJS caches app.legacy.js: app.js lowered to the syntax old TV engines parse, with
// the polyfills of web/static/legacy-polyfills.js in front. index.html loads it instead of
// app.js when the engine cannot parse ?. – nothing else changes, so the web client stays a
// single source with no build step.
type legacyJS struct {
	mu  sync.Mutex
	sum [32]byte
	out []byte
}

func (l *legacyJS) get(root fs.FS) ([]byte, error) {
	src, err := fs.ReadFile(root, "app.js")
	if err != nil {
		return nil, err
	}
	poly, err := fs.ReadFile(root, "legacy-polyfills.js")
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	h.Write(poly)
	h.Write([]byte{0})
	h.Write(src)
	var sum [32]byte
	copy(sum[:], h.Sum(nil))

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.out != nil && l.sum == sum { // embedded files never change; --web ones do while developing
		return l.out, nil
	}
	out, err := lowerJS(src)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	b.Write(poly)
	b.WriteString("\n")
	b.Write(out)
	l.sum, l.out = sum, b.Bytes()
	return l.out, nil
}

// lowerJS rewrites newer syntax (optional chaining, ??, optional catch binding…) for legacyEngine.
func lowerJS(src []byte) ([]byte, error) {
	res := api.Transform(string(src), api.TransformOptions{
		Loader:  api.LoaderJS,
		Engines: []api.Engine{{Name: api.EngineChrome, Version: legacyEngine}},
		Charset: api.CharsetUTF8,
	})
	if len(res.Errors) > 0 {
		e := res.Errors[0]
		msg := e.Text
		if e.Location != nil {
			msg = e.Location.LineText + ": " + msg
		}
		return nil, errors.New("app.legacy.js: " + msg)
	}
	return res.Code, nil
}

func (s *Server) handleLegacyJS(w http.ResponseWriter, r *http.Request) {
	b, err := s.legacy.get(s.webFS())
	if err != nil {
		s.log.Error("legacy web client", "err", err)
		http.Error(w, "app.legacy.js unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(b)
}
