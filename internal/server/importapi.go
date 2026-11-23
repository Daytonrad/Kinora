package server

import (
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ScotyLucas/kinora/internal/jfimport"
	"github.com/ScotyLucas/kinora/internal/store"
)

// Import previews live in memory until applied or expired: they can be large, and a restart only means
// running the preview again (it writes nothing).
const (
	importTTL     = 2 * time.Hour
	importMaxKept = 4
)

type importRun struct {
	plan    *jfimport.Plan
	report  *jfimport.Report
	created time.Time
}

type importRuns struct {
	mu   sync.Mutex
	runs map[string]*importRun
}

func (ir *importRuns) put(p *jfimport.Plan) {
	ir.mu.Lock()
	defer ir.mu.Unlock()
	if ir.runs == nil {
		ir.runs = map[string]*importRun{}
	}
	var oldest string
	for id, r := range ir.runs {
		if time.Since(r.created) > importTTL {
			delete(ir.runs, id)
		} else if oldest == "" || r.created.Before(ir.runs[oldest].created) {
			oldest = id
		}
	}
	if len(ir.runs) >= importMaxKept && oldest != "" {
		delete(ir.runs, oldest)
	}
	ir.runs[p.ID] = &importRun{plan: p, created: time.Now()}
}

func (ir *importRuns) get(id string) *importRun {
	ir.mu.Lock()
	defer ir.mu.Unlock()
	r := ir.runs[id]
	if r == nil || time.Since(r.created) > importTTL {
		return nil
	}
	return r
}

// importErr answers a failed Jellyfin call with a code the web client translates, and a Hungarian text.
func importErr(w http.ResponseWriter, err error) {
	code, status, msg := "unreachable", http.StatusBadGateway, "a Jellyfin szerver nem érhető el"
	switch {
	case errors.Is(err, jfimport.ErrBadURL):
		code, status, msg = "bad_url", http.StatusBadRequest, "érvénytelen cím (http:// vagy https://)"
	case errors.Is(err, jfimport.ErrNoKey):
		code, status, msg = "no_key", http.StatusBadRequest, "add meg a Jellyfin API-kulcsot"
	case errors.Is(err, jfimport.ErrUnauthorized):
		code, msg = "unauthorized", "a Jellyfin elutasította a kulcsot (adminisztrátori API-kulcs kell)"
	case errors.Is(err, jfimport.ErrNotJellyfin):
		code, msg = "not_jellyfin", "a cím nem Jellyfin szerverre mutat"
	}
	writeJSON(w, status, map[string]string{"error": msg, "code": code, "detail": err.Error()})
}

// POST /api/admin/import/jellyfin/preview {url, api_key, path_maps:[{from,to}]} – reads Jellyfin, writes nothing.
func (s *Server) handleJFPreview(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in struct {
		URL      string             `json:"url"`
		APIKey   string             `json:"api_key"`
		PathMaps []jfimport.PathMap `json:"path_maps"`
	}
	if !decode(w, r, &in) {
		return
	}
	cl, err := jfimport.NewClient(in.URL, in.APIKey)
	if err != nil {
		importErr(w, err)
		return
	}
	var maps []jfimport.PathMap
	for _, m := range in.PathMaps {
		if strings.TrimSpace(m.From) != "" && strings.TrimSpace(m.To) != "" {
			maps = append(maps, jfimport.PathMap{From: strings.TrimSpace(m.From), To: strings.TrimSpace(m.To)})
		}
	}
	plan, err := jfimport.Preview(r.Context(), cl, s.st, maps)
	if err != nil {
		s.log.Warn("jellyfin import preview", "url", cl.BaseURL, "err", err)
		importErr(w, err)
		return
	}
	s.imports.put(plan)
	s.log.Info("jellyfin import preview", "server", plan.ServerName, "users", len(plan.Users), "matched", plan.Matched, "unmatched", plan.Unmatched)
	writeJSON(w, http.StatusOK, plan)
}

