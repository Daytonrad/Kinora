// Package cardigann runs indexer definitions in the Cardigann YAML format (the format Jackett and Prowlarr
// use) so Kinora can search sites that have no Torznab API without an indexer proxy. Kinora ships no
// definitions: the admin uploads the ones they want to use.
//
// Supported: the definition header, caps (category mappings, search modes), settings (text, password,
// checkbox, select, info), login (post, form, cookie, get; error selectors, test), search (paths, inputs,
// $raw, keywordsfilters, headers, HTML and JSON responses, rows, fields with selectors, attributes, text
// templates, case, remove, optional, default and filters), and download selectors. Not supported: captchas,
// row merging ("after"), and .NET-only regular expression features.
package cardigann

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Definition is one indexer definition file.
type Definition struct {
	ID          string    `yaml:"id" json:"id"`
	Name        string    `yaml:"name" json:"name"`
	Description string    `yaml:"description" json:"description"`
	Language    string    `yaml:"language" json:"language"`
	Type        string    `yaml:"type" json:"type"` // public | semi-private | private
	Encoding    string    `yaml:"encoding" json:"-"`
	Links       []string  `yaml:"links" json:"links"`
	Caps        Caps      `yaml:"caps" json:"-"`
	Settings    []Setting `yaml:"settings" json:"settings"`
	Login       *Login    `yaml:"login" json:"-"`
	Search      Search    `yaml:"search" json:"-"`
	Download    *Download `yaml:"download" json:"-"`
}

// Caps lists the site's categories and search modes.
type Caps struct {
	CategoryMappings []CategoryMapping   `yaml:"categorymappings"`
	Categories       map[string]string   `yaml:"categories"` // the older form: site id → category name
	Modes            map[string][]string `yaml:"modes"`
}

// CategoryMapping maps a site category to a Torznab category name ("Movies/HD", "TV/UHD").
type CategoryMapping struct {
	ID      string `yaml:"id"`
	Cat     string `yaml:"cat"`
	Desc    string `yaml:"desc"`
	Default bool   `yaml:"default"`
}

// Setting is one value the admin fills in for the indexer.
type Setting struct {
	Name    string            `yaml:"name" json:"name"`
	Type    string            `yaml:"type" json:"type"` // text | password | checkbox | select | info
	Label   string            `yaml:"label" json:"label"`
	Default any               `yaml:"default" json:"default,omitempty"`
	Options map[string]string `yaml:"options" json:"options,omitempty"`
}

// Login says how to sign in.
type Login struct {
	Path           string              `yaml:"path"`
	SubmitPath     string              `yaml:"submitpath"`
	Method         string              `yaml:"method"` // post | form | cookie | get
	Form           string              `yaml:"form"`
	Inputs         map[string]string   `yaml:"inputs"`
	SelectorInputs map[string]Selector `yaml:"selectorinputs"`
	Error          []ErrorSel          `yaml:"error"`
	Test           *LoginTest          `yaml:"test"`
	Headers        map[string]any      `yaml:"headers"`
}

// ErrorSel is a selector whose presence means the sign in failed.
type ErrorSel struct {
	Selector string    `yaml:"selector"`
	Message  *Selector `yaml:"message"`
}

// LoginTest is a page that shows a selector only when signed in.
type LoginTest struct {
	Path     string `yaml:"path"`
	Selector string `yaml:"selector"`
}

// Search says how to search and read the results.
type Search struct {
	Path            string            `yaml:"path"`
	Paths           []SearchPath      `yaml:"paths"`
	Inputs          map[string]string `yaml:"inputs"`
	KeywordsFilters []Filter          `yaml:"keywordsfilters"`
	Headers         map[string]any    `yaml:"headers"`
	Rows            Rows              `yaml:"rows"`
	Fields          yaml.Node         `yaml:"fields"` // ordered: later fields may use earlier ones (.Result.x)
	Response        *Response         `yaml:"response"`
	ErrorSels       []ErrorSel        `yaml:"error"`
}

// SearchPath is one request of a search.
type SearchPath struct {
	Path          string            `yaml:"path"`
	Method        string            `yaml:"method"`
	Inputs        map[string]string `yaml:"inputs"`
	InheritInputs *bool             `yaml:"inheritinputs"`
	Categories    []string          `yaml:"categories"`
	Response      *Response         `yaml:"response"`
}

