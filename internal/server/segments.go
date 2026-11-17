package server

import (
	"context"
	"net/http"
	"time"

	"github.com/ScotyLucas/kinora/internal/store"
)

// effectiveSegments returns an item's skip segments: manual > chapter title > detected.
func (s *Server) effectiveSegments(it *store.Item) []store.Segment {
	stored, _ := s.st.StoredSegments(it.ID)
	return store.EffectiveSegments(stored, it.Chapters, float64(it.RuntimeSec))
}

// GET /api/items/{id}/segments
func (s *Server) handleSegments(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	it, err := s.st.ItemFull(pathID(r, "id"), c.Profile)
	if err != nil {
		writeErr(w, http.StatusNotFound, "az elem nem található")
		return
	}
	stored, _ := s.st.StoredSegments(it.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"segments": store.EffectiveSegments(stored, it.Chapters, float64(it.RuntimeSec)),
		"stored":   stored,
	})
}

// PUT /api/items/{id}/segments/{type} – hand-marked segment (admin). Overrides chapters
// and detection; the response says whether it can be applied to the rest of the season.
func (s *Server) handleSegmentSet(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in struct {
		Start float64 `json:"start"`
		End   float64 `json:"end"`
	}
	if !decode(w, r, &in) {
		return
	}
	it, err := s.st.Item(pathID(r, "id"), nil)
	if err != nil || (it.Kind != "episode" && it.Kind != "movie") {
		writeErr(w, http.StatusNotFound, "az elem nem található")
		return
	}
	if it.RuntimeSec > 0 && in.End > float64(it.RuntimeSec)+1 {
		writeErr(w, http.StatusBadRequest, "a szegmens túlnyúlik a videó végén")
		return
	}
	if err := s.st.SetManualSegment(it.ID, store.Segment{Type: r.PathValue("type"), Start: in.Start, End: in.End}); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "can_propagate": it.Kind == "episode" && s.Intro != nil && s.Intro.Status().Available})
}

// DELETE /api/items/{id}/segments/{type} – drop the manual mark (chapters/detection apply again).
func (s *Server) handleSegmentDelete(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	if err := s.st.DeleteSegment(pathID(r, "id"), r.PathValue("type"), "manual"); err != nil {
		s.internalErr(w, r, err)
		return
	}
	ok(w)
}

// POST /api/items/{id}/segments/{type}/propagate – apply a manual mark to the season's
// other episodes where the same fingerprint stretch is found.
func (s *Server) handleSegmentPropagate(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	if s.Intro == nil {
		writeErr(w, http.StatusServiceUnavailable, "az intró-felismerés nem érhető el")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	n, err := s.Intro.Propagate(ctx, pathID(r, "id"), r.PathValue("type"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "updated": n})
}

// GET /api/admin/segments – detection status.
func (s *Server) handleSegmentStatus(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	if s.Intro == nil {
		writeJSON(w, http.StatusOK, map[string]any{"available": false, "backend": "nincs bekapcsolva"})
		return
	}
	writeJSON(w, http.StatusOK, s.Intro.Status())
}

// POST /api/admin/segments/analyze {item_id} – (re)run detection for a series, season or episode.
func (s *Server) handleSegmentAnalyze(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in struct {
		ItemID int64 `json:"item_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	if s.Intro == nil || !s.Intro.Status().Available {
		writeErr(w, http.StatusServiceUnavailable, "az intró-felismerés nem érhető el (ffmpeg chromaprint vagy fpcalc kell)")
		return
	}
	seasons, err := s.st.AnalysisSeasonsOf(in.ItemID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.Intro.Enqueue(seasons, true)
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "seasons": len(seasons)})
}
