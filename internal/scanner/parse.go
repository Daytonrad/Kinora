package scanner

import (
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Parsed is what can be learned from a file's relative path alone.
type Parsed struct {
	Kind       string // "movie" | "episode"
	Title      string // movie title, or episode title (may be empty)
	Year       int
	Series     string
	SeriesYear int
	SeriesDir  string // relative dir of the series folder ("" for flat layouts)
	SeriesKey  string
	Season     int
	Episode    int // 0 = unknown
	EpisodeEnd int // last episode of a multi-episode file (0 = single)
}

var (
	reSxxEyy  = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])s(\d{1,3})[ ._-]*e(\d{1,4})(?:[ ._-]*-?[ ._-]*e(\d{1,4}))?`)
	reNxNN    = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(\d{1,2})x(\d{2,3})(?:[^0-9p]|$)`)
	reLeadNum = regexp.MustCompile(`^(?:e|ep|episode)?[ ._-]*(\d{1,3})(?:[ ._-]+(.*))?$`)

	reSeasonDir = []*regexp.Regexp{
		regexp.MustCompile(`(?i)^(?:season|staffel|saison|temporada|stagione|évad|evad|series)[ ._-]*(\d{1,3})$`),
		regexp.MustCompile(`(?i)^s(\d{1,3})$`),
		regexp.MustCompile(`(?i)^(\d{1,3})\.?[ ._-]*(?:évad|evad|season|staffel)$`),
	}
	reSpecialsDir = regexp.MustCompile(`(?i)^(?:specials?|extras|különkiadások|bonus)$`)
	reYear        = regexp.MustCompile(`^(19\d\d|20\d\d)$`)
)

// junk tokens mark the end of a title (release group noise).
var junk = map[string]bool{}

func init() {
	for _, t := range strings.Fields(`2160p 1440p 1080p 1080i 720p 576p 480p 360p 4k 8k uhd fhd hd sd
		bluray blu-ray bdrip brrip bdremux remux web-dl webdl webrip web hdtv dvdrip dvd dvdscr hdrip hdcam cam ts tc
		x264 x265 h264 h265 h.264 h.265 hevc avc xvid divx av1 vp9 10bit 8bit 12bit hi10p
		hdr hdr10 hdr10+ hdr10plus dv dovi dolby vision sdr imax
		atmos truehd dts dts-hd dts-x ddp ddp5 ddp5.1 dd5.1 dd+ ac3 eac3 aac aac2.0 aac5.1 flac mp3 opus 5.1 7.1 2.0
		hun eng ger fre spa ita multi dual dub dubbed sub subs subbed hunsub magyar felirat szinkron
		proper repack extended unrated uncut directors remastered limited internal readnfo
		yify yts rarbg ettv eztv`) {
		junk[t] = true
	}
}

