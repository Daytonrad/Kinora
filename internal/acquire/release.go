// Package acquire is Kinora's built-in "get it for me" module: it searches indexers
// (Torznab), scores the releases against a quality profile, hands the chosen one to
// a download client (qBittorrent) and imports the finished file into a library.
// It replaces what Prowlarr, Radarr and Jellyseerr do next to other media servers.
// Off by default (setting acq_enabled); no tracker or indexer ships with Kinora.
package acquire

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Release is what a release name says about the file.
type Release struct {
	Title      string   `json:"title"`
	Year       int      `json:"year,omitempty"`
	Season     int      `json:"season,omitempty"`  // -1 when none
	Episode    int      `json:"episode,omitempty"` // -1 when none (or a whole season pack)
	Resolution int      `json:"resolution,omitempty"`
	Source     string   `json:"source,omitempty"` // remux, bluray, webdl, webrip, hdtv, dvd, cam, ts, tc, scr
	Codec      string   `json:"codec,omitempty"`  // h264, hevc, av1, xvid
	HDR        string   `json:"hdr,omitempty"`    // dv, hdr10+, hdr10, hdr
	Audio      string   `json:"audio,omitempty"`  // atmos, truehd, dtshd, dts, eac3, ac3, aac
	Languages  []string `json:"languages,omitempty"`
	Group      string   `json:"group,omitempty"`
	Proper     bool     `json:"proper,omitempty"`
	Repack     bool     `json:"repack,omitempty"`
	ThreeD     bool     `json:"three_d,omitempty"`
}

var (
	reYearTok  = regexp.MustCompile(`^(19\d\d|20\d\d)$`)
	reSE       = regexp.MustCompile(`(?i)^s(\d{1,2})(?:e(\d{1,3}))?(?:-?e\d{1,3})*$`)
	reRes      = regexp.MustCompile(`(?i)^(\d{3,4})[pi]$`)
	reGroup    = regexp.MustCompile(`-([A-Za-z0-9]+)(?:\.[a-z0-9]{2,4})?$`)
	reSplitter = regexp.MustCompile(`[\s._\[\]()]+`)
)

// tokens that end a title even without a year (quality, source, codec…)
var stopWords = map[string]bool{
	"2160p": true, "1080p": true, "1080i": true, "720p": true, "576p": true, "480p": true, "4k": true, "uhd": true,
	"bluray": true, "blu-ray": true, "bdrip": true, "brrip": true, "bdremux": true, "remux": true, "web-dl": true, "webdl": true,
	"webrip": true, "web": true, "hdtv": true, "dvdrip": true, "dvd": true, "dvdr": true, "hdrip": true,
	"x264": true, "x265": true, "h264": true, "h265": true, "hevc": true, "avc": true, "av1": true, "xvid": true,
	"hdr": true, "hdr10": true, "dv": true, "dovi": true, "proper": true, "repack": true, "internal": true, "multi": true,
	"hun": true, "hungarian": true, "magyar": true, "eng": true, "complete": true,
	"cam": true, "camrip": true, "hdcam": true, "ts": true, "telesync": true, "hdts": true, "tc": true, "telecine": true,
	"scr": true, "screener": true, "dvdscr": true, "extended": true, "unrated": true, "imax": true, "remastered": true,
}

