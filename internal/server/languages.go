package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Web-client languages added or overridden by an admin. The built-in ones (hu, en, de) are embedded files in
// web/static/i18n; these live in <data>/i18n/<code>.json, in the same format the admin page exports.

const langFormat = 1

var (
	langCode   = regexp.MustCompile(`^[a-z]{2,3}(-[a-z0-9]{2,8})?$`)
	langLocale = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)
)

type langFile struct {
	Format    int            `json:"kinora_language"`
	Code      string         `json:"code"`
	Name      string         `json:"name"`
	Strings   map[string]any `json:"strings"`
	UpdatedAt int64          `json:"updated_at,omitempty"`
}

func (s *Server) langDir() string { return filepath.Join(s.cfg.DataDir, "i18n") }

// checkLangValue bounds what a translation may contain: JSON objects, arrays, strings and scalars, not too deep or big.
func checkLangValue(v any, depth int, leaves *int) error {
	if depth > 5 {
		return errors.New("a nyelvi fájl túl mélyen egymásba ágyazott")
	}
	switch x := v.(type) {
	case string:
		*leaves++
		if len(x) > 4000 {
			return errors.New("túl hosszú szöveg a nyelvi fájlban")
		}
	case map[string]any:
		for k, e := range x {
			if len(k) > 600 {
				return errors.New("túl hosszú kulcs a nyelvi fájlban")
			}
			if err := checkLangValue(e, depth+1, leaves); err != nil {
				return err
			}
		}
	case []any:
		for _, e := range x {
			if err := checkLangValue(e, depth+1, leaves); err != nil {
				return err
			}
		}
	case float64, bool, nil:
		*leaves++
	default:
		return errors.New("ismeretlen érték a nyelvi fájlban")
	}
	if *leaves > 10000 {
		return errors.New("túl sok szöveg a nyelvi fájlban")
	}
	return nil
}

func validLang(l *langFile) error {
	l.Code = strings.ToLower(strings.TrimSpace(l.Code))
	l.Name = strings.TrimSpace(l.Name)
	if !langCode.MatchString(l.Code) {
		return errors.New("érvénytelen nyelvkód (pl. fr vagy pt-br)")
	}
	if n := utf8.RuneCountInString(l.Name); n < 1 || n > 40 {
		return errors.New("a nyelv neve 1–40 karakter lehet")
	}
	if l.Format != 0 && l.Format != langFormat {
		return errors.New("ismeretlen nyelvifájl-formátum")
	}
	if len(l.Strings) == 0 {
		return errors.New("a nyelvi fájlban nincsenek szövegek")
	}
	if loc, ok := l.Strings["locale"]; ok {
		if str, ok := loc.(string); !ok || !langLocale.MatchString(str) {
			return errors.New("érvénytelen locale a nyelvi fájlban (pl. fr-FR)")
		}
	}
	leaves := 0
	return checkLangValue(map[string]any(l.Strings), 0, &leaves)
}

// customLangs returns the saved languages, sorted by code. Unreadable files are skipped and logged.
func (s *Server) customLangs() []langFile {
	files, _ := filepath.Glob(filepath.Join(s.langDir(), "*.json"))
	sort.Strings(files)
	out := []langFile{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		var l langFile
		if err == nil {
			err = json.Unmarshal(b, &l)
		}
		if err == nil {
			err = validLang(&l)
		}
		if err != nil || l.Code+".json" != filepath.Base(f) {
			s.log.Warn("skipping language file", "file", f, "err", err)
			continue
		}
		out = append(out, l)
	}
	return out
}

// handleCustomLangsJS serves the saved languages as a script, so the client has them before it starts.
func (s *Server) handleCustomLangsJS(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	b, err := json.Marshal(s.customLangs()) // escapes <, > and & as well
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write([]byte("window.KINORA_I18N_CUSTOM = "))
	w.Write(b)
	w.Write([]byte(";\n"))
}

func (s *Server) handleLangPut(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var l langFile
	if !decode(w, r, &l) {
		return
	}
	code := strings.ToLower(r.PathValue("code"))
	if l.Code == "" {
		l.Code = code
	}
	if err := validLang(&l); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if l.Code != code {
		writeErr(w, http.StatusBadRequest, "a fájl nyelvkódja nem egyezik")
		return
	}
	l.Format, l.UpdatedAt = langFormat, time.Now().Unix()
	b, err := json.MarshalIndent(l, "", " ")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := os.MkdirAll(s.langDir(), 0o755); err != nil {
		s.internalErr(w, r, err)
		return
	}
	dst := filepath.Join(s.langDir(), l.Code+".json")
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		s.internalErr(w, r, err)
		return
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": l.Code, "name": l.Name, "updated_at": l.UpdatedAt})
}

func (s *Server) handleLangDelete(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	code := strings.ToLower(r.PathValue("code"))
	if !langCode.MatchString(code) {
		writeErr(w, http.StatusBadRequest, "érvénytelen nyelvkód (pl. fr vagy pt-br)")
		return
	}
	if err := os.Remove(filepath.Join(s.langDir(), code+".json")); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
