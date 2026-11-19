// Package sites holds indexers written in Go for sites that a Cardigann definition cannot describe
// (Jackett implements these in C#). Each one appears in the admin UI like a definition that cannot be deleted.
package sites

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/ScotyLucas/kinora/internal/acquire/cardigann"
)

// Engine is what the manager needs from an indexer engine; *cardigann.Indexer has the same methods.
type Engine interface {
	Search(ctx context.Context, q cardigann.Query) ([]cardigann.Row, error)
	Download(ctx context.Context, link string) (torrent []byte, magnet string, err error)
	Test(ctx context.Context) (int, error)
}

// Site is a built-in indexer: its description (header and settings, for the UI) and a constructor.
type Site struct {
	Def *cardigann.Definition
	New func(cfg map[string]any, hc *http.Client) Engine
}

// Builtin lists the built-in indexers by id.
var Builtin = map[string]Site{
	"ncore": {Def: ncoreDef, New: func(cfg map[string]any, hc *http.Client) Engine { return NewNCore(cfg, hc) }},
}

var ncoreDef = &cardigann.Definition{
	ID: "ncore", Name: "nCore", Type: "private", Language: "hu-HU", Links: []string{"https://ncore.pro/"},
	Description: "Magyar privát tracker (beépített, Go-ban írt indexer)",
	Settings: []cardigann.Setting{
		{Name: "username", Type: "text", Label: "Felhasználónév"},
		{Name: "password", Type: "password", Label: "Jelszó"},
		{Name: "cookie", Type: "password", Label: "Süti (csak kétlépcsős azonosításnál: a böngészőből kimásolt nick=…; pass=…; PHPSESSID=…)"},
		{Name: "info", Type: "info", Label: "Kétlépcsős azonosítás esetén a felhasználónév és jelszó helyett a sütit add meg. A Kinora csak a film- és sorozatkategóriákban keres."},
		{Name: "url", Type: "text", Label: "Cím", Default: "https://ncore.pro/"},
	},
}

// nCore categories: "_hun" = Hungarian audio.
var (
	ncoreMovieCats = []string{"hd_hun", "hd", "xvid_hun", "xvid", "dvd_hun", "dvd", "dvd9_hun", "dvd9"}
	ncoreTVCats    = []string{"hdser_hun", "hdser", "xvidser_hun", "xvidser", "dvdser_hun", "dvdser"}
	ncoreCatName   = map[string]string{"hd": "Movies/HD", "xvid": "Movies/SD", "dvd": "Movies/DVD", "dvd9": "Movies/DVD",
		"hdser": "TV/HD", "xvidser": "TV/SD", "dvdser": "TV/SD"}
)

// NCore searches ncore.pro with a signed-in session.
type NCore struct {
	base             *url.URL
	user, pass, ckie string
	hc               *http.Client
	mu               sync.Mutex
	in               bool
	key              string // the passkey of the download links
}

const ncoreMaxPage = 8 << 20

// NewNCore makes the indexer from its settings.
func NewNCore(cfg map[string]any, hc *http.Client) *NCore {
	str := func(k string) string { s, _ := cfg[k].(string); return strings.TrimSpace(s) }
	raw := str("url")
	if raw == "" {
		raw = "https://ncore.pro/"
	}
	if !strings.HasSuffix(raw, "/") {
		raw += "/"
	}
	base, err := url.Parse(raw)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") {
		base, _ = url.Parse("https://ncore.pro/")
	}
	jar, _ := cookiejar.New(nil)
	c := *hc
	c.Jar = jar
	return &NCore{base: base, user: str("username"), pass: str("password"), ckie: str("cookie"), hc: &c}
}

func (n *NCore) abs(p string) string {
	u, err := n.base.Parse(p)
	if err != nil {
		return p
	}
	return u.String()
}

func (n *NCore) do(ctx context.Context, method, u string, form url.Values) ([]byte, *http.Response, error) {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Kinora)")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := n.hc.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, nil, fmt.Errorf("az nCore nem érhető el: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, ncoreMaxPage))
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode >= 400 {
		return b, resp, fmt.Errorf("nCore: HTTP %d", resp.StatusCode)
	}
	return b, resp, nil
}

func onLoginPage(resp *http.Response) bool {
	return resp != nil && strings.HasSuffix(strings.ToLower(resp.Request.URL.Path), "login.php")
}

var reKey = regexp.MustCompile(`[?&;]key=([0-9a-fA-F]{16,64})`)

