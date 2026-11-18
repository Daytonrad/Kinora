package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func TestCustomLanguages(t *testing.T) {
	c, _, srv := newTestServerFull(t)
	fr := map[string]any{"kinora_language": 1, "code": "fr", "name": "Français",
		"strings": map[string]any{"locale": "fr-FR", "home": "Accueil", "epCount": map[string]any{"one": "{0} épisode", "other": "{0} épisodes"},
			"srvRe": []any{[]any{"^(.+) csatlakozott$", "{0} a rejoint"}}, "bitrateOpts": []any{[]any{0, "Original"}}}}
	if code := c.do("PUT", "/api/admin/languages/fr", fr, nil); code != 200 {
		t.Fatalf("put fr: %d", code)
	}
	// the script the client loads before app.js carries the saved language
	resp, err := http.Get(c.base + "/i18n/custom.js")
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}
	resp.Body.Close()
	js := b.String()
	if !strings.HasPrefix(js, "window.KINORA_I18N_CUSTOM = ") || !strings.HasSuffix(js, ";\n") || !strings.Contains(resp.Header.Get("Content-Type"), "javascript") {
		t.Fatalf("custom.js: %q %s", js, resp.Header.Get("Content-Type"))
	}
	var langs []langFile
	if err := json.Unmarshal([]byte(strings.TrimSuffix(strings.TrimPrefix(js, "window.KINORA_I18N_CUSTOM = "), ";\n")), &langs); err != nil {
		t.Fatal(err)
	}
	if len(langs) != 1 || langs[0].Code != "fr" || langs[0].Name != "Français" || langs[0].Strings["home"] != "Accueil" || langs[0].UpdatedAt == 0 {
		t.Fatalf("langs: %+v", langs)
	}
	// "</script>" in a translation must not be able to end a script block
	evil := map[string]any{"name": "Evil", "strings": map[string]any{"home": "</script><script>alert(1)</script>"}}
	if code := c.do("PUT", "/api/admin/languages/xx", evil, nil); code != 200 {
		t.Fatalf("put xx: %d", code)
	}
	if _, body := c.raw("GET", "/i18n/custom.js", nil); strings.Contains(string(body), "</script>") {
		t.Fatalf("unescaped markup: %s", body)
	}
	// invalid uploads
	for name, bad := range map[string]map[string]any{
		"code mismatch":  {"code": "de", "name": "X", "strings": map[string]any{"home": "x"}},
		"no name":        {"name": "", "strings": map[string]any{"home": "x"}},
		"no strings":     {"name": "X"},
		"bad locale":     {"name": "X", "strings": map[string]any{"locale": "fr FR; x"}},
		"future format":  {"kinora_language": 9, "name": "X", "strings": map[string]any{"home": "x"}},
		"too long value": {"name": "X", "strings": map[string]any{"home": strings.Repeat("a", 5000)}},
	} {
		if code := c.do("PUT", "/api/admin/languages/fr", bad, nil); code != 400 {
			t.Errorf("%s: %d", name, code)
		}
	}
	for _, code := range []string{"FRENCH", "f", "fr_FR", "..", "fr-"} {
		if got := c.do("PUT", "/api/admin/languages/"+code, map[string]any{"name": "X", "strings": map[string]any{"home": "x"}}, nil); got == 200 {
			t.Errorf("code %q accepted: %d", code, got)
		}
	}
	// only admins may change languages, but anyone may load them
	anon := &client{t: t, base: c.base, hc: &http.Client{}}
	if code := anon.do("PUT", "/api/admin/languages/fr", fr, nil); code != 401 {
		t.Fatalf("anonymous put: %d", code)
	}
	if code := anon.do("GET", "/i18n/custom.js", nil, nil); code != 200 {
		t.Fatalf("anonymous custom.js: %d", code)
	}
	if code := c.do("DELETE", "/api/admin/languages/fr", nil, nil); code != 200 || fileExists(filepath.Join(srv.langDir(), "fr.json")) {
		t.Fatalf("delete: %d", code)
	}
	if got := srv.customLangs(); len(got) != 1 || got[0].Code != "xx" {
		t.Fatalf("after delete: %+v", got)
	}
}
