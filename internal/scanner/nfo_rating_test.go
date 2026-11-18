package scanner

import "testing"

func TestParseRatingRejectsNonFinite(t *testing.T) {
	for in, want := range map[string]float64{"7.5": 7.5, " 7,5 ": 7.5, "Inf": 0, "-inf": 0, "NaN": 0, "0": 0, "x": 0} {
		got, ok := parseRating(in)
		if got != want || ok != (want > 0) {
			t.Errorf("parseRating(%q) = %v, %v", in, got, ok)
		}
	}
}
