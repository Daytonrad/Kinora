package cardigann

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"text/template"
	"time"
)

// Query is the search as the templates see it (.Query.*).
type Query struct {
	Type        string // search | movie | tv
	Q           string
	Keywords    string
	Season      string
	Ep          string
	IMDBID      string // tt0133093
	IMDBIDShort string // 0133093
	TMDBID      string
	TVDBID      string
	Year        string
}

// templateCtx is the data a template gets.
type templateCtx struct {
	Config     map[string]any
	Keywords   string
	Query      Query
	Categories []string
	Result     map[string]string
	Today      struct{ Year int }
	True       bool
	False      bool
}

var funcs = template.FuncMap{
	"re_replace": func(s, pattern, repl string) (string, error) {
		re, err := compileRegexp(pattern)
		if err != nil {
			return "", err
		}
		return re.ReplaceAllString(s, dotnetReplacement(repl)), nil
	},
	"join": func(list any, sep string) string {
		switch v := list.(type) {
		case []string:
			return strings.Join(v, sep)
		case []any:
			s := make([]string, len(v))
			for i, x := range v {
				s[i] = fmt.Sprint(x)
			}
			return strings.Join(s, sep)
		}
		return fmt.Sprint(list)
	},
	"replace": strings.ReplaceAll,
	"lower":   strings.ToLower,
	"upper":   strings.ToUpper,
}

var (
	tmplMu    sync.Mutex
	tmplCache = map[string]*template.Template{}
	reCache   = map[string]*regexp.Regexp{}
)

// render applies a Go template ({{ .Keywords }}, {{ if .Query.IMDBID }}…{{ end }}) to s.
func render(s string, ctx *templateCtx) (string, error) {
	if !strings.Contains(s, "{{") {
		return s, nil
	}
	tmplMu.Lock()
	t, ok := tmplCache[s]
	if !ok {
		var err error
		t, err = template.New("").Funcs(funcs).Option("missingkey=zero").Parse(s)
		if err != nil {
			tmplMu.Unlock()
			return "", fmt.Errorf("sablon: %w", err)
		}
		tmplCache[s] = t
	}
	tmplMu.Unlock()
	var b bytes.Buffer
	if err := t.Execute(&b, ctx); err != nil {
		return "", fmt.Errorf("sablon: %w", err)
	}
	return strings.ReplaceAll(b.String(), "<no value>", ""), nil
}

// compileRegexp compiles a definition's (.NET flavoured) pattern with Go's regexp; constructs Go lacks
// (lookarounds, backreferences) fail with an error naming the pattern.
func compileRegexp(p string) (*regexp.Regexp, error) {
	tmplMu.Lock()
	defer tmplMu.Unlock()
	if re, ok := reCache[p]; ok {
		return re, nil
	}
	re, err := regexp.Compile(p)
	if err != nil {
		return nil, fmt.Errorf("a %q reguláris kifejezés nem támogatott: %w", p, err)
	}
	reCache[p] = re
	return re, nil
}

var reDotnetGroup = regexp.MustCompile(`\$(\d+)`)

// dotnetReplacement turns .NET's "$1" into Go's "${1}" so "$1x" keeps working.
func dotnetReplacement(r string) string { return reDotnetGroup.ReplaceAllString(r, "$${$1}") }

func newCtx(config map[string]any, q Query, cats []string) *templateCtx {
	c := &templateCtx{Config: config, Keywords: q.Keywords, Query: q, Categories: cats, Result: map[string]string{}, True: true}
	c.Today.Year = time.Now().Year()
	return c
}

func regexpMust(p string) *regexp.Regexp { return regexp.MustCompile(p) }
