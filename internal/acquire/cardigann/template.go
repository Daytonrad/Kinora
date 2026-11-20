package cardigann

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/dlclark/regexp2"
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

// fields is the query as templates see it.
func (q Query) fields() map[string]any {
	return map[string]any{"Type": q.Type, "Q": q.Q, "Keywords": q.Keywords, "Season": q.Season, "Ep": q.Ep, "IMDBID": q.IMDBID,
		"IMDBIDShort": q.IMDBIDShort, "TMDBID": q.TMDBID, "TVDBID": q.TVDBID, "Year": q.Year}
}

// templateCtx is the data a template gets.
type templateCtx struct {
	Config     map[string]any
	Keywords   string
	Query      map[string]any // the Query fields by name; ones a definition names that Kinora lacks are empty
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
		return re.replace(s, repl)
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
	reCache   = map[string]*dnRegexp{}
)

// reDashKey finds map keys Go templates cannot name directly (".Config.sort-by").
var reDashKey = regexp.MustCompile(`\.(Config|Result|Query)\.([A-Za-z0-9_]+(?:-[A-Za-z0-9_]+)+)`)

var (
	reAction  = regexp.MustCompile(`{{.*?}}`)
	reLiteral = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`)
)

// rawLiterals turns the quoted strings of template actions that Go cannot read into raw strings: definitions
// write regular expressions there ("\b0(\d)\b"), which Go would read as invalid escapes.
func rawLiterals(s string) string {
	return reAction.ReplaceAllStringFunc(s, func(a string) string {
		return reLiteral.ReplaceAllStringFunc(a, func(l string) string {
			in := l[1 : len(l)-1]
			if _, err := strconv.Unquote(l); err == nil || strings.Contains(in, "`") { // already a valid Go string
				return l
			}
			return "`" + strings.ReplaceAll(in, `\"`, `"`) + "`"
		})
	})
}

// render applies a Go template ({{ .Keywords }}, {{ if .Query.IMDBID }}…{{ end }}) to s.
func render(s string, ctx *templateCtx) (string, error) {
	if !strings.Contains(s, "{{") {
		return s, nil
	}
	tmplMu.Lock()
	t, ok := tmplCache[s]
	if !ok {
		var err error
		src := rawLiterals(reDashKey.ReplaceAllString(s, `(index .$1 "$2")`))
		t, err = template.New("").Funcs(funcs).Option("missingkey=zero").Parse(src)
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

// dnRegexp is a definition's pattern. Definitions are written for .NET (Jackett, Prowlarr), so they run on a
// .NET-compatible engine: lookarounds, backreferences, named Unicode blocks and "$1" replacements work as written.
type dnRegexp struct{ re *regexp2.Regexp }

// compileRegexp compiles a definition's pattern; a pattern can take at most a second on one value.
func compileRegexp(p string) (*dnRegexp, error) {
	tmplMu.Lock()
	defer tmplMu.Unlock()
	if re, ok := reCache[p]; ok {
		return re, nil
	}
	re, err := regexp2.Compile(dotnetBlocks(p), regexp2.None)
	if err != nil {
		return nil, fmt.Errorf("a %q reguláris kifejezés nem támogatott: %w", p, err)
	}
	re.MatchTimeout = time.Second
	r := &dnRegexp{re}
	reCache[p] = r
	return r, nil
}

var reDotnetBlock = regexp.MustCompile(`\\([pP])\{Is([A-Za-z]+)\}`)

// dotnetBlocks turns .NET's named Unicode blocks (\p{IsCyrillic}), which the engine lacks, into scripts (\p{Cyrillic}).
func dotnetBlocks(p string) string {
	return reDotnetBlock.ReplaceAllStringFunc(p, func(m string) string {
		sm := reDotnetBlock.FindStringSubmatch(m)
		name := sm[2]
		switch {
		case strings.HasPrefix(name, "CJK"):
			name = "Han"
		case strings.HasPrefix(name, "Latin"), strings.HasPrefix(name, "Basic"):
			name = "Latin"
		}
		return `\` + sm[1] + "{" + name + "}"
	})
}

// replace replaces every match; repl uses .NET's syntax ($1, ${name}, $0).
func (r *dnRegexp) replace(s, repl string) (string, error) {
	out, err := r.re.Replace(s, repl, -1, -1)
	if err != nil {
		return "", fmt.Errorf("reguláris kifejezés: %w", err)
	}
	return out, nil
}

// submatch returns the first match and its groups, or nil.
func (r *dnRegexp) submatch(s string) []string {
	m, err := r.re.FindStringMatch(s)
	if err != nil || m == nil {
		return nil
	}
	gs := m.Groups()
	out := make([]string, len(gs))
	for i, g := range gs {
		out[i] = g.String()
	}
	return out
}

func newCtx(config map[string]any, q Query, cats []string) *templateCtx {
	c := &templateCtx{Config: config, Keywords: q.Keywords, Query: q.fields(), Categories: cats, Result: map[string]string{}, True: true}
	c.Today.Year = time.Now().Year()
	return c
}

func regexpMust(p string) *regexp.Regexp { return regexp.MustCompile(p) }
