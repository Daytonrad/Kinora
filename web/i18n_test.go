package web

import (
	"regexp"
	"strings"
	"testing"
)

// topKeys reads the top-level keys of a language file's "strings" object (two-space indent: nested objects such as
// "kinds" or plural forms are deeper).
var reTopKey = regexp.MustCompile(`(?m)^  "([^"]+)":`)

func langKeys(t *testing.T, code string) map[string]bool {
	t.Helper()
	b, err := Static.ReadFile("static/i18n/" + code + ".js")
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{}
	for _, m := range reTopKey.FindAllStringSubmatch(string(b), -1) {
		keys[m[1]] = true
	}
	if len(keys) < 100 {
		t.Fatalf("%s: only %d keys read", code, len(keys))
	}
	return keys
}

// Every text the web client asks for exists in Hungarian (the base language), and English and German have every
// Hungarian key, so no screen falls back to another language or shows "undefined".
func TestI18nKeys(t *testing.T) {
	hu := langKeys(t, "hu")
	for _, code := range []string{"en", "de"} {
		other := langKeys(t, code)
		var missing []string
		for k := range hu {
			if !other[k] {
				missing = append(missing, k)
			}
		}
		if len(missing) > 0 {
			t.Errorf("%s lacks %d keys: %s", code, len(missing), strings.Join(missing, ", "))
		}
	}

	src, err := Static.ReadFile("static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	// optional tables a language may add; app.js reads them with a fallback (T.srv || {}, lbl(T.sortLabels, …))
	assigned := map[string]bool{"srv": true, "srvRe": true, "sortLabels": true, "kindOpts": true, "ruleFields": true}
	// T.x = … in app.js itself (derived tables)
	for _, m := range regexp.MustCompile(`\bT\.([A-Za-z_]\w*)\s*=[^=]`).FindAllStringSubmatch(string(src), -1) {
		assigned[m[1]] = true
	}
	var unknown []string
	seen := map[string]bool{}
	for _, m := range regexp.MustCompile(`\bT\.([A-Za-z_]\w*)`).FindAllStringSubmatch(string(src), -1) {
		k := m[1]
		if !hu[k] && !assigned[k] && !seen[k] {
			seen[k] = true
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		t.Errorf("app.js uses texts that no language has: %s", strings.Join(unknown, ", "))
	}

	// the first-run wizard builds some keys from its step names
	for _, step := range []string{"media", "metadata", "hardware", "downloads", "remote", "summary"} {
		for _, k := range []string{"wizStep_" + step, "wizTitle_" + step, "wizLede_" + step} {
			if !hu[k] {
				t.Errorf("wizard text %s is missing", k)
			}
		}
	}
	if !strings.Contains(string(src), `const WIZ = ['media', 'metadata', 'hardware', 'downloads', 'remote', 'summary'];`) {
		t.Error("the wizard's steps changed: update this test's list")
	}
}
