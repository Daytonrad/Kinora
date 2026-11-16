package store

import (
	"strconv"
	"strings"
)

// RatingLevel maps a content rating ("PG-13", "US:R", "HU:16", "FSK 12",
// "Rated TV-MA", "KN"…) to Kinora's 0..4 scale. ok=false means unknown.
//
//	0 = everyone   1 = PG / 6–7   2 = PG-13 / 12–14   3 = R / 15–16   4 = NC-17 / 18
func RatingLevel(s string) (level int, ok bool) {
	r := strings.ToUpper(strings.TrimSpace(s))
	if r == "" {
		return 0, false
	}
	r = strings.TrimPrefix(r, "RATED ")
	if i := strings.LastIndex(r, ":"); i >= 0 {
		r = strings.TrimSpace(r[i+1:])
	}
	r = strings.TrimPrefix(r, "FSK")
	r = strings.TrimPrefix(r, "AB ")
	r = strings.TrimSpace(strings.TrimSuffix(r, "+"))
	switch r {
	case "G", "TV-Y", "TV-G", "KN", "U", "ALL", "L", "A", "0", "E", "KORHATÁR NÉLKÜL", "MINDENKI":
		return 0, true
	case "PG", "TV-PG", "TV-Y7", "TV-Y7-FV", "6", "7", "PG-8", "8", "9", "10":
		return 1, true
	case "PG-13", "TV-14", "12", "12A", "13", "14", "M", "11":
		return 2, true
	case "R", "TV-MA", "15", "16", "17", "MA15", "MA", "R16":
		return 3, true
	case "NC-17", "X", "XXX", "18", "R18", "R-18", "21":
		return 4, true
	case "NR", "NOT RATED", "UNRATED", "N/A", "":
		return 0, false
	}
	if n, err := strconv.Atoi(r); err == nil {
		switch {
		case n <= 0:
			return 0, true
		case n <= 10:
			return 1, true
		case n <= 14:
			return 2, true
		case n <= 17:
			return 3, true
		default:
			return 4, true
		}
	}
	return 0, false
}
