package cardigann

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// Row is one release a definition found.
type Row struct {
	Title       string
	Details     string
	Download    string // a .torrent link or a details page (see Download)
	Magnet      string
	InfoHash    string
	IMDb        string
	Size        int64
	Seeders     int
	Leechers    int
	Grabs       int
	Date        time.Time
	Categories  []string // Torznab category names ("Movies/HD")
	Freeleech   bool
	Description string
}

// Indexer is a definition with its settings and its own signed-in session.
type Indexer struct {
	Def    *Definition
	Config map[string]any
	hc     *http.Client
	mu     sync.Mutex
	in     bool // signed in
}

const userAgent = "Mozilla/5.0 (compatible; Kinora)"
const maxPage = 8 << 20

// New makes an indexer from a definition and the admin's values (missing ones get the definition's defaults).
func New(def *Definition, values map[string]any, base *http.Client) *Indexer {
	cfg := map[string]any{"sitelink": def.SiteLink()}
	for _, s := range def.Settings {
		if s.Default != nil {
			cfg[s.Name] = s.Default
		} else if s.Type == "checkbox" {
			cfg[s.Name] = false
		} else {
			cfg[s.Name] = ""
		}
	}
	for k, v := range values {
		cfg[k] = v
	}
	jar, _ := cookiejar.New(nil)
	c := *base
	c.Jar = jar
	return &Indexer{Def: def, Config: cfg, hc: &c}
}

func (ix *Indexer) resolve(base, ref string) (string, error) {
	b, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	r, err := url.Parse(strings.TrimSpace(ref))
	if err != nil {
		return "", err
	}
	return b.ResolveReference(r).String(), nil
}

func (ix *Indexer) headers(req *http.Request, h map[string]any, ctx *templateCtx) {
	req.Header.Set("User-Agent", userAgent)
	for k, v := range h {
		for _, s := range argList(v) {
			if r, err := render(s, ctx); err == nil {
				req.Header.Add(k, r)
			}
		}
	}
}

// fetch sends a request and reads the answer (bounded).
func (ix *Indexer) fetch(ctx context.Context, method, u string, form url.Values, raw string, hdr map[string]any, tctx *templateCtx) ([]byte, *http.Response, error) {
	var body io.Reader
	if method == "POST" {
		enc := form.Encode()
		if raw != "" {
			if enc != "" {
				enc += "&"
			}
			enc += raw
		}
		body = strings.NewReader(enc)
	} else if len(form) > 0 || raw != "" {
		sep := "?"
		if strings.Contains(u, "?") {
			sep = "&"
		}
		q := form.Encode()
		if raw != "" {
			if q != "" {
				q += "&"
			}
			q += raw
		}
		u += sep + q
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, nil, err
	}
	ix.headers(req, hdr, tctx)
	if method == "POST" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := ix.hc.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, nil, fmt.Errorf("%s nem érhető el: %w", ix.Def.Name, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxPage))
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode >= 400 {
		return b, resp, fmt.Errorf("%s: HTTP %d", ix.Def.Name, resp.StatusCode)
	}
	return b, resp, nil
}

func doc(b []byte) (*goquery.Document, error) {
	return goquery.NewDocumentFromReader(bytes.NewReader(b))
}

func renderMap(m map[string]string, ctx *templateCtx) (url.Values, string, error) {
	v := url.Values{}
	raw := ""
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		r, err := render(m[k], ctx)
		if err != nil {
			return nil, "", err
		}
		if k == "$raw" {
			raw = r
			continue
		}
		v.Set(k, r)
	}
	return v, raw, nil
}

// ------------------------------------------------------------------ login

// Login signs in as the definition says (no-op for public sites).
func (ix *Indexer) Login(ctx context.Context) error {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	return ix.login(ctx)
}

