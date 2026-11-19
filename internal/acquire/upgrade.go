package acquire

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ScotyLucas/kinora/internal/store"
)

// Quality profiles (several, one default) and upgrades: a film Kinora downloaded below the profile's cutoff
// is searched again now and then; a better release replaces it, and the old file is removed once the new one
// is in the library.

// upgradesPerRound is how many films are checked for a better release on one round.
const upgradesPerRound = 3

// QualityRank orders qualities: resolution first, then the source (Remux > BluRay > WEB-DL > WEBRip > HDTV > DVD).
func QualityRank(r Release) int { return r.Resolution*100 + sourceScore[r.Source] }

// CutoffRank is the quality from which a film is not upgraded any more.
func (p Profile) CutoffRank() int { return p.CutoffRes*100 + sourceScore[p.CutoffSource] }

// ProfileFor returns a stored profile, or the default (the one marked default, else the settings' profile).
func (m *Manager) ProfileFor(id int64) Profile {
	if id > 0 {
		if p, err := m.st.AcqProfileByID(id); err == nil {
			return ParseProfile(p.JSON)
		}
	}
	if p, err := m.st.DefaultAcqProfile(); err == nil {
		return ParseProfile(p.JSON)
	}
	return ParseProfile(m.st.Setting("acq_profile"))
}

// tvProfileFor is the profile of a followed series, sized for one episode.
func (m *Manager) tvProfileFor(x *store.AcqSeries) Profile {
	p := m.ProfileFor(m.st.SeriesProfileID(x.ID))
	p.MinSizeGB /= 4
	if p.MaxSizeGB > 0 {
		p.MaxSizeGB /= 2
	}
	return p
}

// ProfileJSON serializes a profile for storing.
func ProfileJSON(p Profile) string {
	b, _ := json.Marshal(p)
	return string(b)
}

// ProcessUpgrades looks for better releases of films below their profile's cutoff.
func (m *Manager) ProcessUpgrades(ctx context.Context) {
	if !m.Enabled() {
		return
	}
	cands, err := m.st.UpgradeCandidates(time.Now().Add(-m.retryWait()).Unix(), upgradesPerRound)
	if err != nil {
		return
	}
	for _, g := range cands {
		if ctx.Err() != nil {
			return
		}
		m.st.MarkUpgradeChecked(g.ID)
		pid := int64(0)
		if g.ProfileID != nil {
			pid = *g.ProfileID
		}
		p := m.ProfileFor(pid)
		cur := QualityRank(ParseRelease(g.ReleaseTitle))
		if !p.Upgrade || g.LibraryID == nil || cur >= p.CutoffRank() {
			continue
		}
		rs, err := m.releasesWith(ctx, g.TmdbID, p)
		if err != nil {
			continue
		}
		var better *Result
		for _, r := range rs.Results {
			if r.Rejected == "" && QualityRank(r.Release) > cur {
				if b := m.bestRelease([]Result{r}); b != nil {
					better = b
					break
				}
			}
		}
		if better == nil {
			continue
		}
		m.log.Info("acquire: upgrade found", "title", g.Title, "from", g.ReleaseTitle, "to", better.Title)
		if _, err := m.Grab(ctx, GrabInput{TmdbID: g.TmdbID, LibraryID: *g.LibraryID, Release: better, ProfileID: pid, Replaces: g.TargetPath}); err != nil {
			m.log.Warn("acquire: upgrade grab", "title", g.Title, "err", err)
		}
	}
}

// removeReplaced deletes the file an upgrade replaced, with its subtitles – only inside the library root.
func removeReplaced(libRoot, old, kept string) {
	root, err := filepath.Abs(libRoot)
	if err != nil || old == "" || old == kept {
		return
	}
	abs, err := filepath.Abs(old)
	if err != nil {
		return
	}
	if rel, err := filepath.Rel(root, abs); err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return
	}
	if fi, err := os.Lstat(abs); err != nil || !fi.Mode().IsRegular() {
		return
	}
	os.Remove(abs)
	base := strings.TrimSuffix(filepath.Base(abs), filepath.Ext(abs)) + "."
	ents, _ := os.ReadDir(filepath.Dir(abs))
	for _, e := range ents {
		if !e.IsDir() && strings.HasPrefix(e.Name(), base) && subExts[strings.ToLower(filepath.Ext(e.Name()))] {
			os.Remove(filepath.Join(filepath.Dir(abs), e.Name()))
		}
	}
}
