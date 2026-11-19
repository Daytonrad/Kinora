package cardigann

import (
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func argList(a any) []string {
	switch v := a.(type) {
	case nil:
		return nil
	case []any:
		out := make([]string, len(v))
		for i, x := range v {
			out[i] = fmt.Sprint(x)
		}
		return out
	case []string:
		return v
	}
	return []string{fmt.Sprint(a)}
}

func arg(a any, i int) string {
	l := argList(a)
	if i < len(l) {
		return l[i]
	}
	return ""
}

// applyFilters runs a value through a definition's filter chain.
func applyFilters(v string, filters []Filter, ctx *templateCtx) (string, error) {
	for _, f := range filters {
		var err error
		if v, err = applyFilter(v, f, ctx); err != nil {
			return "", fmt.Errorf("%s szűrő: %w", f.Name, err)
		}
	}
	return v, nil
}

func applyFilter(v string, f Filter, ctx *templateCtx) (string, error) {
	a0 := arg(f.Args, 0)
	if ctx != nil && strings.Contains(a0, "{{") {
		a0, _ = render(a0, ctx)
	}
	switch strings.ToLower(f.Name) {
	case "querystring":
		u, err := url.Parse(strings.TrimSpace(v))
		if err != nil {
			return "", err
		}
		return u.Query().Get(a0), nil
	case "prepend":
		return a0 + v, nil
	case "append":
		return v + a0, nil
	case "replace":
		return strings.ReplaceAll(v, a0, arg(f.Args, 1)), nil
	case "re_replace":
		re, err := compileRegexp(a0)
		if err != nil {
			return "", err
		}
		return re.ReplaceAllString(v, dotnetReplacement(arg(f.Args, 1))), nil
	case "regexp":
		re, err := compileRegexp(a0)
		if err != nil {
			return "", err
		}
		m := re.FindStringSubmatch(v)
		switch {
		case m == nil:
			return "", nil
		case len(m) > 1:
			return m[1], nil
		}
		return m[0], nil
	case "split":
		parts := strings.Split(v, a0)
		i, _ := strconv.Atoi(arg(f.Args, 1))
		if i < 0 {
			i += len(parts)
		}
		if i < 0 || i >= len(parts) {
			return "", nil
		}
		return parts[i], nil
	case "trim":
		if a0 != "" {
			return strings.Trim(v, a0), nil
		}
		return strings.TrimSpace(v), nil
	case "tolower":
		return strings.ToLower(v), nil
	case "toupper":
		return strings.ToUpper(v), nil
	case "urlencode":
		return url.QueryEscape(v), nil
	case "urldecode":
		d, err := url.QueryUnescape(v)
		if err != nil {
			return v, nil
		}
		return d, nil
	case "htmldecode":
		return html.UnescapeString(v), nil
	case "htmlencode":
		return html.EscapeString(v), nil
	case "diacritics":
		return stripDiacritics(v), nil
	case "dateparse", "timeparse":
		t, err := parseDate(v, a0)
		if err != nil {
			return "", err
		}
		return t.Format(time.RFC3339), nil
	case "timeago", "reltime":
		return timeAgo(v).Format(time.RFC3339), nil
	case "fuzzytime":
		t, err := fuzzyTime(v)
		if err != nil {
			return "", err
		}
		return t.Format(time.RFC3339), nil
	case "validfilename":
		return strings.Map(func(r rune) rune {
			if strings.ContainsRune(`<>:"/\|?*`, r) || r < 32 {
				return '_'
			}
			return r
		}, v), nil
	case "jsonjoinarray":
		var x any
		if err := json.Unmarshal([]byte(v), &x); err != nil {
			return "", err
		}
		if a0 != "" && a0 != "$" {
			x = jsonPath(x, a0)
		}
		return strings.Join(argList(x), arg(f.Args, 1)), nil
	case "validate":
		allowed := map[string]bool{}
		for _, w := range strings.Split(strings.ToLower(a0), ",") {
			allowed[strings.TrimSpace(w)] = true
		}
		var keep []string
		for _, w := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' }) {
			if allowed[strings.ToLower(w)] {
				keep = append(keep, w)
			}
		}
		return strings.Join(keep, ","), nil
	case "strdump", "hexdump", "andmatch":
		return v, nil
	}
	return "", fmt.Errorf("ismeretlen szűrő: %s", f.Name)
}

