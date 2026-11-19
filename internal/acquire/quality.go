package acquire

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// Profile decides which releases are acceptable and which is best.
type Profile struct {
	// Resolutions in order of preference; a release with another resolution is rejected.
	// 0 stands for "unknown" (no resolution in the name).
	Resolutions []int   `json:"resolutions"`
	MinSizeGB   float64 `json:"min_size_gb"`
	MaxSizeGB   float64 `json:"max_size_gb"` // 0 = no limit
	MinSeeders  int     `json:"min_seeders"`
	// PreferHungarian ranks releases with Hungarian audio or subtitles first; RequireHungarian drops the rest.
	PreferHungarian  bool     `json:"prefer_hun"`
	RequireHungarian bool     `json:"require_hun"`
	BlockedSources   []string `json:"blocked_sources"`
	PreferHEVC       bool     `json:"prefer_hevc"`
	// Upgrade: a film below the cutoff (CutoffRes + CutoffSource, e.g. 1080 + bluray) is replaced by a better release.
	Upgrade      bool   `json:"upgrade"`
	CutoffRes    int    `json:"cutoff_resolution"`
	CutoffSource string `json:"cutoff_source"`
}

// DefaultProfile: 1080p first, then 2160p and 720p; no cams; Hungarian preferred.
func DefaultProfile() Profile {
	return Profile{
		Resolutions:     []int{1080, 2160, 720},
		MinSizeGB:       0.3,
		MaxSizeGB:       40,
		MinSeeders:      1,
		PreferHungarian: true,
		BlockedSources:  []string{"cam", "ts", "tc", "scr"},
	}
}

// ParseProfile reads the stored JSON; an empty or broken value gives the default.
func ParseProfile(s string) Profile {
	p := DefaultProfile()
	if strings.TrimSpace(s) == "" {
		return p
	}
	var q Profile
	if err := json.Unmarshal([]byte(s), &q); err != nil {
		return p
	}
	if len(q.Resolutions) == 0 {
		q.Resolutions = p.Resolutions
	}
	if q.BlockedSources == nil {
		q.BlockedSources = p.BlockedSources
	}
	return q
}

// Rejection codes (the web client translates them).
const (
	RejectResolution = "resolution"
	RejectSource     = "source"
	RejectSize       = "size"
	RejectSeeders    = "seeders"
	RejectLanguage   = "language"
	RejectTitle      = "title"
	Reject3D         = "3d"
	// RejectBlocklisted: an earlier download of this release failed
	RejectBlocklisted = "blocklisted"
)

var sourceScore = map[string]int{"remux": 60, "bluray": 50, "webdl": 45, "webrip": 35, "hdtv": 15, "dvd": 5}

// Score rates a release for the profile: higher is better. A non-empty rejection
// means the release is not acceptable at all.
func (p Profile) Score(r Release, sizeBytes int64, seeders int) (score int, rejected string) {
	rank := -1
	for i, res := range p.Resolutions {
		if res == r.Resolution {
			rank = i
			break
		}
	}
	if rank < 0 {
		return 0, RejectResolution
	}
	for _, b := range p.BlockedSources {
		if b == r.Source && r.Source != "" {
			return 0, RejectSource
		}
	}
	if r.ThreeD {
		return 0, Reject3D
	}
	gb := float64(sizeBytes) / (1 << 30)
	if sizeBytes > 0 && (gb < p.MinSizeGB || (p.MaxSizeGB > 0 && gb > p.MaxSizeGB)) {
		return 0, RejectSize
	}
	if seeders >= 0 && seeders < p.MinSeeders {
		return 0, RejectSeeders
	}
	hun := false
	for _, l := range r.Languages {
		if l == "hun" {
			hun = true
		}
	}
	if p.RequireHungarian && !hun {
		return 0, RejectLanguage
	}
	score = (len(p.Resolutions) - rank) * 100
	score += sourceScore[r.Source]
	if hun && (p.PreferHungarian || p.RequireHungarian) {
		score += 1000 // "Hungarian preferred" wins over every other quality step
	}
	if p.PreferHEVC && r.Codec == "hevc" {
		score += 20
	}
	if r.Proper || r.Repack {
		score += 10
	}
	if seeders > 0 { // well-seeded releases finish: up to +50
		score += int(math.Min(50, 12*math.Log2(float64(seeders)+1)))
	}
	return score, ""
}

// TitleMatches checks that a release is the wanted film: its name starts with one of
// the known titles and, when both have one, the years are at most one apart.
func TitleMatches(releaseName string, rel Release, year int, titles ...string) bool {
	name := NormalizeTitle(releaseName)
	for _, t := range titles {
		nt := NormalizeTitle(t)
		if nt == "" || !strings.HasPrefix(name, nt) {
			continue
		}
		if year <= 0 {
			return true
		}
		// the year right after the title ("Blade Runner 2049 2017": the title's own number is not the year)
		rest := name[len(nt):]
		for d := -1; d <= 1; d++ {
			if strings.HasPrefix(rest, strconv.Itoa(year+d)) {
				return true
			}
		}
		if rel.Year > 0 && (rel.Year < year-1 || rel.Year > year+1) {
			continue
		}
		return true
	}
	return false
}
