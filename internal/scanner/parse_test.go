package scanner

import "testing"

func TestCleanName(t *testing.T) {
	cases := []struct {
		in    string
		title string
		year  int
	}{
		{"The.Matrix.1999.1080p.BluRay.x264", "The Matrix", 1999},
		{"Blade Runner 2049 (2017)", "Blade Runner 2049", 2017},
		{"2001 A Space Odyssey (1968)", "2001 A Space Odyssey", 1968},
		{"1917.2019.2160p.UHD.BluRay.x265.HDR.Atmos", "1917", 2019},
		{"Inception.2010.HUN.ENG.MULTI.WEB-DL.1080p", "Inception", 2010},
		{"Mad_Max_Fury_Road_2015_HEVC", "Mad Max Fury Road", 2015},
		{"Amelie", "Amelie", 0},
		{"Kids.Cartoon.2018", "Kids Cartoon", 2018},
		{"Up.1080p.BluRay", "Up", 0},
	}
	for _, c := range cases {
		title, year := CleanName(c.in)
		if title != c.title || year != c.year {
			t.Errorf("CleanName(%q) = %q,%d; want %q,%d", c.in, title, year, c.title, c.year)
		}
	}
}

func TestParseMovies(t *testing.T) {
	cases := []struct {
		rel, kind string
		title     string
		year      int
	}{
		{"The.Matrix.1999.1080p.BluRay.x264.mkv", "movies", "The Matrix", 1999},
		{"Blade Runner 2049 (2017).mkv", "movies", "Blade Runner 2049", 2017},
		{"2001 A Space Odyssey (1968)/2001.A.Space.Odyssey.1968.mkv", "movies", "2001 A Space Odyssey", 1968},
		{"Space Drama (2015)/Space.Drama.2015.1080p.BluRay.x265.mkv", "movies", "Space Drama", 2015},
		{"Big Test Movie (2019)/Big Test Movie (2019).mp4", "movies", "Big Test Movie", 2019},
		{"Kids.Cartoon.2018.mp4", "movies", "Kids Cartoon", 2018},
		{"Amélie (2001)/movie.mkv", "movies", "Amélie", 2001},
		// a film in a mixed library must not look like an episode
		{"The.Matrix.1999.1080p.BluRay.x264.mkv", "mixed", "The Matrix", 1999},
		{"Movies/Se7en.1995.1920x1080.mkv", "mixed", "Se7en", 1995},
		// SxxEyy in a movies library is still a movie
		{"Weird.S01E01.Named.Movie.2004.mkv", "movies", "Weird S01E01 Named Movie", 2004},
	}
	for _, c := range cases {
		p := ParseFile(c.rel, c.kind)
		if p.Kind != "movie" || p.Title != c.title || p.Year != c.year {
			t.Errorf("ParseFile(%q) = %+v; want movie %q (%d)", c.rel, p, c.title, c.year)
		}
	}
}

func TestParseEpisodes(t *testing.T) {
	cases := []struct {
		rel        string
		series     string
		seriesYear int
		key        string
		season, ep int
		epEnd      int
		title      string
	}{
		{"Breaking Bad (2008)/Season 01/Breaking.Bad.S01E02.Cats.in.the.Bag.720p.mkv", "Breaking Bad", 2008, "series:breaking bad (2008)", 1, 2, 0, "Cats in the Bag"},
		{"The Office/S02E05.mkv", "The Office", 0, "series:the office", 2, 5, 0, ""},
		{"Seinfeld.S03E10.The.Old.Man.mkv", "Seinfeld", 0, "flat:seinfeld", 3, 10, 0, "The Old Man"},
		{"Dark/Season 2/03 - Ghosts.mkv", "Dark", 0, "series:dark", 2, 3, 0, "Ghosts"},
		{"Futurama/Specials/Futurama.S00E01.mkv", "Futurama", 0, "series:futurama", 0, 1, 0, ""},
		{"Friends/1x05.mkv", "Friends", 0, "series:friends", 1, 5, 0, ""},
		{"Aranyélet/2. évad/Aranyelet.S02E04.mkv", "Aranyélet", 0, "series:aranyélet", 2, 4, 0, ""},
		{"Lost/S01/Lost.S01E01E02.Pilot.mkv", "Lost", 0, "series:lost", 1, 1, 2, "Pilot"},
		{"Lost/Season 1/Lost.S01E03-E04.mkv", "Lost", 0, "series:lost", 1, 3, 4, ""},
		{"Dark/Staffel 2/Dark.S02E01.mkv", "Dark", 0, "series:dark", 2, 1, 0, ""},
		{"Test Show (2020)/Season 01/Test.Show.S01E01.Pilot.Part.1.mkv", "Test Show", 2020, "series:test show (2020)", 1, 1, 0, "Pilot Part 1"},
		{"Other.Series/Other.Series.S01E01.mp4", "Other Series", 0, "series:other.series", 1, 1, 0, ""},
		{"Doctor Who (2005)/Évad 3/Doctor.Who.S03E07.1080p.WEB-DL.mkv", "Doctor Who", 2005, "series:doctor who (2005)", 3, 7, 0, ""},
	}
	for _, c := range cases {
		p := ParseFile(c.rel, "mixed")
		if p.Kind != "episode" || p.Series != c.series || p.SeriesYear != c.seriesYear || p.SeriesKey != c.key ||
			p.Season != c.season || p.Episode != c.ep || p.EpisodeEnd != c.epEnd || p.Title != c.title {
			t.Errorf("ParseFile(%q) =\n  %+v\nwant series=%q(%d) key=%q s%d e%d-%d title=%q",
				c.rel, p, c.series, c.seriesYear, c.key, c.season, c.ep, c.epEnd, c.title)
		}
	}
}

