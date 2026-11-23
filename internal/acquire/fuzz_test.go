package acquire

import (
	"strings"
	"testing"
	"unicode/utf8"
)

var releaseSeeds = []string{
	"The.Matrix.1999.1080p.BluRay.DTS.HUN.ENG.x264-GRP",
	"2001.A.Space.Odyssey.1968.2160p.UHD.BluRay.REMUX.HDR10.HEVC.TrueHD.Atmos-FGT",
	"Blade.Runner.2049.2017.1080p.WEB-DL.DDP5.1.H.264-NTb",
	"Dark.S02E03.720p.NF.WEBRip.x264-GRP",
	"Show.S01E01E02E03.1080p.HDTV.x264-GRP",
	"Show.S01E01-E02.HUN.720p",
	"Show.S03.COMPLETE.1080p.AMZN.WEB-DL",
	"Movie 2019",
	"Tsunami.2020.HDTS.XviD",
	"Film.3D.HSBS.1080p.BluRay",
	"Árvíztűrő.Tükörfúrógép.2021.MAGYAR.WEBRip",
	"[Group] Anime - 01 [1080p].mkv",
	"",
	"-",
	"....",
	"x264-",
	"S99E999",
	"İstanbul.2020.1080p",
	strings.Repeat("A.", 300) + "1080p",
}

func FuzzParseRelease(f *testing.F) {
	for _, s := range releaseSeeds {
		f.Add(s, "The Matrix", 1999, int64(8<<30), 12)
	}
	prof := DefaultProfile()
	all := Profile{Resolutions: []int{2160, 1080, 720, 576, 480, 0}, MinSeeders: 0}
	f.Fuzz(func(t *testing.T, name, title string, year int, size int64, seeders int) {
		r := ParseRelease(name)
		if r.Season < -1 || r.Episode < -1 || r.Resolution < 0 {
			t.Fatalf("ParseRelease(%q): negative numbers %+v", name, r)
		}
		for _, e := range r.Episodes {
			if e < 0 {
				t.Fatalf("ParseRelease(%q): episode %d", name, e)
			}
		}
		if r.Year != 0 && (r.Year < 1900 || r.Year > 2099) {
			t.Fatalf("ParseRelease(%q): year %d", name, r.Year)
		}
		if utf8.ValidString(name) && (!utf8.ValidString(r.Title) || !utf8.ValidString(r.Group)) {
			t.Fatalf("ParseRelease(%q): invalid UTF-8 %+v", name, r)
		}
		if strings.TrimSpace(r.Title) != r.Title {
			t.Fatalf("ParseRelease(%q): untrimmed title %q", name, r.Title)
		}
		for _, p := range []Profile{prof, all} {
			if score, rej := p.Score(r, size, seeders); rej != "" && score != 0 {
				t.Fatalf("Score(%q): rejected %q with score %d", name, rej, score)
			}
		}
		TitleMatches(name, r, year, title, r.Title)
		r.QualityLabel()
		NormalizeTitle(name)
	})
}

func FuzzParseProfile(f *testing.F) {
	for _, s := range []string{
		``, `{}`, `null`, `[]`, `{"resolutions":[1080]}`, `{"resolutions":null,"blocked_sources":[]}`,
		`{"min_size_gb":-1,"max_size_gb":1e308,"min_seeders":-5}`, `{"resolutions":"1080"}`, `{`,
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		p := ParseProfile(s)
		if len(p.Resolutions) == 0 {
			t.Fatalf("ParseProfile(%q): no resolutions", s)
		}
		p.Score(ParseRelease("Movie.2020.1080p.BluRay.x264-GRP"), 5<<30, 10)
	})
}
