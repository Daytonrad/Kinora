package server

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/ScotyLucas/kinora/internal/meta"
	"github.com/ScotyLucas/kinora/internal/store"
)

func (s *Server) handleMetadataRefresh(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in struct {
		RetryNoMatch bool `json:"retry_no_match"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !s.Meta.Enabled() {
		writeErr(w, http.StatusBadRequest, meta.ErrNoKey.Error())
		return
	}
	if s.Meta.Status().Running {
		writeErr(w, http.StatusConflict, "a metaadat-frissítés már fut")
		return
	}
	go func() {
		if err := s.Meta.Refresh(s.bgCtx, in.RetryNoMatch); err != nil {
			s.log.Warn("metadata refresh", "err", err)
		}
	}()
	writeJSON(w, http.StatusAccepted, map[string]bool{"started": true})
}

func (s *Server) handleMetadataStatus(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	writeJSON(w, http.StatusOK, map[string]any{"enabled": s.Meta.Enabled(), "status": s.Meta.Status()})
}

func (s *Server) handleMetadataSearch(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	q := r.URL.Query()
	kind := q.Get("kind")
	if kind != "series" {
		kind = "movie"
	}
	year, _ := strconv.Atoi(q.Get("year"))
	res, err := s.Meta.Search(r.Context(), kind, q.Get("q"), year)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleMetadataMatch(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in struct {
		TmdbID int64 `json:"tmdb_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.TmdbID <= 0 {
		writeErr(w, http.StatusBadRequest, "adj meg egy TMDB azonosítót")
		return
	}
	err := s.Meta.Match(r.Context(), pathID(r, "id"), in.TmdbID)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "csak film vagy sorozat párosítható")
		return
	} else if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	ok(w)
}
