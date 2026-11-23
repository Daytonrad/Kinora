package server

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ScotyLucas/kinora/internal/arrimport"
	"github.com/ScotyLucas/kinora/internal/safe"
)

// Sonarr / Radarr / Prowlarr import previews, kept in memory like the Jellyfin ones.
type arrRun struct {
	plan     *arrimport.Plan
	progress *arrimport.Progress
	created  time.Time
}

type arrRuns struct {
	mu   sync.Mutex
	runs map[string]*arrRun
}

func (ar *arrRuns) put(p *arrimport.Plan) {
	ar.mu.Lock()
	defer ar.mu.Unlock()
	if ar.runs == nil {
		ar.runs = map[string]*arrRun{}
	}
	for id, r := range ar.runs {
		if time.Since(r.created) > importTTL && (r.progress == nil || !r.progress.Snapshot().Running) {
			delete(ar.runs, id)
		}
	}
	ar.runs[p.ID] = &arrRun{plan: p, created: time.Now()}
}

func (ar *arrRuns) get(id string) *arrRun {
	ar.mu.Lock()
	defer ar.mu.Unlock()
	return ar.runs[id]
}

func arrErr(w http.ResponseWriter, err error) {
	code, status, msg := "unreachable", http.StatusBadGateway, "a szolgáltatás nem érhető el"
	switch {
	case errors.Is(err, arrimport.ErrBadURL):
		code, status, msg = "bad_url", http.StatusBadRequest, "érvénytelen cím (http:// vagy https://)"
	case errors.Is(err, arrimport.ErrNoKey):
		code, status, msg = "no_key", http.StatusBadRequest, "add meg az API-kulcsot"
	case errors.Is(err, arrimport.ErrUnauthorized):
		code, msg = "unauthorized", "a szolgáltatás elutasította az API-kulcsot"
	case errors.Is(err, arrimport.ErrWrongApp):
		code, msg = "wrong_app", "ezen a címen nem a kiválasztott alkalmazás válaszol"
	}
	writeJSON(w, status, map[string]string{"error": msg, "code": code, "detail": err.Error()})
}

func (s *Server) arrDeps() arrimport.Deps {
	d := arrimport.Deps{Store: s.st, Acquire: s.Acquire, Scan: func(id int64) {
		if s.Scanner != nil {
			s.Scanner.Trigger(s.bgCtx, id)
		}
	}}
	if s.Meta != nil {
		d.TMDB = s.Meta
	}
	return d
}

// POST /api/admin/import/arr/preview {app, url, api_key, path_maps} – reads the app, writes nothing.
func (s *Server) handleArrPreview(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in struct {
		App      string              `json:"app"`
		URL      string              `json:"url"`
		APIKey   string              `json:"api_key"`
		PathMaps []arrimport.PathMap `json:"path_maps"`
	}
	if !decode(w, r, &in) {
		return
	}
	cl, err := arrimport.NewClient(strings.ToLower(in.App), in.URL, in.APIKey)
	if err != nil {
		arrErr(w, err)
		return
	}
	var maps []arrimport.PathMap
	for _, m := range in.PathMaps {
		if strings.TrimSpace(m.From) != "" && strings.TrimSpace(m.To) != "" {
			maps = append(maps, arrimport.PathMap{From: strings.TrimSpace(m.From), To: strings.TrimSpace(m.To)})
		}
	}
	plan, err := arrimport.Preview(r.Context(), cl, s.arrDeps(), maps)
	if err != nil {
		s.log.Warn("arr import preview", "app", cl.App, "url", cl.BaseURL, "err", err)
		arrErr(w, err)
		return
	}
	s.arrImports.put(plan)
	writeJSON(w, http.StatusOK, plan)
}

// POST /api/admin/import/arr/apply {plan_id, sections} – starts the import in the background.
func (s *Server) handleArrApply(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in struct {
		PlanID   string   `json:"plan_id"`
		Sections []string `json:"sections"`
	}
	if !decode(w, r, &in) {
		return
	}
	run := s.arrImports.get(in.PlanID)
	if run == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "az előnézet lejárt, készítsd el újra", "code": "plan_expired"})
		return
	}
	s.arrImports.mu.Lock()
	if run.progress != nil && run.progress.Snapshot().Running {
		s.arrImports.mu.Unlock()
		writeErr(w, http.StatusConflict, "ez az importálás már fut")
		return
	}
	pr := arrimport.NewProgress(run.plan, in.Sections)
	run.progress = pr
	s.arrImports.mu.Unlock()
	go func() {
		defer safe.Recover(s.log, "arr-import")
		arrimport.Apply(s.bgCtx, run.plan, s.arrDeps(), in.Sections, pr)
		v := pr.Snapshot()
		s.log.Info("arr import done", "app", run.plan.App, "done", v.Done, "errors", len(v.Errors))
	}()
	writeJSON(w, http.StatusAccepted, pr.Snapshot())
}

// GET /api/admin/import/arr/{id} – the progress of an import.
func (s *Server) handleArrStatus(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	run := s.arrImports.get(r.PathValue("id"))
	if run == nil {
		writeErr(w, http.StatusNotFound, "az előnézet lejárt, készítsd el újra")
		return
	}
	s.arrImports.mu.Lock()
	pr := run.progress
	s.arrImports.mu.Unlock()
	if pr == nil {
		writeJSON(w, http.StatusOK, map[string]any{"started": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"started": true, "progress": pr.Snapshot()})
}
