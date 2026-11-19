package server

import "testing"

func TestEditionOf(t *testing.T) {
	for in, want := range map[string]string{
		"/m/Blade Runner (1982) {edition-Final Cut}.mkv": "Final Cut",
		"/m/Aliens.1986.Directors.Cut.1080p.mkv":         "Director's Cut",
		"/m/LOTR (2001) - Extended Edition.mkv":          "Extended",
		"/m/Dune (2021) - 2160p HDR.mkv":                 "",
		"/m/Dune (2021) - Fan Edit.mkv":                  "Fan Edit",
		"/m/Dune (2021).mkv":                             "",
		"/m/Amadeus (1984) - rendezői változat.mkv":      "Director's Cut",
	} {
		if got := editionOf(in); got != want {
			t.Errorf("editionOf(%q) = %q, want %q", in, got, want)
		}
	}
}