// tokenize splits a name into words: separators are dots, underscores,
// spaces, brackets; hyphens are kept inside words (web-dl) but dropped alone.
func tokenize(s string) []string {
	f := func(r rune) bool {
		return r == '.' || r == '_' || r == ' ' || r == '(' || r == ')' || r == '[' || r == ']' || r == '{' || r == '}' || r == ','
	}
	var out []string
	for _, t := range strings.FieldsFunc(s, f) {
		t = strings.Trim(t, "-")
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// CleanName turns a release-style name into (title, year).
// The last plausible year token wins ("Blade Runner 2049 (2017)" → 2017),
// but a year cannot be the very first token ("2001 A Space Odyssey").
func CleanName(name string) (string, int) {
	toks := tokenize(name)
	yearIdx := -1
	for i, t := range toks {
		if i > 0 && reYear.MatchString(t) {
			yearIdx = i
		}
	}
	end := len(toks)
	year := 0
	if yearIdx > 0 {
		year, _ = strconv.Atoi(toks[yearIdx])
		end = yearIdx
	}
	for i := 0; i < end; i++ {
		if i > 0 && junk[strings.ToLower(toks[i])] {
			end = i
			break
		}
	}
	return strings.Join(toks[:end], " "), year
}

// cleanEpisodeTitle strips leading separators and trailing junk.
func cleanEpisodeTitle(s string) string {
	toks := tokenize(s)
	for i, t := range toks {
		if junk[strings.ToLower(t)] || reYear.MatchString(t) && i > 0 && i == len(toks)-1 {
			toks = toks[:i]
			break
		}
	}
	return strings.Join(toks, " ")
}

// SeasonFromDir reports whether a folder name is a season folder.
func SeasonFromDir(name string) (int, bool) {
	name = strings.TrimSpace(name)
	if reSpecialsDir.MatchString(name) {
		return 0, true
	}
	for _, re := range reSeasonDir {
		if m := re.FindStringSubmatch(name); m != nil {
			n, _ := strconv.Atoi(m[1])
			return n, true
		}
	}
	return 0, false
}

// ParseFile interprets a path relative to the library root (forward slashes).
// libKind is "movies", "shows" or "mixed".
func ParseFile(rel, libKind string) Parsed {
	rel = strings.TrimPrefix(path.Clean(strings.ReplaceAll(rel, `\`, "/")), "/")
	dir, file := path.Split(rel)
	dir = strings.TrimSuffix(dir, "/")
	base := strings.TrimSuffix(file, path.Ext(file))
	var dirs []string
	if dir != "" {
		dirs = strings.Split(dir, "/")
	}

	if libKind != "movies" {
		if p, ok := parseEpisode(dirs, base, libKind == "shows"); ok {
			return p
		}
	}
	return parseMovie(dirs, base)
}

// EpisodeInMoviesLibrary parses a file of a movies library as an episode when its name
// carries an explicit S01E02. The scanner uses it only when at least two files belong to
// the same series, so a single oddly named film stays a film.
func EpisodeInMoviesLibrary(rel string) (Parsed, bool) {
	rel = strings.TrimPrefix(path.Clean(strings.ReplaceAll(rel, `\`, "/")), "/")
	dir, file := path.Split(rel)
	dir = strings.TrimSuffix(dir, "/")
	base := strings.TrimSuffix(file, path.Ext(file))
	if !reSxxEyy.MatchString(base) {
		return Parsed{}, false
	}
	var dirs []string
	if dir != "" {
		dirs = strings.Split(dir, "/")
	}
	return parseEpisode(dirs, base, false)
}

func parseMovie(dirs []string, base string) Parsed {
	title, year := CleanName(base)
	if len(dirs) > 0 {
		folder := dirs[len(dirs)-1]
		ft, fy := CleanName(folder)
		own := fy > 0 || (ft != "" && strings.HasPrefix(normalize(title), normalize(ft)))
		if _, isSeason := SeasonFromDir(folder); own && ft != "" && !isSeason {
			title = ft
			if fy > 0 {
				year = fy
			}
		}
	}
	if title == "" {
		title = base
	}
	return Parsed{Kind: "movie", Title: title, Year: year}
}

func parseEpisode(dirs []string, base string, force bool) (Parsed, bool) {
	p := Parsed{Kind: "episode", Season: -1}
	prefix := ""
	matched := false
	if m := reSxxEyy.FindStringSubmatchIndex(base); m != nil {
		p.Season = atoi(base[m[2]:m[3]])
		p.Episode = atoi(base[m[4]:m[5]])
		if m[6] >= 0 {
			p.EpisodeEnd = atoi(base[m[6]:m[7]])
		}
		prefix = base[:m[0]]
		p.Title = cleanEpisodeTitle(base[m[1]:])
		matched = true
	} else if m := reNxNN.FindStringSubmatchIndex(base); m != nil {
		p.Season = atoi(base[m[2]:m[3]])
		p.Episode = atoi(base[m[4]:m[5]])
		prefix = base[:m[0]]
		p.Title = cleanEpisodeTitle(base[m[5]:])
		matched = true
	}

	// season folder
	seasonIdx := -1
	for i := len(dirs) - 1; i >= 0; i-- {
		if n, ok := SeasonFromDir(dirs[i]); ok {
			seasonIdx = i
			if p.Season < 0 {
				p.Season = n
			}
			break
		}
	}
	if !matched && seasonIdx >= 0 && seasonIdx == len(dirs)-1 {
		// "Dark/Season 2/03 - Ghosts.mkv"
		if m := reLeadNum.FindStringSubmatch(strings.TrimSpace(base)); m != nil {
			p.Episode = atoi(m[1])
			p.Title = cleanEpisodeTitle(m[2])
			matched = true
		}
	}
	if !matched && !force {
		return Parsed{}, false
	}

	// series folder
	switch {
	case seasonIdx > 0:
		p.SeriesDir = strings.Join(dirs[:seasonIdx], "/")
		p.Series, p.SeriesYear = CleanName(dirs[seasonIdx-1])
	case seasonIdx < 0 && len(dirs) > 0:
		p.SeriesDir = dirs[0]
		p.Series, p.SeriesYear = CleanName(dirs[0])
	default:
		// flat layout: "Seinfeld.S03E10.The.Old.Man.mkv"
		p.Series, p.SeriesYear = CleanName(prefix)
	}
	if p.Series == "" {
		p.Series, _ = CleanName(prefix)
	}
	if p.Series == "" {
		p.Series = "Ismeretlen sorozat"
	}
	if p.SeriesDir != "" {
		p.SeriesKey = "series:" + strings.ToLower(p.SeriesDir)
	} else {
		p.SeriesKey = "flat:" + normalize(p.Series)
	}
	if p.Season < 0 {
		p.Season = 1
	}
	if !matched && p.Title == "" {
		p.Title, _ = CleanName(base)
	}
	return p, true
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

// normalize lowercases and keeps only letters/digits (for comparisons).
func normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// SortTitle lowercases and drops leading articles (English and Hungarian).
func SortTitle(t string) string {
	s := strings.ToLower(strings.TrimSpace(t))
	for _, a := range []string{"the ", "a ", "an ", "az ", "egy "} {
		if strings.HasPrefix(s, a) && len(s) > len(a) {
			s = s[len(a):]
			break
		}
	}
	return strings.TrimLeft(s, " '\"¡¿.-")
}

// ---------------------------------------------------------------- languages

var langMap = map[string]string{}

func init() {
	for code, names := range map[string]string{
		"hun": "hu hun hungarian magyar",
		"eng": "en eng english angol",
		"ger": "de ger deu german deutsch német nemet",
		"fre": "fr fre fra french français francais francia",
		"spa": "es spa spanish español espanol spanyol",
		"ita": "it ita italian italiano olasz",
		"por": "pt por portuguese portugués portugal portugál",
		"pob": "pb pob pt-br ptbr brazilian",
		"rus": "ru rus russian русский orosz",
		"pol": "pl pol polish polski lengyel",
		"cze": "cs cze ces czech čeština cseh",
		"slo": "sk slo slk slovak szlovák",
		"rum": "ro rum ron romanian román",
		"hrv": "hr hrv croatian horvát",
		"srp": "sr srp serbian szerb",
		"slv": "sl slv slovenian szlovén",
		"ukr": "uk ukr ukrainian ukrán",
		"dut": "nl dut nld dutch holland",
		"swe": "sv swe swedish svéd",
		"nor": "no nor nob norwegian norvég",
		"dan": "da dan danish dán",
		"fin": "fi fin finnish finn",
		"gre": "el gre ell greek görög",
		"tur": "tr tur turkish török",
		"jpn": "ja jpn japanese japán",
		"kor": "ko kor korean koreai",
		"chi": "zh chi zho chinese kínai",
		"ara": "ar ara arabic arab",
		"heb": "he heb hebrew héber",
		"hin": "hi hin hindi",
		"bul": "bg bul bulgarian bolgár",
	} {
		for _, n := range strings.Fields(names) {
			langMap[n] = code
		}
	}
}

// NormalizeLang maps 2/3 letter codes and English/Hungarian names to a
// 3-letter (ISO 639-2/B) code. Unknown values return "".
func NormalizeLang(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if c, ok := langMap[s]; ok {
		return c
	}
	if len(s) == 3 && strings.Trim(s, "abcdefghijklmnopqrstuvwxyz") == "" && s != "und" {
		return s
	}
	return ""
}
