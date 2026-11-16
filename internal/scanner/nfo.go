package scanner

import (
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/scoty_lucas/kinora/internal/store"
)

type nfoDoc struct {
	Title     string   `xml:"title"`
	Year      string   `xml:"year"`
	Premiered string   `xml:"premiered"`
	Aired     string   `xml:"aired"`
	Plot      string   `xml:"plot"`
	Outline   string   `xml:"outline"`
	Genres    []string `xml:"genre"`
	Tags      []string `xml:"tag"`
	MPAA      string   `xml:"mpaa"`
	Rating    string   `xml:"rating"`
	Ratings   []struct {
		Name    string `xml:"name,attr"`
		Default string `xml:"default,attr"`
		Value   string `xml:"value"`
	} `xml:"ratings>rating"`
	TmdbID    string `xml:"tmdbid"`
	UniqueIDs []struct {
		Type  string `xml:"type,attr"`
		Value string `xml:",chardata"`
	} `xml:"uniqueid"`
}

// ReadNFO parses a Kodi/Jellyfin compatible movie.nfo / tvshow.nfo / <video>.nfo.
// Files that are not XML (e.g. a bare URL) return nil.
func ReadNFO(path string) *store.LocalMeta {
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Contains(data, []byte("<")) {
		return nil
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	dec.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }
	var d nfoDoc
	if err := dec.Decode(&d); err != nil {
		return nil
	}
	m := &store.LocalMeta{
		Title:         strings.TrimSpace(d.Title),
		Overview:      strings.TrimSpace(firstNonEmpty(d.Plot, d.Outline)),
		ContentRating: cleanMPAA(d.MPAA),
	}
	m.Year, _ = strconv.Atoi(strings.TrimSpace(d.Year))
	if m.Year == 0 {
		for _, dt := range []string{d.Premiered, d.Aired} {
			if dt = strings.TrimSpace(dt); len(dt) >= 4 {
				if y, err := strconv.Atoi(dt[:4]); err == nil {
					m.Year = y
					break
				}
			}
		}
	}
	if r, err := strconv.ParseFloat(strings.TrimSpace(strings.ReplaceAll(d.Rating, ",", ".")), 64); err == nil && r > 0 {
		m.Rating = &r
	} else {
		for _, rt := range d.Ratings {
			if v, err := strconv.ParseFloat(strings.TrimSpace(rt.Value), 64); err == nil && v > 0 {
				m.Rating = &v
				if rt.Default == "true" {
					break
				}
			}
		}
	}
	seen := map[string]bool{}
	for _, g := range d.Genres {
		for _, part := range strings.Split(g, "/") {
			part = strings.TrimSpace(part)
			if part != "" && !seen[strings.ToLower(part)] {
				seen[strings.ToLower(part)] = true
				m.Genres = append(m.Genres, part)
			}
		}
	}
	for _, t := range d.Tags {
		if t = strings.TrimSpace(t); t != "" {
			m.Tags = append(m.Tags, t)
		}
	}
	m.TmdbID, _ = strconv.ParseInt(strings.TrimSpace(d.TmdbID), 10, 64)
	if m.TmdbID == 0 {
		for _, u := range d.UniqueIDs {
			if strings.EqualFold(u.Type, "tmdb") {
				m.TmdbID, _ = strconv.ParseInt(strings.TrimSpace(u.Value), 10, 64)
			}
		}
	}
	return m
}

// cleanMPAA turns "Rated PG-13" / "US:PG-13 / HU:16" into a single rating string,
// preferring the Hungarian one when several are listed.
func cleanMPAA(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	parts := strings.Split(s, "/")
	pick := parts[0]
	for _, p := range parts {
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(p)), "HU:") {
			pick = p
		}
	}
	pick = strings.TrimSpace(pick)
	pick = strings.TrimPrefix(pick, "Rated ")
	if i := strings.Index(pick, ":"); i >= 0 && i <= 3 {
		pick = pick[i+1:]
	}
	return strings.TrimSpace(pick)
}

func firstNonEmpty(a ...string) string {
	for _, s := range a {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