func TestShowsLibraryFallback(t *testing.T) {
	p := ParseFile("Nature Docs/Some Random Name.mkv", "shows")
	if p.Kind != "episode" || p.Series != "Nature Docs" || p.SeriesKey != "series:nature docs" || p.Title != "Some Random Name" {
		t.Fatalf("got %+v", p)
	}
}

func TestSeasonDirs(t *testing.T) {
	for in, want := range map[string]int{"Season 01": 1, "S1": 1, "Staffel 2": 2, "Évad 2": 2, "2. évad": 2, "Specials": 0, "Extras": 0, "season 10": 10} {
		n, ok := SeasonFromDir(in)
		if !ok || n != want {
			t.Errorf("SeasonFromDir(%q) = %d,%v want %d", in, n, ok, want)
		}
	}
	for _, in := range []string{"Breaking Bad", "Movies", "2019"} {
		if _, ok := SeasonFromDir(in); ok {
			t.Errorf("SeasonFromDir(%q) should be false", in)
		}
	}
}

func TestSortTitle(t *testing.T) {
	for in, want := range map[string]string{"The Matrix": "matrix", "A Beautiful Mind": "beautiful mind", "Az ember": "ember", "Egy nap": "nap", "Up": "up"} {
		if got := SortTitle(in); got != want {
			t.Errorf("SortTitle(%q)=%q want %q", in, got, want)
		}
	}
}

func TestNormalizeLang(t *testing.T) {
	for in, want := range map[string]string{"hu": "hun", "HUN": "hun", "Hungarian": "hun", "magyar": "hun", "en": "eng", "angol": "eng", "deu": "ger", "und": "", "forced": ""} {
		if got := NormalizeLang(in); got != want {
			t.Errorf("NormalizeLang(%q)=%q want %q", in, got, want)
		}
	}
}

func TestImdbFromName(t *testing.T) {
	for in, want := range map[string]string{
		"The Matrix (1999) {imdb-tt0133093}.mkv": "tt0133093",
		"The Matrix (1999) [imdbid-tt0133093]":   "tt0133093",
		"Matrix.tt0133093.1080p.mkv":             "tt0133093",
		"The Matrix (1999).mkv":                  "",
	} {
		if got := ImdbFromName(in); got != want {
			t.Errorf("ImdbFromName(%q) = %q, want %q", in, got, want)
		}
	}
}

// Episodes downloaded one by one land in a folder per release; they must still form one show.
func TestParseReleaseFolders(t *testing.T) {
	cases := []struct {
		rel, series, key string
		season, episode  int
	}{
		{"Line.of.Fire.2026.S01E03.1080p.WEB.h264-GRP/Line.of.Fire.2026.S01E03.1080p.WEB.h264-GRP.mkv", "Line of Fire", "flat:lineoffire", 1, 3},
		{"Line.of.Fire.S01E04.720p.HDTV/Line.of.Fire.S01E04.720p.HDTV.mkv", "Line of Fire", "flat:lineoffire", 1, 4},
		{"Line.of.Fire.S01E05.1080p.WEB/a8f3c1.mkv", "Line of Fire", "flat:lineoffire", 1, 5},
		{"Adventure.Time.S08E13.Reboot.1080p/Adventure.Time.S08E13.Reboot.1080p.mkv", "Adventure Time", "flat:adventuretime", 8, 13},
		{"Sex.and.the.City.S02.1080p.BluRay/Sex.and.the.City.S02E05.1080p.mkv", "Sex and the City", "flat:sexandthecity", 2, 5},
		{"Sex and the City S03 Complete/07 - Name.mkv", "Sex and the City", "flat:sexandthecity", 3, 7},
		// the show's own folder above the release folder is kept
		{"Line of Fire/Line.of.Fire.S01E06.1080p/Line.of.Fire.S01E06.1080p.mkv", "Line of Fire", "series:line of fire", 1, 6},
		// a classic layout is unchanged
		{"Dark (2017)/Season 2/Dark.S02E03.mkv", "Dark", "series:dark (2017)", 2, 3},
	}
	for _, c := range cases {
		p := ParseFile(c.rel, "shows")
		if p.Kind != "episode" || p.Series != c.series || p.SeriesKey != c.key || p.Season != c.season || p.Episode != c.episode {
			t.Errorf("ParseFile(%q) = %q %q S%dE%d; want %q %q S%dE%d", c.rel, p.Series, p.SeriesKey, p.Season, p.Episode, c.series, c.key, c.season, c.episode)
		}
	}
	if p, ok := EpisodeInMoviesLibrary("Line.of.Fire.S01E05.1080p.WEB/a8f3c1.mkv"); !ok || p.SeriesKey != "flat:lineoffire" {
		t.Errorf("movies library: %+v %v", p, ok)
	}
}