func (ix *Indexer) login(ctx context.Context) error {
	l := ix.Def.Login
	if l == nil || (l.Method == "" && l.Path == "" && len(l.Inputs) == 0) {
		ix.in = true
		return nil
	}
	tctx := newCtx(ix.Config, Query{}, nil)
	site := ix.Def.SiteLink()
	path := l.Path
	if path == "" {
		path = "/"
	}
	loginURL, err := ix.resolve(site, path)
	if err != nil {
		return err
	}
	var page []byte
	switch strings.ToLower(l.Method) {
	case "cookie":
		v, _, err := renderMap(l.Inputs, tctx)
		if err != nil {
			return err
		}
		cookie := v.Get("cookie")
		if cookie == "" {
			cookie = fmt.Sprint(ix.Config["cookie"])
		}
		if strings.TrimSpace(cookie) == "" {
			return errors.New("add meg a sütit (cookie) a beállításokban")
		}
		u, _ := url.Parse(site)
		var cs []*http.Cookie
		for _, p := range strings.Split(cookie, ";") {
			if k, val, ok := strings.Cut(strings.TrimSpace(p), "="); ok {
				cs = append(cs, &http.Cookie{Name: k, Value: val, Path: "/"})
			}
		}
		ix.hc.Jar.SetCookies(u, cs)
	case "form":
		b, resp, err := ix.fetch(ctx, "GET", loginURL, nil, "", l.Headers, tctx)
		if err != nil {
			return err
		}
		d, err := doc(b)
		if err != nil {
			return err
		}
		sel := l.Form
		if sel == "" {
			sel = "form"
		}
		form := d.Find(sel).First()
		if form.Length() == 0 {
			return fmt.Errorf("%s: nem találom a belépő űrlapot (%s)", ix.Def.Name, sel)
		}
		vals := url.Values{}
		form.Find("input[name]").Each(func(_ int, s *goquery.Selection) {
			t, _ := s.Attr("type")
			if t == "checkbox" || t == "radio" {
				if _, on := s.Attr("checked"); !on {
					return
				}
			}
			vals.Set(s.AttrOr("name", ""), s.AttrOr("value", ""))
		})
		for name, sl := range l.SelectorInputs {
			if v, err := ix.value(sl, d.Selection, tctx); err == nil {
				vals.Set(name, v)
			}
		}
		in, _, err := renderMap(l.Inputs, tctx)
		if err != nil {
			return err
		}
		for k := range in {
			vals.Set(k, in.Get(k))
		}
		action := form.AttrOr("action", "")
		if l.SubmitPath != "" {
			action = l.SubmitPath
		}
		target, err := ix.resolve(resp.Request.URL.String(), action)
		if err != nil {
			return err
		}
		if page, _, err = ix.fetch(ctx, "POST", target, vals, "", l.Headers, tctx); err != nil {
			return err
		}
	case "get":
		in, raw, err := renderMap(l.Inputs, tctx)
		if err != nil {
			return err
		}
		if page, _, err = ix.fetch(ctx, "GET", loginURL, in, raw, l.Headers, tctx); err != nil {
			return err
		}
	default: // post
		in, raw, err := renderMap(l.Inputs, tctx)
		if err != nil {
			return err
		}
		target := loginURL
		if l.SubmitPath != "" {
			if target, err = ix.resolve(site, l.SubmitPath); err != nil {
				return err
			}
		}
		if page, _, err = ix.fetch(ctx, "POST", target, in, raw, l.Headers, tctx); err != nil {
			return err
		}
	}
	if page != nil && len(l.Error) > 0 {
		if d, err := doc(page); err == nil {
			for _, e := range l.Error {
				if s := d.Find(e.Selector); s.Length() > 0 {
					msg := strings.TrimSpace(s.First().Text())
					if e.Message != nil {
						if m, err := ix.value(*e.Message, d.Selection, tctx); err == nil && m != "" {
							msg = m
						}
					}
					if msg == "" {
						msg = "hibás belépési adatok"
					}
					return fmt.Errorf("%s: %s", ix.Def.Name, msg)
				}
			}
		}
	}
	if l.Test != nil && l.Test.Selector != "" {
		tu, err := ix.resolve(site, l.Test.Path)
		if err != nil {
			return err
		}
		b, _, err := ix.fetch(ctx, "GET", tu, nil, "", nil, tctx)
		if err != nil {
			return err
		}
		d, err := doc(b)
		if err != nil || d.Find(l.Test.Selector).Length() == 0 {
			return fmt.Errorf("%s: a belépés nem sikerült (ellenőrizd a felhasználónevet, jelszót vagy sütit)", ix.Def.Name)
		}
	}
	ix.in = true
	return nil
}

// ------------------------------------------------------------------ values