// Response says how to read a search answer.
type Response struct {
	Type             string `yaml:"type"` // html (default) | json | xml
	NoResultsMessage string `yaml:"noResultsMessage"`
}

// Rows selects the result rows.
type Rows struct {
	Selector                        string    `yaml:"selector"`
	Remove                          string    `yaml:"remove"`
	After                           int       `yaml:"after"`
	Filters                         []Filter  `yaml:"filters"`
	Count                           *Selector `yaml:"count"`
	Attribute                       string    `yaml:"attribute"`
	MissingAttributeEqualsNoResults bool      `yaml:"missingAttributeEqualsNoResults"`
}

// Download says how to find the torrent or magnet link on a details page.
type Download struct {
	Selectors []Selector     `yaml:"selectors"`
	Method    string         `yaml:"method"`
	Headers   map[string]any `yaml:"headers"`
	Before    *SearchPath    `yaml:"before"`
}

// Selector reads one value from a row (or a page).
type Selector struct {
	Selector          string            `yaml:"selector"`
	Attribute         string            `yaml:"attribute"`
	Text              *string           `yaml:"text"`
	Remove            string            `yaml:"remove"`
	Filters           []Filter          `yaml:"filters"`
	Case              map[string]string `yaml:"case"`
	Optional          bool              `yaml:"optional"`
	Default           *string           `yaml:"default"`
	UseBeforeResponse bool              `yaml:"usebeforeresponse"`
}

// Filter transforms a value: {name: dateparse, args: "yyyy-MM-dd"}.
type Filter struct {
	Name string `yaml:"name"`
	Args any    `yaml:"args"`
}

// Field is a named row selector, in the order of the definition.
type Field struct {
	Name string
	Selector
}

// FieldList returns the fields in their order.
func (d *Definition) FieldList() ([]Field, error) {
	n := d.Search.Fields
	if n.Kind == 0 {
		return nil, nil
	}
	if n.Kind != yaml.MappingNode {
		return nil, errors.New("a search.fields nem leképezés")
	}
	var out []Field
	for i := 0; i+1 < len(n.Content); i += 2 {
		var sel Selector
		v := n.Content[i+1]
		if v.Kind == yaml.ScalarNode { // "title: a.name" shorthand
			sel.Selector = v.Value
		} else if err := v.Decode(&sel); err != nil {
			return nil, fmt.Errorf("mező %s: %w", n.Content[i].Value, err)
		}
		out = append(out, Field{Name: n.Content[i].Value, Selector: sel})
	}
	return out, nil
}

var reID = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// Parse reads and checks a definition.
func Parse(b []byte) (*Definition, error) {
	var d Definition
	if err := yaml.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("a leíró nem olvasható: %w", err)
	}
	d.ID = strings.ToLower(strings.TrimSpace(d.ID))
	switch {
	case !reID.MatchString(d.ID):
		return nil, errors.New("a leíróban nincs érvényes id")
	case strings.TrimSpace(d.Name) == "":
		return nil, errors.New("a leíróban nincs név")
	case len(d.Links) == 0:
		return nil, errors.New("a leíróban nincs link (links)")
	case len(d.Search.Paths) == 0 && d.Search.Path == "":
		return nil, errors.New("a leíróban nincs keresési útvonal (search.paths)")
	}
	if d.Search.Path != "" && len(d.Search.Paths) == 0 {
		d.Search.Paths = []SearchPath{{Path: d.Search.Path}}
	}
	if _, err := d.FieldList(); err != nil {
		return nil, err
	}
	if len(d.Caps.CategoryMappings) == 0 { // the older "categories" form
		for id, cat := range d.Caps.Categories {
			d.Caps.CategoryMappings = append(d.Caps.CategoryMappings, CategoryMapping{ID: id, Cat: cat})
		}
	}
	return &d, nil
}

// Secret reports whether a setting holds a secret (masked in the admin UI).
func (s Setting) Secret() bool {
	n := strings.ToLower(s.Name)
	return s.Type == "password" || n == "cookie" || n == "apikey" || n == "passkey" || strings.Contains(n, "token")
}

// SiteLink is the first link, with a trailing slash.
func (d *Definition) SiteLink() string {
	l := strings.TrimSpace(d.Links[0])
	if !strings.HasSuffix(l, "/") {
		l += "/"
	}
	return l
}