// ParseRelease reads a release name such as "The.Matrix.1999.1080p.BluRay.DTS.HUN.ENG.x264-GRP".
func ParseRelease(name string) Release {
	r := Release{Season: -1, Episode: -1}
	n := strings.TrimSpace(name)
	if m := reGroup.FindStringSubmatch(n); m != nil && !stopWords[strings.ToLower(m[1])] {
		r.Group = m[1]
	}
	low := strings.ToLower(n)
	toks := reSplitter.Split(n, -1)
	var title []string
	titleDone := false
	for i, t := range toks {
		if t == "" {
			continue
		}
		lt := strings.ToLower(t)
		// "x264-GRP": the group is not part of the token's meaning
		if j := strings.Index(lt, "-"); j > 0 && i == len(toks)-1 && r.Group != "" && strings.HasSuffix(t, "-"+r.Group) {
			lt = lt[:j]
		}
		switch {
		case reYearTok.MatchString(lt) && len(title) > 0 && !titleDone:
			// the last year-looking token before the quality part is the year ("2001 A Space Odyssey 1968")
			r.Year, _ = strconv.Atoi(lt)
			titleDone = true
			continue
		case reYearTok.MatchString(lt) && r.Year == 0 && titleDone:
			r.Year, _ = strconv.Atoi(lt)
			continue
		}
		if m := reSE.FindStringSubmatch(lt); m != nil {
			r.Season, _ = strconv.Atoi(m[1])
			if m[2] != "" {
				r.Episode, _ = strconv.Atoi(m[2])
			}
			titleDone = true
			continue
		}
		if m := reRes.FindStringSubmatch(lt); m != nil {
			r.Resolution, _ = strconv.Atoi(m[1])
			titleDone = true
			continue
		}
		if stopWords[lt] {
			titleDone = true
		}
		if !titleDone {
			title = append(title, t)
		}
	}
	// a year glued to the title without a quality part ("Movie 2019")
	if r.Year == 0 && len(title) > 1 && reYearTok.MatchString(title[len(title)-1]) {
		r.Year, _ = strconv.Atoi(title[len(title)-1])
		title = title[:len(title)-1]
	}
	r.Title = strings.TrimSpace(strings.Join(title, " "))

	has := func(words ...string) bool {
		for _, w := range words {
			if containsToken(low, w) {
				return true
			}
		}
		return false
	}
	if r.Resolution == 0 {
		switch {
		case has("4k", "uhd"):
			r.Resolution = 2160
		}
	}
	switch {
	case has("remux", "bdremux"):
		r.Source = "remux"
	case has("bluray", "blu-ray", "bdrip", "brrip", "bd"):
		r.Source = "bluray"
	case has("web-dl", "webdl", "amzn", "nf", "dsnp", "hmax", "atvp") && !has("webrip"):
		r.Source = "webdl"
	case has("webrip", "web"):
		r.Source = "webrip"
	case has("hdtv", "pdtv", "dsr"):
		r.Source = "hdtv"
	case has("dvdscr", "scr", "screener"):
		r.Source = "scr"
	case has("dvdrip", "dvd", "dvdr", "dvd5", "dvd9"):
		r.Source = "dvd"
	case has("cam", "camrip", "hdcam"):
		r.Source = "cam"
	case has("ts", "telesync", "hdts", "pdvd"):
		r.Source = "ts"
	case has("tc", "telecine"):
		r.Source = "tc"
	}
	switch {
	case has("x265", "h265", "hevc", "h.265"):
		r.Codec = "hevc"
	case has("x264", "h264", "avc", "h.264"):
		r.Codec = "h264"
	case has("av1"):
		r.Codec = "av1"
	case has("xvid", "divx"):
		r.Codec = "xvid"
	}
	switch {
	case has("dv", "dovi", "dolbyvision", "dolby-vision"):
		r.HDR = "dv"
	case has("hdr10+", "hdr10plus"):
		r.HDR = "hdr10+"
	case has("hdr10"):
		r.HDR = "hdr10"
	case has("hdr"):
		r.HDR = "hdr"
	}
	switch {
	case has("atmos"):
		r.Audio = "atmos"
	case has("truehd"):
		r.Audio = "truehd"
	case has("dts-hd", "dtshd", "dts-ma", "dts-x", "dtsx"):
		r.Audio = "dtshd"
	case has("dts"):
		r.Audio = "dts"
	case has("ddp", "dd+", "eac3", "e-ac3", "ddp5", "ddp2", "ddp7"):
		r.Audio = "eac3"
	case has("dd", "ac3", "dd5", "dd2"):
		r.Audio = "ac3"
	case has("aac"):
		r.Audio = "aac"
	}
	if has("hun", "hungarian", "magyar", "hunsub") || strings.Contains(low, ".hun.") || strings.Contains(low, ".hun-") {
		r.Languages = append(r.Languages, "hun")
	}
	if has("eng", "english") {
		r.Languages = append(r.Languages, "eng")
	}
	if has("multi") {
		r.Languages = append(r.Languages, "multi")
	}
	r.Proper = has("proper")
	r.Repack = has("repack", "rerip")
	r.ThreeD = has("3d", "sbs", "hsbs", "hou")
	return r
}

// containsToken reports whether w appears in s as a whole token (separated by
// non-alphanumerics), so "ts" does not match "tsunami" and "dd" not "kiddo".
func containsToken(s, w string) bool {
	for i := 0; ; {
		j := strings.Index(s[i:], w)
		if j < 0 {
			return false
		}
		a, b := i+j, i+j+len(w)
		left := a == 0 || !isAlnum(rune(s[a-1]))
		right := b == len(s) || !isAlnum(rune(s[b]))
		// "ddp5.1", "dd5.1", "ddp2.0": a digit may follow audio codecs
		if !right && (w == "dd" || w == "ddp" || w == "dts" || w == "aac" || w == "ac3") && b < len(s) && s[b] >= '0' && s[b] <= '9' {
			right = true
		}
		if left && right {
			return true
		}
		i = a + 1
	}
}

func isAlnum(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// NormalizeTitle folds a title for comparison: lower case, accents removed, only letters and digits.
func NormalizeTitle(s string) string {
	s = strings.ToLower(strings.TrimSpace(reSplitter.ReplaceAllString(s, " ")))
	for _, p := range []string{"the ", "a ", "an "} { // "The Matrix" == "Matrix"
		if strings.HasPrefix(s, p) && len(s) > len(p)+2 {
			s = s[len(p):]
			break
		}
	}
	var b strings.Builder
	for _, r := range s {
		r = fold(r)
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var folds = map[rune]rune{'á': 'a', 'é': 'e', 'í': 'i', 'ó': 'o', 'ö': 'o', 'ő': 'o', 'ú': 'u', 'ü': 'u', 'ű': 'u',
	'à': 'a', 'â': 'a', 'ä': 'a', 'ã': 'a', 'å': 'a', 'è': 'e', 'ê': 'e', 'ë': 'e', 'ì': 'i', 'î': 'i', 'ï': 'i',
	'ò': 'o', 'ô': 'o', 'õ': 'o', 'ù': 'u', 'û': 'u', 'ç': 'c', 'ñ': 'n', 'ß': 's', '&': 'n'}

func fold(r rune) rune {
	if f, ok := folds[r]; ok {
		return f
	}
	return r
}

// QualityLabel is the short label used in file names and the UI: "1080p BluRay".
func (r Release) QualityLabel() string {
	var parts []string
	if r.Resolution > 0 {
		parts = append(parts, strconv.Itoa(r.Resolution)+"p")
	}
	if s := sourceLabel[r.Source]; s != "" {
		parts = append(parts, s)
	}
	return strings.Join(parts, " ")
}

var sourceLabel = map[string]string{"remux": "Remux", "bluray": "BluRay", "webdl": "WEB-DL", "webrip": "WEBRip", "hdtv": "HDTV",
	"dvd": "DVD", "cam": "CAM", "ts": "TS", "tc": "TC", "scr": "SCR"}