// value reads one selector from a selection (a row, or a whole page).
func (ix *Indexer) value(sel Selector, s *goquery.Selection, ctx *templateCtx) (string, error) {
	var v string
	switch {
	case sel.Text != nil:
		r, err := render(*sel.Text, ctx)
		if err != nil {
			return "", err
		}
		v = r
	case sel.Selector != "" || len(sel.Case) > 0:
		t := s
		if sel.Selector != "" && sel.Selector != ":root" {
			t = s.Find(sel.Selector)
		}
		if t.Length() == 0 {
			if sel.Default != nil {
				return render(*sel.Default, ctx)
			}
			if sel.Optional {
				return "", nil
			}
			return "", fmt.Errorf("nincs találat: %s", sel.Selector)
		}
		t = t.First()
		if sel.Remove != "" {
			t = t.Clone()
			t.Find(sel.Remove).Remove()
		}
		if len(sel.Case) > 0 {
			keys := make([]string, 0, len(sel.Case))
			for k := range sel.Case {
				keys = append(keys, k)
			}
			sort.Slice(keys, func(i, j int) bool { return keys[i] != "*" && (keys[j] == "*" || keys[i] < keys[j]) })
			found := false
			for _, k := range keys {
				if k == "*" || t.Is(k) || t.Find(k).Length() > 0 {
					v, found = sel.Case[k], true
					break
				}
			}
			if !found {
				if sel.Optional {
					return "", nil
				}
				return "", errors.New("egyik case sem illik")
			}
		} else if sel.Attribute != "" {
			a, ok := t.Attr(sel.Attribute)
			if !ok && !sel.Optional {
				return "", fmt.Errorf("nincs %s attribútum", sel.Attribute)
			}
			v = a
		} else {
			v = strings.Join(strings.Fields(t.Text()), " ")
		}
	default:
		if sel.Default != nil {
			return render(*sel.Default, ctx)
		}
	}
	return applyFilters(v, sel.Filters, ctx)
}

// jsonValue reads a field from a JSON row: a dotted path ("info.size"), a text template, case or default.
func (ix *Indexer) jsonValue(sel Selector, row any, ctx *templateCtx) (string, error) {
	var v string
	switch {
	case sel.Text != nil:
		r, err := render(*sel.Text, ctx)
		if err != nil {
			return "", err
		}
		v = r
	case sel.Selector != "":
		x := jsonPath(row, sel.Selector)
		if x == nil {
			if sel.Default != nil {
				return render(*sel.Default, ctx)
			}
			if sel.Optional {
				return "", nil
			}
			return "", fmt.Errorf("nincs ilyen JSON-mező: %s", sel.Selector)
		}
		v = jsonString(x)
		if len(sel.Case) > 0 {
			if c, ok := sel.Case[v]; ok {
				v = c
			} else if c, ok := sel.Case["*"]; ok {
				v = c
			}
		}
	default:
		if sel.Default != nil {
			return render(*sel.Default, ctx)
		}
	}
	return applyFilters(v, sel.Filters, ctx)
}

func jsonString(x any) string {
	switch v := x.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		if v {
			return "True"
		}
		return "False"
	case nil:
		return ""
	}
	b, _ := json.Marshal(x)
	return string(b)
}

// ------------------------------------------------------------------ search

// categoriesFor returns the site's category ids for a kind of search (movie, tv; others: all).
func (ix *Indexer) categoriesFor(kind string) []string {
	var out []string
	for _, c := range ix.Def.Caps.CategoryMappings {
		cat := strings.ToLower(c.Cat)
		if (kind == "movie" && strings.HasPrefix(cat, "movies")) || (kind == "tv" && strings.HasPrefix(cat, "tv")) {
			out = append(out, c.ID)
		}
	}
	return out
}

func (ix *Indexer) catNames(id string) []string {
	var out []string
	for _, c := range ix.Def.Caps.CategoryMappings {
		if c.ID == id {
			out = append(out, c.Cat)
		}
	}
	return out
}

// Search runs every search path of the definition for a query.
func (ix *Indexer) Search(ctx context.Context, q Query) ([]Row, error) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	if !ix.in {
		if err := ix.login(ctx); err != nil {
			return nil, err
		}
	}
	rows, err := ix.search(ctx, q)
	if errors.Is(err, errSignedOut) && ix.Def.Login != nil { // the session ran out: once more after signing in
		ix.in = false
		if err := ix.login(ctx); err != nil {
			return nil, err
		}
		rows, err = ix.search(ctx, q)
	}
	return rows, err
}

var errSignedOut = errors.New("a munkamenet lejárt")

