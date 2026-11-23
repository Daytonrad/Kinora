package cardigann

import (
	"strings"
	"testing"
)

func FuzzParseDefinition(f *testing.F) {
	f.Add([]byte(minimalDef))
	f.Add([]byte("id: x\nname: X\nlinks: [https://example.org/]\nsearch:\n  paths:\n    - path: \"{{ .Keywords }}\"\n"))
	f.Add([]byte("id: [\nname"))
	f.Add([]byte("a: &a [*a, *a, *a]\nb: *a\n"))
	f.Add([]byte(""))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 64<<10 {
			return // definitions are limited to a few hundred kB by the catalog; keep runs fast
		}
		d, err := Parse(b)
		if err == nil && (d.ID == "" || len(d.Links) == 0) {
			t.Fatalf("Parse accepted a definition without id or links: %+v", d)
		}
	})
}

const minimalDef = `id: demo
name: Demo
links:
  - https://demo.example/
caps:
  categorymappings:
    - {id: 1, cat: Movies}
search:
  paths:
    - path: "search?q={{ .Keywords }}{{ if .Query.IMDBID }}&imdb={{ .Query.IMDBID }}{{ end }}"
  rows:
    selector: tr
  fields:
    title:
      selector: a
`

func FuzzRender(f *testing.F) {
	for _, s := range []string{
		"{{ .Keywords }}",
		"{{ if .Query.IMDBID }}{{ .Query.IMDBID }}{{ else }}{{ .Keywords }}{{ end }}",
		"{{ .Config.sort }}-{{ .Result.title }}",
		`{{ re_replace .Keywords "\s+" "+" }}`,
		"{{ join .Categories \",\" }}",
		"{{ .Today.Year }}",
		"{{ .Config.some-key }}",
		"{{ range .Categories }}{{ . }}{{ end }}",
		"{{",
		"{{ template \"x\" }}",
		"{{ call .Keywords }}",
		"plain text",
	} {
		f.Add(s, "the matrix 1999")
	}
	f.Fuzz(func(t *testing.T, tmpl, kw string) {
		if len(tmpl) > 4096 {
			return
		}
		ctx := newCtx(map[string]any{"sort": "seeders", "some-key": "v"}, Query{Type: "search", Keywords: kw, Q: kw, IMDBID: "tt0133093"}, []string{"1", "2"})
		ctx.Result["title"] = kw
		render(tmpl, ctx) // errors are fine; panics and hangs are not
	})
}

func FuzzFilters(f *testing.F) {
	names := []string{"querystring", "prepend", "append", "replace", "re_replace", "regexp", "split", "trim", "tolower",
		"toupper", "urlencode", "urldecode", "htmldecode", "htmlencode", "diacritics", "dateparse", "timeparse", "timeago",
		"reltime", "fuzzytime", "validfilename", "jsonjoinarray", "validate", "strdump", "andmatch", "unknown"}
	for _, n := range names {
		f.Add(n, "", "", "The Matrix (1999) 1.5 GB 2 days ago")
	}
	f.Add("re_replace", `(\w+)`, "$1-$2", "a b c")
	f.Add("regexp", `(?<=a)b+`, "", "abbb")
	f.Add("regexp", `(a+)+$`, "", strings.Repeat("a", 30)+"!")
	f.Add("split", "/", "5", "a/b")
	f.Add("split", "/", "-1", "a/b")
	f.Add("dateparse", "dd.MM.yyyy HH:mm", "", "31.12.2023 23:59")
	f.Add("querystring", "q", "", "http://x/?q=1&q=2")
	f.Add("jsonjoinarray", "$.a", ",", `{"a":[1,"x",null]}`)
	f.Add("validate", "1080p,720p", "", "1080p 720p")
	f.Fuzz(func(t *testing.T, name, a1, a2, v string) {
		if len(v) > 4096 || len(a1) > 1024 || len(a2) > 1024 {
			return
		}
		ctx := newCtx(map[string]any{}, Query{Keywords: "k"}, nil)
		for _, args := range []any{nil, a1, []any{a1, a2}, []any{a1}} {
			applyFilter(v, Filter{Name: name, Args: args}, ctx)
		}
	})
}

func FuzzParseSize(f *testing.F) {
	for _, s := range []string{"1.5 GB", "700 MiB", "1,234.5 MB", "1.234,5 MB", "12345", "-5 GB", "1e400 TB", "NaN GB", "", "GB", "9999999999999 PB"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if n := ParseSize(s); n < 0 {
			t.Fatalf("ParseSize(%q) = %d", s, n)
		}
	})
}
