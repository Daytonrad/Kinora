package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net/http"
	"path"
	"sort"
	"strings"
)

// Languages for the native apps (iOS, tvOS). They use the same language files as the web client, but only
// the parts that concern them: "app" (the apps' own texts, keyed by their Hungarian source text), "srv" and
// "srvRe" (the server's Hungarian messages) and "locale". A language is the built-in file (hu, en, de) with
// the admin's saved overrides on top, and English below it for whatever it leaves out, the same chain the
// web client uses. The apps keep every language they downloaded, and re-download one when its version changes.

var appLangKeys = []string{"locale", "app", "srv", "srvRe"}

type appLangInfo struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Locale  string `json:"locale,omitempty"`
	Version string `json:"version"`
	Builtin bool   `json:"builtin"`
}

type appLang struct {
	Format  int            `json:"kinora_language"`
	Code    string         `json:"code"`
	Name    string         `json:"name"`
	Version string         `json:"version"`
	Strings map[string]any `json:"strings"`
}

// builtinLangs parses the web client's built-in language files, web/static/i18n/<code>.js. Each is one
// JSON object assigned to window.KINORA_I18N.<code>; a file that does not parse is skipped and logged.
func (s *Server) builtinLangs() map[string]langFile {
	root := s.webFS()
	files, _ := fs.Glob(root, "i18n/*.js")
	out := map[string]langFile{}
	for _, f := range files {
		if path.Base(f) == "custom.js" {
			continue
		}
		b, err := fs.ReadFile(root, f)
		var l langFile
		if err == nil {
			err = json.Unmarshal(jsObject(b), &l)
		}
		if err == nil {
			err = validLang(&l)
		}
		if err != nil || l.Code+".js" != path.Base(f) {
			s.log.Warn("skipping built-in language file", "file", f, "err", err)
			continue
		}
		out[l.Code] = l
	}
	return out
}

// jsObject cuts the object literal out of "… = { … };".
func jsObject(b []byte) []byte {
	i := bytes.Index(b, []byte("= {"))
	j := bytes.LastIndexByte(b, '}')
	if i < 0 || j < i {
		return nil
	}
	return b[i+2 : j+1]
}

func isPluralVal(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	if _, ok := m["other"]; !ok {
		return false
	}
	for k := range m {
		switch k {
		case "zero", "one", "two", "few", "many", "other":
		default:
			if len(k) < 2 || k[0] != '=' || strings.Trim(k[1:], "0123456789") != "" {
				return false
			}
		}
	}
	return true
}

// mergeStrings lays b over a like the web client does: objects merge key by key, except plural forms, which
// replace each other whole; "srvRe" patterns merge by pattern, b's first.
func mergeStrings(a, b map[string]any) map[string]any {
	o := make(map[string]any, len(a)+len(b))
	for k, v := range a {
		o[k] = v
	}
	for k, v := range b {
		if k == "srvRe" {
			if mine, ok := v.([]any); ok {
				if theirs, ok := o[k].([]any); ok {
					seen := map[string]bool{}
					for _, p := range mine {
						if pp, ok := p.([]any); ok && len(pp) > 0 {
							if s, ok := pp[0].(string); ok {
								seen[s] = true
							}
						}
					}
					merged := append([]any{}, mine...)
					for _, p := range theirs {
						if pp, ok := p.([]any); ok && len(pp) > 0 {
							if s, ok := pp[0].(string); ok && seen[s] {
								continue
							}
						}
						merged = append(merged, p)
					}
					o[k] = merged
					continue
				}
			}
		}
		bm, bok := v.(map[string]any)
		am, aok := o[k].(map[string]any)
		if bok && aok && !isPluralVal(v) {
			o[k] = mergeStrings(am, bm)
		} else {
			o[k] = v
		}
	}
	return o
}

// appLangs builds every language the apps can use, sorted: built-in ones first, then by name.
func (s *Server) appLangs() []appLang {
	builtin := s.builtinLangs()
	all := map[string]langFile{}
	for code, l := range builtin {
		all[code] = l
	}
	for _, c := range s.customLangs() {
		if b, ok := all[c.Code]; ok {
			b.Strings = mergeStrings(b.Strings, c.Strings)
			b.Name = c.Name
			all[c.Code] = b
		} else {
			all[c.Code] = c
		}
	}
	pick := func(m map[string]any) map[string]any {
		out := map[string]any{}
		for _, k := range appLangKeys {
			if v, ok := m[k]; ok {
				out[k] = v
			}
		}
		return out
	}
	out := make([]appLang, 0, len(all))
	for code, l := range all {
		str := l.Strings
		if en, ok := all["en"]; ok && code != "en" && code != "hu" {
			str = mergeStrings(en.Strings, str)
		}
		str = pick(str)
		b, _ := json.Marshal(str) // map keys are sorted, so the hash is stable
		sum := sha256.Sum256(append([]byte(l.Name+"\x00"), b...))
		out = append(out, appLang{Format: langFormat, Code: code, Name: l.Name, Version: hex.EncodeToString(sum[:8]), Strings: str})
	}
	sort.Slice(out, func(i, j int) bool {
		_, bi := builtin[out[i].Code]
		_, bj := builtin[out[j].Code]
		if bi != bj {
			return bi
		}
		if bi {
			return out[i].Code < out[j].Code
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (s *Server) handleAppLangs(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	builtin := s.builtinLangs()
	list := []appLangInfo{}
	for _, l := range s.appLangs() {
		loc, _ := l.Strings["locale"].(string)
		_, b := builtin[l.Code]
		list = append(list, appLangInfo{Code: l.Code, Name: l.Name, Locale: loc, Version: l.Version, Builtin: b})
	}
	writeJSON(w, http.StatusOK, map[string]any{"languages": list})
}

func (s *Server) handleAppLang(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	code := strings.ToLower(r.PathValue("code"))
	for _, l := range s.appLangs() {
		if l.Code == code {
			writeJSON(w, http.StatusOK, l)
			return
		}
	}
	writeErr(w, http.StatusNotFound, "nincs ilyen nyelv")
}