func (ix *Indexer) search(ctx context.Context, q Query) ([]Row, error) {
	cats := ix.categoriesFor(q.Type)
	tctx := newCtx(ix.Config, q, cats)
	kw, err := applyFilters(q.Keywords, ix.Def.Search.KeywordsFilters, tctx)
	if err != nil {
		return nil, err
	}
	tctx.Keywords, tctx.Query.Keywords = kw, kw
	fields, _ := ix.Def.FieldList()
	var out []Row
	seen := map[string]bool{}
	for _, p := range ix.Def.Search.Paths {
		if len(p.Categories) > 0 && len(cats) > 0 && !overlap(p.Categories, cats) {
			continue
		}
		path, err := render(p.Path, tctx)
		if err != nil {
			return nil, err
		}
		u, err := ix.resolve(ix.Def.SiteLink(), path)
		if err != nil {
			return nil, err
		}
		inputs := map[string]string{}
		if p.InheritInputs == nil || *p.InheritInputs {
			for k, v := range ix.Def.Search.Inputs {
				inputs[k] = v
			}
		}
		for k, v := range p.Inputs {
			inputs[k] = v
		}
		form, raw, err := renderMap(inputs, tctx)
		if err != nil {
			return nil, err
		}
		method := strings.ToUpper(p.Method)
		if method != "POST" {
			method = "GET"
		}
		body, resp, err := ix.fetch(ctx, method, u, form, raw, ix.Def.Search.Headers, tctx)
		if err != nil {
			return nil, err
		}
		if l := ix.Def.Login; l != nil && l.Path != "" && resp.Request.URL.Path != "" &&
			strings.TrimLeft(resp.Request.URL.Path, "/") == strings.TrimLeft(strings.SplitN(l.Path, "?", 2)[0], "/") {
			return nil, errSignedOut // redirected to the sign-in page
		}
		respType := "html"
		if r := p.Response; r != nil && r.Type != "" {
			respType = r.Type
		} else if r := ix.Def.Search.Response; r != nil && r.Type != "" {
			respType = r.Type
		}
		var rows []Row
		if respType == "json" {
			rows, err = ix.jsonRows(body, fields, tctx, resp.Request.URL.String())
		} else {
			rows, err = ix.htmlRows(body, fields, tctx, resp.Request.URL.String())
		}
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			key := r.Download + "|" + r.Magnet + "|" + r.Title
			if !seen[key] {
				seen[key] = true
				out = append(out, r)
			}
		}
	}
	return out, nil
}

func overlap(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}

func (ix *Indexer) htmlRows(body []byte, fields []Field, tctx *templateCtx, pageURL string) ([]Row, error) {
	d, err := doc(body)
	if err != nil {
		return nil, err
	}
	for _, e := range ix.Def.Search.ErrorSels {
		if s := d.Find(e.Selector); s.Length() > 0 {
			return nil, fmt.Errorf("%s: %s", ix.Def.Name, strings.TrimSpace(s.First().Text()))
		}
	}
	if ix.Def.Search.Rows.Selector == "" {
		return nil, errors.New("a leíróban nincs search.rows.selector")
	}
	var out []Row
	d.Find(ix.Def.Search.Rows.Selector).Each(func(_ int, s *goquery.Selection) {
		if ix.Def.Search.Rows.Remove != "" {
			s = s.Clone()
			s.Find(ix.Def.Search.Rows.Remove).Remove()
		}
		rc := *tctx
		rc.Result = map[string]string{}
		ok := true
		for _, f := range fields {
			v, err := ix.value(f.Selector, s, &rc)
			if err != nil {
				if isKeyField(f.Name) {
					ok = false
					break
				}
				v = ""
			}
			rc.Result[f.Name] = v
		}
		if ok {
			if r, good := ix.toRow(rc.Result, pageURL); good {
				out = append(out, r)
			}
		}
	})
	return out, nil
}

func (ix *Indexer) jsonRows(body []byte, fields []Field, tctx *templateCtx, pageURL string) ([]Row, error) {
	var root any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("%s: érthetetlen JSON-válasz", ix.Def.Name)
	}
	list := jsonPath(root, ix.Def.Search.Rows.Selector)
	var items []any
	switch v := list.(type) {
	case []any:
		items = v
	case map[string]any:
		for _, x := range v {
			items = append(items, x)
		}
	case nil:
		return nil, nil
	}
	var out []Row
	for _, it := range items {
		rc := *tctx
		rc.Result = map[string]string{}
		ok := true
		for _, f := range fields {
			v, err := ix.jsonValue(f.Selector, it, &rc)
			if err != nil {
				if isKeyField(f.Name) {
					ok = false
					break
				}
				v = ""
			}
			rc.Result[f.Name] = v
		}
		if ok {
			if r, good := ix.toRow(rc.Result, pageURL); good {
				out = append(out, r)
			}
		}
	}
	return out, nil
}

func isKeyField(name string) bool {
	switch name {
	case "title", "download", "magnet":
		return true
	}
	return false
}

