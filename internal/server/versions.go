package server

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ScotyLucas/kinora/internal/store"
)

// versionInfo is one file of a film with several versions, as the detail page lists it.
type versionInfo struct {
	ID         int64   `json:"id"`
	Edition    string  `json:"edition,omitempty"` // "Director's Cut", from the file name; "" for the plain version
	Resolution string  `json:"resolution"`        // 4K, 1080p, 720p, SD
	HDR        string  `json:"hdr,omitempty"`
	VideoCodec string  `json:"video_codec,omitempty"`
	AudioCodec string  `json:"audio_codec,omitempty"`
	Channels   int     `json:"channels,omitempty"`
	Size       int64   `json:"size"`
	Bitrate    int     `json:"bitrate,omitempty"`
	RuntimeSec int     `json:"runtime_sec,omitempty"`
	Container  string  `json:"container,omitempty"`
	Library    string  `json:"library,omitempty"`
	Position   float64 `json:"position,omitempty"`
}

var (
	// {edition-Director's Cut} (Plex / Jellyfin naming)
	reEditionTag = regexp.MustCompile(`(?i)\{edition-([^}]+)\}`)
	editionWords = []struct {
		re    *regexp.Regexp
		label string
	}{
		{regexp.MustCompile(`(?i)director'?s[ ._-]*cut|rendez[oő]i[ ._-]*v[aá]ltozat`), "Director's Cut"},
		{regexp.MustCompile(`(?i)extended[ ._-]*(?:cut|edition)?|b[oő]v[ií]tett`), "Extended"},
		{regexp.MustCompile(`(?i)theatrical[ ._-]*(?:cut|edition)?|mozi[ ._-]*v[aá]ltozat`), "Theatrical"},
		{regexp.MustCompile(`(?i)\bunrated\b|v[aá]gatlan`), "Unrated"},
		{regexp.MustCompile(`(?i)\buncut\b`), "Uncut"},
		{regexp.MustCompile(`(?i)\bimax\b`), "IMAX"},
		{regexp.MustCompile(`(?i)\bremastered\b|feljav[ií]tott`), "Remastered"},
		{regexp.MustCompile(`(?i)ultimate[ ._-]*(?:cut|edition)`), "Ultimate Cut"},
		{regexp.MustCompile(`(?i)final[ ._-]*cut`), "Final Cut"},
		{regexp.MustCompile(`(?i)special[ ._-]*edition`), "Special Edition"},
		{regexp.MustCompile(`(?i)criterion`), "Criterion"},
	}
	reResolution = regexp.MustCompile(`(?i)^(?:2160p|1080p|720p|480p|4k|uhd|hdr\d*|sdr|dv|hevc|x26[45]|h\.?26[45]|av1|remux|bluray|web-?dl|webrip)$`)
)

// editionOf names the cut of a film from its file name: an {edition-…} tag, a known
// word (Director's Cut, Extended…), or the free text after " - " in "Film (2010) - Label.mkv"
// when that text is not just a resolution / source note.
func editionOf(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if m := reEditionTag.FindStringSubmatch(base); m != nil {
		return strings.TrimSpace(m[1])
	}
	for _, e := range editionWords {
		if e.re.MatchString(base) {
			return e.label
		}
	}
	if i := strings.LastIndex(base, " - "); i > 0 {
		label := strings.TrimSpace(base[i+3:])
		label = strings.Trim(label, "[]() ")
		allTech := label != ""
		for _, w := range strings.FieldsFunc(label, func(r rune) bool { return r == ' ' || r == '.' || r == '_' }) {
			if !reResolution.MatchString(w) {
				allTech = false
				break
			}
		}
		if label != "" && !allTech && len([]rune(label)) <= 40 && !strings.ContainsAny(label, "{}") {
			return label
		}
	}
	return ""
}

func resolutionOf(w, h int) string {
	switch {
	case w >= 3200 || h >= 1800:
		return "4K"
	case w >= 1800 || h >= 1000:
		return "1080p"
	case w >= 1200 || h >= 700:
		return "720p"
	case h > 0:
		return "SD"
	}
	return ""
}

// versionsOf describes the versions of a movie for the detail page.
func (s *Server) versionsOf(p *store.Profile, it *store.Item) []versionInfo {
	vs, err := s.st.Versions(p, it)
	if err != nil || len(vs) < 2 {
		return nil
	}
	libs := map[int64]string{}
	multiLib := false
	for _, v := range vs {
		if v.LibraryID != vs[0].LibraryID {
			multiLib = true
		}
	}
	if multiLib {
		if all, err := s.st.Libraries(); err == nil {
			for _, l := range all {
				libs[l.ID] = l.Name
			}
		}
	}
	out := make([]versionInfo, 0, len(vs))
	for _, v := range vs {
		vi := versionInfo{ID: v.ID, Edition: editionOf(v.Path), Resolution: resolutionOf(v.Width, v.Height), HDR: v.HDR,
			VideoCodec: v.VideoCodec, AudioCodec: v.AudioCodec, Size: v.Size, Bitrate: v.Bitrate, RuntimeSec: v.RuntimeSec,
			Container: v.Container, Library: libs[v.LibraryID], Position: v.UserData.Position}
		if sts, err := s.st.Streams(v.ID); err == nil {
			for _, st := range sts {
				if st.Type == "audio" {
					vi.Channels = st.Channels
					break
				}
			}
		}
		out = append(out, vi)
	}
	return out
}