var diacritics = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ä", "a", "ã", "a", "å", "a", "é", "e", "è", "e", "ê", "e", "ë", "e", "í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "ö", "o", "õ", "o", "ő", "o", "ú", "u", "ù", "u", "û", "u", "ü", "u", "ű", "u", "ç", "c", "ñ", "n", "ý", "y",
	"Á", "A", "À", "A", "Â", "A", "Ä", "A", "Ã", "A", "Å", "A", "É", "E", "È", "E", "Ê", "E", "Ë", "E", "Í", "I", "Ì", "I", "Î", "I", "Ï", "I",
	"Ó", "O", "Ò", "O", "Ô", "O", "Ö", "O", "Õ", "O", "Ő", "O", "Ú", "U", "Ù", "U", "Û", "U", "Ü", "U", "Ű", "U", "Ç", "C", "Ñ", "N", "Ý", "Y", "ß", "ss")

func stripDiacritics(s string) string { return diacritics.Replace(s) }

// dotnetLayout turns a .NET date format ("yyyy-MM-dd HH:mm") into a Go layout; a Go layout passes through.
func dotnetLayout(f string) string {
	if strings.Contains(f, "2006") || strings.Contains(f, "Jan") || strings.Contains(f, "15:04") {
		return f
	}
	repl := []struct{ a, b string }{
		{"yyyy", "2006"}, {"yy", "06"}, {"MMMM", "January"}, {"MMM", "Jan"}, {"MM", "01"}, {"dddd", "Monday"}, {"ddd", "Mon"},
		{"dd", "02"}, {"HH", "15"}, {"hh", "03"}, {"mm", "04"}, {"ss", "05"}, {"tt", "PM"}, {"zzz", "-07:00"}, {"fff", "000"},
	}
	var b strings.Builder
	for i := 0; i < len(f); {
		matched := false
		for _, r := range repl {
			if strings.HasPrefix(f[i:], r.a) {
				b.WriteString(r.b)
				i += len(r.a)
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		switch f[i] {
		case 'M':
			b.WriteString("1")
		case 'd':
			b.WriteString("2")
		case 'H':
			b.WriteString("15")
		case 'h':
			b.WriteString("3")
		case 'm':
			b.WriteString("4")
		case 's':
			b.WriteString("5")
		case '\'':
		default:
			b.WriteByte(f[i])
		}
		i++
	}
	return b.String()
}

func parseDate(v, layout string) (time.Time, error) {
	v = strings.TrimSpace(v)
	if layout != "" {
		if t, err := time.ParseInLocation(dotnetLayout(layout), v, time.Local); err == nil {
			return t, nil
		}
	}
	return fuzzyTime(v)
}

var reAgo = regexp.MustCompile(`(?i)(\d+(?:[.,]\d+)?)\s*(sec|second|min|minute|hour|hr|day|week|wk|month|mo|year|yr)s?`)

// timeAgo reads "3 hours ago", "2 days, 4 hours", "1 week".
func timeAgo(v string) time.Time {
	t := time.Now()
	for _, m := range reAgo.FindAllStringSubmatch(v, -1) {
		n, _ := strconv.ParseFloat(strings.Replace(m[1], ",", ".", 1), 64)
		var d time.Duration
		switch strings.ToLower(m[2]) {
		case "sec", "second":
			d = time.Second
		case "min", "minute":
			d = time.Minute
		case "hour", "hr":
			d = time.Hour
		case "day":
			d = 24 * time.Hour
		case "week", "wk":
			d = 7 * 24 * time.Hour
		case "month", "mo":
			d = 30 * 24 * time.Hour
		case "year", "yr":
			d = 365 * 24 * time.Hour
		}
		t = t.Add(-time.Duration(n * float64(d)))
	}
	return t
}

var dateLayouts = []string{time.RFC3339, time.RFC1123Z, time.RFC1123, "2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02T15:04:05", "2006-01-02",
	"2006.01.02 15:04:05", "2006.01.02 15:04", "2006.01.02", "02/01/2006 15:04", "02.01.2006 15:04", "02.01.2006", "Jan 2 2006", "Jan 2, 2006", "2 Jan 2006",
	"Mon, 02 Jan 2006 15:04:05", "01/02/2006"}

// fuzzyTime reads common date forms, "today 10:00", "yesterday", "now" and "… ago".
func fuzzyTime(v string) (time.Time, error) {
	s := strings.TrimSpace(v)
	low := strings.ToLower(s)
	now := time.Now()
	switch {
	case low == "now" || low == "just now":
		return now, nil
	case strings.HasPrefix(low, "today"), strings.HasPrefix(low, "ma"):
		return withClock(now, s), nil
	case strings.HasPrefix(low, "yesterday"), strings.HasPrefix(low, "tegnap"):
		return withClock(now.AddDate(0, 0, -1), s), nil
	case strings.Contains(low, "ago") || reAgo.MatchString(low) && !strings.ContainsAny(low, "-/."):
		return timeAgo(low), nil
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 1e9 { // unix time
		return time.Unix(n, 0), nil
	}
	for _, l := range dateLayouts {
		if t, err := time.ParseInLocation(l, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("ismeretlen dátum: %q", v)
}

var reClock = regexp.MustCompile(`(\d{1,2}):(\d{2})`)

func withClock(day time.Time, s string) time.Time {
	h, m := 0, 0
	if c := reClock.FindStringSubmatch(s); c != nil {
		h, _ = strconv.Atoi(c[1])
		m, _ = strconv.Atoi(c[2])
	}
	return time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, time.Local)
}

var reSize = regexp.MustCompile(`(?i)([\d.,]+)\s*([kmgtp]?i?b|bytes?)?`)

// parseSize reads "1.4 GiB", "700 MB", "1,234.5 KB", "123456789".
func parseSize(v string) int64 {
	m := reSize.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return 0
	}
	num := m[1]
	switch {
	case strings.Count(num, ",") > 0 && strings.Count(num, ".") > 0: // 1,234.5
		num = strings.ReplaceAll(num, ",", "")
	case strings.Count(num, ",") == 1 && len(num)-strings.Index(num, ",") != 4: // 1,4 (decimal comma)
		num = strings.Replace(num, ",", ".", 1)
	default:
		num = strings.ReplaceAll(num, ",", "")
	}
	f, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0
	}
	mult := 1.0
	switch strings.ToUpper(strings.TrimSuffix(strings.TrimSuffix(strings.ToUpper(m[2]), "S"), "E")) {
	case "KB", "KIB":
		mult = 1 << 10
	case "MB", "MIB":
		mult = 1 << 20
	case "GB", "GIB":
		mult = 1 << 30
	case "TB", "TIB":
		mult = 1 << 40
	case "PB", "PIB":
		mult = 1 << 50
	}
	return int64(f * mult)
}

// jsonPath reads "a.b.0.c" (an optional "$." prefix) from decoded JSON.
func jsonPath(x any, path string) any {
	path = strings.TrimPrefix(strings.TrimPrefix(path, "$"), ".")
	if path == "" {
		return x
	}
	for _, p := range strings.Split(path, ".") {
		switch v := x.(type) {
		case map[string]any:
			x = v[p]
		case []any:
			i, err := strconv.Atoi(p)
			if err != nil || i < 0 || i >= len(v) {
				return nil
			}
			x = v[i]
		default:
			return nil
		}
	}
	return x
}