// login signs in with the username and password, or uses the pasted cookie (two-factor accounts).
func (n *NCore) login(ctx context.Context) error {
	n.in, n.key = false, ""
	switch {
	case n.ckie != "":
		var cs []*http.Cookie
		for _, p := range strings.Split(n.ckie, ";") {
			if k, v, ok := strings.Cut(strings.TrimSpace(p), "="); ok && k != "" {
				cs = append(cs, &http.Cookie{Name: k, Value: v, Path: "/"})
			}
		}
		n.hc.Jar.SetCookies(n.base, cs)
	case n.user == "" || n.pass == "":
		return errors.New("nCore: add meg a felhasználónevet és a jelszót (vagy kétlépcsős azonosításnál a sütit)")
	default:
		form := url.Values{"nev": {n.user}, "pass": {n.pass}, "set_lang": {"hu"}, "submitted": {"1"}, "ne_leptessen_ki": {"1"}}
		b, resp, err := n.do(ctx, "POST", n.abs("login.php"), form)
		if err != nil {
			return err
		}
		if onLoginPage(resp) || strings.Contains(string(b), `name="nev"`) {
			if strings.Contains(string(b), "2factor") {
				return errors.New("nCore: a fiókon kétlépcsős azonosítás van, add meg a sütit a jelszó helyett")
			}
			msg := "hibás felhasználónév vagy jelszó"
			if d, err := goquery.NewDocumentFromReader(strings.NewReader(string(b))); err == nil {
				if t := strings.TrimSpace(d.Find(".box_alert_red, .login_error, .hibauzenet").First().Text()); t != "" {
					msg = t
				}
			}
			return fmt.Errorf("nCore: %s", msg)
		}
	}
	// the passkey: in the RSS link and the download links of every signed-in page
	b, resp, err := n.do(ctx, "GET", n.abs("torrents.php"), nil)
	if err != nil {
		return err
	}
	if onLoginPage(resp) {
		if n.ckie != "" {
			return errors.New("nCore: a süti lejárt vagy hibás, másold ki újra a böngészőből")
		}
		return errors.New("nCore: a belépés nem sikerült")
	}
	if m := reKey.FindSubmatch(b); m != nil {
		n.key = string(m[1])
	}
	n.in = true
	return nil
}

// Search asks nCore; an IMDb id searches by IMDb (every release of the film or series), otherwise by name.
func (n *NCore) Search(ctx context.Context, q cardigann.Query) ([]cardigann.Row, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.in {
		if err := n.login(ctx); err != nil {
			return nil, err
		}
	}
	rows, err := n.search(ctx, q)
	if errors.Is(err, errSignedOut) {
		if err := n.login(ctx); err != nil {
			return nil, err
		}
		rows, err = n.search(ctx, q)
	}
	return rows, err
}

var errSignedOut = errors.New("signed out")

func (n *NCore) search(ctx context.Context, q cardigann.Query) ([]cardigann.Row, error) {
	cats := append(append([]string{}, ncoreMovieCats...), ncoreTVCats...)
	switch q.Type {
	case "movie":
		cats = ncoreMovieCats
	case "tv":
		cats = ncoreTVCats
	}
	v := url.Values{"tipus": {"kivalasztottak_kozott"}, "kivalasztott_tipus": {strings.Join(cats, ",")}, "miszerint": {"ctime"}, "hogyan": {"DESC"}}
	switch {
	case q.IMDBID != "":
		v.Set("miben", "imdb")
		v.Set("mire", q.IMDBID)
	case strings.TrimSpace(q.Keywords) != "":
		v.Set("miben", "name")
		v.Set("mire", strings.TrimSpace(q.Keywords))
	}
	b, resp, err := n.do(ctx, "GET", n.abs("torrents.php?"+v.Encode()), nil)
	if err != nil {
		return nil, err
	}
	if onLoginPage(resp) {
		return nil, errSignedOut
	}
	rows, err := n.parseHTML(b)
	if err != nil {
		return nil, err
	}
	if q.Type == "tv" && q.IMDBID != "" && q.Season != "" { // an IMDb search lists every season: keep the asked one
		s, _ := strconv.Atoi(q.Season)
		tag := fmt.Sprintf("S%02d", s)
		var keep []cardigann.Row
		for _, r := range rows {
			if strings.Contains(strings.ToUpper(r.Title), tag) {
				keep = append(keep, r)
			}
		}
		rows = keep
	}
	return rows, nil
}