// POST /api/admin/import/jellyfin/apply {plan_id, users:[jellyfin ids]} – carries out a preview.
func (s *Server) handleJFApply(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in struct {
		PlanID string   `json:"plan_id"`
		Users  []string `json:"users"`
	}
	if !decode(w, r, &in) {
		return
	}
	run := s.imports.get(in.PlanID)
	if run == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "az előnézet lejárt, készítsd el újra", "code": "plan_expired"})
		return
	}
	rep, err := jfimport.Apply(s.st, run.plan, in.Users)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.imports.mu.Lock()
	run.report = rep
	s.imports.mu.Unlock()
	s.log.Info("jellyfin import applied", "server", run.plan.ServerName, "users", len(rep.Users), "changed", rep.Changed)
	writeJSON(w, http.StatusOK, rep)
}

// GET /api/admin/import/jellyfin/{id}/export.csv – every entry of a preview with its outcome.
func (s *Server) handleJFExport(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	run := s.imports.get(r.PathValue("id"))
	if run == nil {
		writeErr(w, http.StatusNotFound, "az előnézet lejárt, készítsd el újra")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", contentDisposition("attachment", "kinora-jellyfin-import-"+run.plan.ID+".csv"))
	w.Write([]byte{0xEF, 0xBB, 0xBF}) // Excel reads UTF-8 only with the byte order mark
	cw := csv.NewWriter(w)
	cw.Write([]string{"user", "kinora_user", "kind", "title", "series", "season", "episode", "year", "jellyfin_path",
		"status", "method_or_reason", "kinora_item_id", "kinora_title", "played", "play_count", "position_sec", "favorite", "last_played"})
	num := func(p *int) string {
		if p == nil {
			return ""
		}
		return strconv.Itoa(*p)
	}
	for _, u := range run.plan.Users {
		for _, e := range u.Entries {
			status, why, id, title := "unmatched", e.Reason, "", ""
			if e.Match != nil {
				status, why, id, title = "matched", e.Method, strconv.FormatInt(e.Match.ID, 10), e.Match.Title
			}
			last := ""
			if e.LastPlayed > 0 {
				last = time.Unix(e.LastPlayed, 0).UTC().Format(time.RFC3339)
			}
			cw.Write([]string{u.Name, u.KinoraName, e.Kind, csvSafe(e.Title), csvSafe(e.Series), num(e.Season), num(e.Episode), yearStr(e.Year),
				csvSafe(e.Path), status, why, id, csvSafe(title), strconv.FormatBool(e.Played), strconv.Itoa(e.PlayCount),
				fmt.Sprintf("%.0f", e.Position), strconv.FormatBool(e.Favorite), last})
		}
	}
	cw.Flush()
}

func yearStr(y int) string {
	if y == 0 {
		return ""
	}
	return strconv.Itoa(y)
}

// csvSafe keeps a spreadsheet from running a title as a formula ("=HYPERLINK(…)").
func csvSafe(v string) string {
	if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
		return "'" + v
	}
	return v
}

// ------------------------------------------------------------------ password links

// POST /api/admin/users/{id}/password-link – a new one-time link that sets the account's password.
func (s *Server) handlePasswordLinkCreate(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	u, err := s.st.UserByID(pathID(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "a felhasználó nem található")
		return
	}
	code, exp, err := s.st.CreatePasswordLink(u.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": code, "expires_at": exp})
}

// GET /api/password-links/{code} – who the link is for (the page greets them by name).
func (s *Server) handlePasswordLinkInfo(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	if !s.limiter.allow("pwlink:"+clientIP(r), 30, 10*time.Minute) {
		writeErr(w, http.StatusTooManyRequests, "túl sok próbálkozás, próbáld újra később")
		return
	}
	u, err := s.st.PasswordLinkUser(r.PathValue("code"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"username": u.Username, "server_name": s.st.Setting("server_name")})
}

// POST /api/password-links/{code} {password} – sets the password, uses up the link and signs in.
func (s *Server) handlePasswordLinkUse(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in credentials
	if !decode(w, r, &in) {
		return
	}
	if !s.limiter.allow("pwlink:"+clientIP(r), 30, 10*time.Minute) {
		writeErr(w, http.StatusTooManyRequests, "túl sok próbálkozás, próbáld újra később")
		return
	}
	if msg := validCredentials("ok", in.Password); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	u, err := s.st.UsePasswordLink(r.PathValue("code"), in.Password)
	if errors.Is(err, store.ErrLinkInvalid) {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	} else if err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.log.Info("password set with a link", "user", u.Username)
	s.startSession(w, r, u, in.DeviceInfo)
}
