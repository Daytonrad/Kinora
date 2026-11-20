package server

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/ScotyLucas/kinora/internal/store"
)

// The audio and subtitle language of one film or series, for the current profile. Any item of the title can
// be named: an episode or a season stands for its series, so a choice made while watching carries to the
// next episodes. The playback answer applies it (audio_idx, sub_idx).

type titleLangsOut struct {
	store.TitleLangs
	Audio []string `json:"audio"` // the languages the title's files have
	Subs  []string `json:"subs"`
}

var reLangCode = regexp.MustCompile(`^[a-z]{2,3}(-[a-z0-9]{2,8})?$`)

func (s *Server) titleItem(w http.ResponseWriter, r *http.Request, c *reqCtx) (*store.Item, bool) {
	it, err := s.st.ItemFull(pathID(r, "id"), c.Profile)
	if err != nil {
		writeErr(w, http.StatusNotFound, "az elem nem található")
		return nil, false
	}
	return it, true
}

func (s *Server) titleLangsOut(c *reqCtx, title int64) titleLangsOut {
	out := titleLangsOut{TitleLangs: s.st.TitleLangsFor(c.Profile.ID, title)}
	out.Audio, out.Subs, _ = s.st.TitleLanguages(title)
	return out
}

func (s *Server) handleTitleLangs(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	it, ok := s.titleItem(w, r, c)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.titleLangsOut(c, store.TitleOf(it)))
}

// handleTitleLangsPut takes {"audio_lang": "hun", "sub_lang": "off"}; a missing field keeps its value, ""
// gives it back to the profile's settings.
func (s *Server) handleTitleLangsPut(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	it, ok := s.titleItem(w, r, c)
	if !ok {
		return
	}
	var in struct {
		AudioLang *string `json:"audio_lang"`
		SubLang   *string `json:"sub_lang"`
	}
	if !decode(w, r, &in) {
		return
	}
	title := store.TitleOf(it)
	t := s.st.TitleLangsFor(c.Profile.ID, title)
	for _, f := range []struct {
		in  *string
		dst *string
		off bool
	}{{in.AudioLang, &t.AudioLang, false}, {in.SubLang, &t.SubLang, true}} {
		if f.in == nil {
			continue
		}
		v := strings.ToLower(strings.TrimSpace(*f.in))
		if v != "" && !reLangCode.MatchString(v) && !(f.off && v == store.SubOff) {
			writeErr(w, http.StatusBadRequest, "érvénytelen nyelvkód")
			return
		}
		*f.dst = v
	}
	if err := s.st.SetTitleLangs(c.Profile.ID, t); err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.titleLangsOut(c, title))
}