var (
	reID     = regexp.MustCompile(`[?&]id=(\d+)`)
	reTipus  = regexp.MustCompile(`tipus=([a-z0-9_]+)`)
	reIMDb   = regexp.MustCompile(`tt\d{7,10}`)
	reSpaces = regexp.MustCompile(`\s+`)
)

// parseHTML reads the result list (div.box_torrent rows) of torrents.php.
func (n *NCore) parseHTML(b []byte) ([]cardigann.Row, error) {
	d, err := goquery.NewDocumentFromReader(strings.NewReader(string(b)))
	if err != nil {
		return nil, err
	}
	var out []cardigann.Row
	d.Find("div.box_torrent").Each(func(_ int, s *goquery.Selection) {
		a := s.Find("div.torrent_txt a[href*='action=details'], div.torrent_txt2 a[href*='action=details']").First()
		href, _ := a.Attr("href")
		m := reID.FindStringSubmatch(href)
		if m == nil {
			return
		}
		id := m[1]
		title := strings.TrimSpace(a.AttrOr("title", ""))
		if title == "" {
			title = strings.TrimSpace(a.Text())
		}
		if title == "" {
			return
		}
		r := cardigann.Row{Title: title, Details: n.abs("torrents.php?action=details&id=" + id)}
		r.Download = n.abs("torrents.php?action=download&id=" + id)
		if n.key != "" {
			r.Download += "&key=" + n.key
		}
		catHref, _ := s.Find("div.box_alap_img a, a[href*='tipus=']").First().Attr("href")
		if c := reTipus.FindStringSubmatch(catHref); c != nil {
			base := strings.TrimSuffix(c[1], "_hun")
			if name := ncoreCatName[base]; name != "" {
				r.Categories = []string{name}
			}
			if strings.HasSuffix(c[1], "_hun") {
				r.Language = "hun"
			}
		}
		r.Size = cardigann.ParseSize(s.Find("div.box_meret2").First().Text())
		r.Seeders = atoi(s.Find("div.box_s2").First().Text())
		r.Leechers = atoi(s.Find("div.box_l2").First().Text())
		r.Grabs = atoi(s.Find("div.box_d2").First().Text())
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", dateText(s.Find("div.box_feltoltve2").First()), time.Local); err == nil {
			r.Date = t
		}
		if h, ok := s.Find("a.infolink, a[href*='imdb.com']").First().Attr("href"); ok {
			r.IMDb = reIMDb.FindString(h)
		}
		r.Freeleech = s.Find("img[src*='freeleech'], .freeleech").Length() > 0
		out = append(out, r)
	})
	return out, nil
}

// dateText reads "2023-01-02<br>10:00:00" as "2023-01-02 10:00:00".
func dateText(s *goquery.Selection) string {
	h, _ := s.Html()
	h = reTags.ReplaceAllString(h, " ")
	return reSpaces.ReplaceAllString(strings.TrimSpace(h), " ")
}

var reTags = regexp.MustCompile(`<[^>]*>`)

func atoi(s string) int {
	v, _ := strconv.Atoi(strings.NewReplacer(" ", "", ".", "", ",", "", " ", "").Replace(strings.TrimSpace(s)))
	return v
}

// Download fetches the .torrent with the session (the passkey link works on its own too).
func (n *NCore) Download(ctx context.Context, link string) ([]byte, string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.in {
		if err := n.login(ctx); err != nil {
			return nil, "", err
		}
	}
	for try := 0; try < 2; try++ {
		b, resp, err := n.do(ctx, "GET", link, nil)
		if err != nil {
			return nil, "", err
		}
		if len(b) > 0 && b[0] == 'd' && strings.Contains(string(b[:min(len(b), 4096)]), "4:info") {
			return b, "", nil
		}
		if try == 0 && onLoginPage(resp) {
			if err := n.login(ctx); err != nil {
				return nil, "", err
			}
			continue
		}
		break
	}
	return nil, "", errors.New("nCore: a letöltés nem torrent fájl (lejárt a belépés, vagy elfogyott a letöltési keret)")
}

// Test signs in again and lists the newest film and series releases.
func (n *NCore) Test(ctx context.Context) (int, error) {
	n.mu.Lock()
	n.in = false
	n.mu.Unlock()
	rows, err := n.Search(ctx, cardigann.Query{Type: "search"})
	return len(rows), err
}
