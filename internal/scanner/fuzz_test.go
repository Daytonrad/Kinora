package scanner

import (
	"math"
	"strings"
	"testing"
	"unicode/utf8"
)

// Seeds cover the layouts the scanner documents plus inputs that broke parsers elsewhere:
// double episodes, years in titles, Unicode, empty and dot-only names, very long names.
var parseSeeds = []string{
	"Movies/Blade Runner 2049 (2017)/Blade Runner 2049 (2017).mkv",
	"The.Matrix.1999.1080p.BluRay.x264.mkv",
	"Breaking Bad (2008)/Season 01/Breaking.Bad.S01E02.mkv",
	"Dark/Staffel 2/03 - Ghosts.mkv",
	"Aranyélet/2. évad/Aranyelet.S02E04.mkv",
	"Futurama/Specials/Futurama.S00E01.mkv",
	"Seinfeld.S03E10.The.Old.Man.mkv",
	"Show/Season 1/Show.S01E01E02.mkv",
	"Show/Season 1/Show.S01E01-E03.mkv",
	"Show.1x05.mkv",
	"2001 A Space Odyssey (1968).mkv",
	"1917 (2019)/1917.mkv",
	"Show.S2024E1000.mkv",
	"Movie {imdb-tt0133093}/Movie.mkv",
	"Film [imdbid-tt0133093] (1999).mkv",
	"",
	".",
	"..",
	"/",
	"../../etc/passwd",
	`C:\Media\Show\Season 01\Show - 01.mkv`,
	"日本語のタイトル (2020)/日本語のタイトル.mkv",
	"Ünnepi.Műsor.S01E01.mkv",
	"\xff\xfe.mkv",
	strings.Repeat("Very.Long.Name.", 40) + "S01E01.mkv",
}

func FuzzParseFile(f *testing.F) {
	for _, s := range parseSeeds {
		for _, k := range []string{"movies", "shows", "mixed"} {
			f.Add(s, k)
		}
	}
	f.Fuzz(func(t *testing.T, rel, kind string) {
		p := ParseFile(rel, kind)
		if p.Kind != "movie" && p.Kind != "episode" {
			t.Fatalf("ParseFile(%q, %q): kind %q", rel, kind, p.Kind)
		}
		if p.Season < 0 || p.Episode < 0 || p.EpisodeEnd < 0 {
			t.Fatalf("ParseFile(%q, %q): negative numbers %+v", rel, kind, p)
		}
		if p.EpisodeEnd != 0 && p.EpisodeEnd < p.Episode {
			t.Fatalf("ParseFile(%q, %q): episode range %d-%d", rel, kind, p.Episode, p.EpisodeEnd)
		}
		if utf8.ValidString(rel) && (!utf8.ValidString(p.Title) || !utf8.ValidString(p.Series)) {
			t.Fatalf("ParseFile(%q, %q): invalid UTF-8 in %+v", rel, kind, p)
		}
		if p2, ok := EpisodeInMoviesLibrary(rel); ok && p2.Kind != "episode" {
			t.Fatalf("EpisodeInMoviesLibrary(%q): kind %q", rel, p2.Kind)
		}
	})
}

func FuzzNameHelpers(f *testing.F) {
	for _, s := range parseSeeds {
		f.Add(s)
	}
	for _, s := range []string{"Season 01", "S2", "3. évad", "Staffel 10", "Specials", "eng", "hun", "pt-BR", "zh_Hans"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		title, year := CleanName(s)
		if year != 0 && (year < 1800 || year > 2200) {
			t.Fatalf("CleanName(%q): year %d", s, year)
		}
		if utf8.ValidString(s) && !utf8.ValidString(title) {
			t.Fatalf("CleanName(%q): invalid UTF-8 %q", s, title)
		}
		if n, ok := SeasonFromDir(s); ok && n < 0 {
			t.Fatalf("SeasonFromDir(%q) = %d", s, n)
		}
		if id := ImdbFromName(s); id != "" && !strings.HasPrefix(id, "tt") {
			t.Fatalf("ImdbFromName(%q) = %q", s, id)
		}
		SortTitle(s)
		NormalizeLang(s)
	})
}

func FuzzParseNFO(f *testing.F) {
	for _, s := range []string{
		`<movie><title>The Matrix</title><year>1999</year><rating>8.7</rating><genre>Action / Sci-Fi</genre><mpaa>US:R / HU:16</mpaa><uniqueid type="imdb">tt0133093</uniqueid><uniqueid type="tmdb">603</uniqueid></movie>`,
		`<tvshow><title>Dark</title><premiered>2017-12-01</premiered><ratings><rating name="imdb" default="true"><value>8,7</value></rating></ratings></tvshow>`,
		`<?xml version="1.0" encoding="ISO-8859-2"?><movie><title>Árvíztűrő</title></movie>`,
		`<movie><rating>NaN</rating><rating>Inf</rating></movie>`,
		`<movie><title>`,
		`https://www.imdb.com/title/tt0133093/`,
		`<movie><id>tt0133093</id><year>-5</year><premiered>99</premiered></movie>`,
		`<movie><tmdbid>-1</tmdbid></movie>`,
		`<movie><uniqueid type="tmdb">-603</uniqueid></movie>`,
		``,
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		m := parseNFO(data)
		if m == nil {
			return
		}
		if m.Rating != nil && (math.IsNaN(*m.Rating) || math.IsInf(*m.Rating, 0) || *m.Rating <= 0) {
			t.Fatalf("rating %v from %q", *m.Rating, data)
		}
		for _, g := range m.Genres {
			if strings.TrimSpace(g) == "" {
				t.Fatalf("empty genre from %q", data)
			}
		}
		if m.TmdbID < 0 {
			// a negative id would collide with the "manually edited" marker (-1)
			t.Fatalf("tmdb id %d from %q", m.TmdbID, data)
		}
	})
}