func (ix *Indexer) toRow(f map[string]string, pageURL string) (Row, bool) {
	r := Row{Title: strings.TrimSpace(f["title"])}
	abs := func(s string) string {
		s = strings.TrimSpace(s)
		if s == "" || strings.HasPrefix(s, "magnet:") {
			return s
		}
		if u, err := ix.resolve(pageURL, s); err == nil {
			return u
		}
		return s
	}
	r.Details = abs(firstOf(f["details"], f["comments"]))
	r.Download = abs(f["download"])
	r.Magnet = strings.TrimSpace(f["magnet"])
	if strings.HasPrefix(r.Download, "magnet:") {
		r.Magnet, r.Download = r.Download, ""
	}
	r.InfoHash = strings.ToLower(strings.TrimSpace(f["infohash"]))
	r.Size = parseSize(f["size"])
	r.Seeders = atoiLoose(f["seeders"])
	r.Leechers = atoiLoose(f["leechers"])
	r.Grabs = atoiLoose(f["grabs"])
	if d := firstOf(f["date"], f["publishdate"]); d != "" {
		if t, err := time.Parse(time.RFC3339, d); err == nil {
			r.Date = t
		} else if t, err := fuzzyTime(d); err == nil {
			r.Date = t
		}
	}
	if imdb := firstOf(f["imdbid"], f["imdb"]); imdb != "" {
		if m := reIMDb.FindString(imdb); m != "" {
			r.IMDb = m
		} else if n := strings.TrimLeft(strings.TrimSpace(imdb), "t"); n != "" && n != "0" {
			for len(n) < 7 {
				n = "0" + n
			}
			r.IMDb = "tt" + n
		}
	}
	if c := f["category"]; c != "" {
		r.Categories = ix.catNames(c)
	}
	r.Freeleech = f["downloadvolumefactor"] == "0"
	r.Description = f["description"]
	return r, r.Title != "" && (r.Download != "" || r.Magnet != "")
}

func firstOf(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func atoiLoose(s string) int {
	s = strings.NewReplacer(",", "", ".", "", " ", "").Replace(strings.TrimSpace(s))
	n, _ := strconv.Atoi(s)
	return n
}

var reIMDb = regexpMust(`tt\d{7,10}`)

// ------------------------------------------------------------------ download

// Download fetches a release with the signed-in session: a .torrent file, a magnet link, or – when the link is a
// details page – the one the definition's download selectors point at.
func (ix *Indexer) Download(ctx context.Context, link string) (torrent []byte, magnet string, err error) {
	if strings.HasPrefix(link, "magnet:") {
		return nil, link, nil
	}
	ix.mu.Lock()
	defer ix.mu.Unlock()
	if !ix.in {
		if err := ix.login(ctx); err != nil {
			return nil, "", err
		}
	}
	tctx := newCtx(ix.Config, Query{}, nil)
	var hdr map[string]any
	if ix.Def.Download != nil {
		hdr = ix.Def.Download.Headers
	}
	for step := 0; step < 3; step++ {
		b, resp, err := ix.fetch(ctx, "GET", link, nil, "", hdr, tctx)
		if err != nil {
			return nil, "", err
		}
		if len(b) > 0 && b[0] == 'd' && bytes.Contains(b[:min(len(b), 4096)], []byte("4:info")) {
			return b, "", nil
		}
		if ix.Def.Download == nil || len(ix.Def.Download.Selectors) == 0 || step > 0 {
			return nil, "", fmt.Errorf("%s: a letöltés nem torrent fájl (lejárt a belépés, vagy a leíróból hiányzik a download szakasz)", ix.Def.Name)
		}
		d, err := doc(b)
		if err != nil {
			return nil, "", err
		}
		next := ""
		for _, sel := range ix.Def.Download.Selectors {
			if v, err := ix.value(sel, d.Selection, tctx); err == nil && v != "" {
				next = v
				break
			}
		}
		if next == "" {
			return nil, "", fmt.Errorf("%s: az adatlapon nem találom a letöltő linket", ix.Def.Name)
		}
		if strings.HasPrefix(next, "magnet:") {
			return nil, next, nil
		}
		if link, err = ix.resolve(resp.Request.URL.String(), next); err != nil {
			return nil, "", err
		}
	}
	return nil, "", errors.New("túl sok lépés a letöltésig")
}

// Test signs in and runs an empty search: what the admin's Test button does.
func (ix *Indexer) Test(ctx context.Context) (int, error) {
	ix.mu.Lock()
	ix.in = false
	ix.mu.Unlock()
	rows, err := ix.Search(ctx, Query{Type: "search"})
	return len(rows), err
}
