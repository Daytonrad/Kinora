package server

import (
	"net/http"
	"testing"
)

func TestAppLanguages(t *testing.T) {
	c, _, srv := newTestServerFull(t)
	// every built-in web language file parses
	if b := srv.builtinLangs(); b["hu"].Code == "" || b["en"].Code == "" || b["de"].Code == "" {
		t.Fatalf("built-in languages: %d", len(b))
	}
	type info struct {
		Code, Name, Version string
		Builtin             bool
	}
	var list struct{ Languages []info }
	anon := &client{t: t, base: c.base, hc: &http.Client{}}
	if code := anon.do("GET", "/api/languages", nil, &list); code != 200 || len(list.Languages) != 3 || list.Languages[0].Code != "de" || !list.Languages[0].Builtin {
		t.Fatalf("list: %d %+v", code, list)
	}
	versions := map[string]string{}
	for _, l := range list.Languages {
		versions[l.Code] = l.Version
	}
	var en struct {
		Code, Version string
		Strings       map[string]any
	}
	if code := anon.do("GET", "/api/languages/en", nil, &en); code != 200 || en.Version != versions["en"] {
		t.Fatalf("en: %d %s", code, en.Version)
	}
	if _, ok := en.Strings["home"]; ok {
		t.Fatalf("web-only strings in the app language: %v", en.Strings["home"])
	}
	app, _ := en.Strings["app"].(map[string]any)
	if app["Beállítások"] != "Settings" || en.Strings["locale"] != "en-GB" && en.Strings["locale"] != "en-US" {
		t.Fatalf("en app strings: %v %v", app["Beállítások"], en.Strings["locale"])
	}

	// a new language falls back to English; an override of a built-in one changes its version
	fr := map[string]any{"name": "Français", "strings": map[string]any{"locale": "fr-FR", "app": map[string]any{"Beállítások": "Réglages"},
		"srvRe": []any{[]any{"^(.+) csatlakozott$", "{0} a rejoint"}}}}
	if code := c.do("PUT", "/api/admin/languages/fr", fr, nil); code != 200 {
		t.Fatalf("put fr: %d", code)
	}
	if code := c.do("PUT", "/api/admin/languages/en", map[string]any{"name": "English", "strings": map[string]any{"app": map[string]any{"Kész": "Finished"}}}, nil); code != 200 {
		t.Fatalf("put en: %d", code)
	}
	var got struct {
		Name, Version string
		Strings       map[string]any
	}
	if code := anon.do("GET", "/api/languages/fr", nil, &got); code != 200 || got.Name != "Français" {
		t.Fatalf("fr: %d %+v", code, got)
	}
	app, _ = got.Strings["app"].(map[string]any)
	if app["Beállítások"] != "Réglages" || app["Kész"] != "Finished" || app["Mégse"] != "Cancel" || got.Strings["locale"] != "fr-FR" {
		t.Fatalf("fr app strings: %v %v %v", app["Beállítások"], app["Kész"], app["Mégse"])
	}
	if re, _ := got.Strings["srvRe"].([]any); len(re) < 2 || re[0].([]any)[1] != "{0} a rejoint" {
		t.Fatalf("fr patterns: %v", got.Strings["srvRe"])
	}
	list.Languages = nil
	anon.do("GET", "/api/languages", nil, &list)
	for _, l := range list.Languages {
		if l.Code == "en" && l.Version == versions["en"] {
			t.Fatalf("en version did not change")
		}
	}
	if len(list.Languages) != 4 || list.Languages[3].Code != "fr" || list.Languages[3].Builtin {
		t.Fatalf("list after put: %+v", list.Languages)
	}
	if code := anon.do("GET", "/api/languages/xx", nil, nil); code != 404 {
		t.Fatalf("missing language: %d", code)
	}
}
