package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/ScotyLucas/kinora/internal/acquire"
	"github.com/ScotyLucas/kinora/internal/acquire/cardigann"
	"github.com/ScotyLucas/kinora/internal/acquire/sites"
	"github.com/ScotyLucas/kinora/internal/store"
)

// Built-in indexer engine (M5): the admin uploads indexer definitions (Cardigann YAML, the Jackett / Prowlarr
// format) and adds indexers that run them. Kinora ships none: the admin uploads them, or adds them from the
// catalogue (the community collection Prowlarr keeps, fetched on demand).

const maxDefinitionSize = 1 << 20

type definitionInfo struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Type        string              `json:"type"`
	Description string              `json:"description"`
	Language    string              `json:"language"`
	Links       []string            `json:"links"`
	Settings    []cardigann.Setting `json:"settings"`
	InUse       int                 `json:"in_use"`
	CreatedAt   int64               `json:"created_at"`
	Builtin     bool                `json:"builtin"` // written in Go, part of Kinora
}

func definitionSummary(d *store.AcqDefinition) (*definitionInfo, error) {
	def, err := cardigann.Parse([]byte(d.YAML))
	if err != nil {
		return nil, err
	}
	return &definitionInfo{ID: def.ID, Name: def.Name, Type: def.Type, Description: def.Description, Language: def.Language,
		Links: def.Links, Settings: def.Settings, InUse: d.InUse, CreatedAt: d.CreatedAt}, nil
}

func (s *Server) handleAcqDefinitions(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	list, err := s.st.AcqDefinitions()
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	out := []*definitionInfo{}
	ixs, _ := s.st.AcqIndexers()
	for _, id := range sortedBuiltins() {
		def := sites.Builtin[id].Def
		info := &definitionInfo{ID: def.ID, Name: def.Name, Type: def.Type, Description: def.Description, Language: def.Language,
			Links: def.Links, Settings: def.Settings, Builtin: true}
		for _, x := range ixs {
			if x.Kind == "cardigann" && x.DefinitionID == id {
				info.InUse++
			}
		}
		out = append(out, info)
	}
	for _, d := range list {
		if info, err := definitionSummary(d); err == nil {
			out = append(out, info)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleAcqDefinitionUpload takes {"yaml": "..."}; the same id replaces the earlier version.
func (s *Server) handleAcqDefinitionUpload(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in struct {
		YAML string `json:"yaml"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.YAML) > maxDefinitionSize {
		writeErr(w, http.StatusRequestEntityTooLarge, "a leíró legfeljebb 1 MB lehet")
		return
	}
	b := []byte(in.YAML)
	def, err := cardigann.Parse(b)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, ok := sites.Builtin[def.ID]; ok {
		writeErr(w, http.StatusConflict, "ilyen azonosítójú beépített indexer már van a Kinorában")
		return
	}
	d := &store.AcqDefinition{ID: def.ID, Name: def.Name, Type: def.Type, YAML: string(b)}
	if err := s.st.SaveAcqDefinition(d); err != nil {
		s.internalErr(w, r, err)
		return
	}
	info, _ := definitionSummary(d)
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleAcqDefinitionDelete(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	id := r.PathValue("id")
	if _, ok := sites.Builtin[id]; ok {
		writeErr(w, http.StatusConflict, "a beépített indexer nem törölhető")
		return
	}
	list, _ := s.st.AcqDefinitions()
	for _, d := range list {
		if d.ID == id && d.InUse > 0 {
			writeErr(w, http.StatusConflict, "ezt a leírót indexer használja: előbb töröld azt")
			return
		}
	}
	s.st.DeleteAcqDefinition(id)
	ok(w)
}

// definitionOf parses the definition an indexer runs.
func (s *Server) definitionOf(id string) (*cardigann.Definition, error) {
	if b, ok := sites.Builtin[id]; ok {
		return b.Def, nil
	}
	d, err := s.st.AcqDefinition(id)
	if err != nil {
		return nil, errors.New("válassz feltöltött leírót")
	}
	return cardigann.Parse([]byte(d.YAML))
}

// maskConfig hides the secret values (password, cookie, passkey…) of an engine indexer's settings.
func (s *Server) maskConfig(defID, cfg string) string {
	def, err := s.definitionOf(defID)
	if err != nil {
		return "{}"
	}
	values := map[string]any{}
	json.Unmarshal([]byte(cfg), &values)
	for _, st := range def.Settings {
		if v, ok := values[st.Name].(string); ok && st.Secret() {
			values[st.Name] = maskPassword(v)
		}
	}
	b, _ := json.Marshal(values)
	return string(b)
}

// mergeConfig checks the incoming values against the definition and keeps stored secrets sent back masked.
func (s *Server) mergeConfig(x *store.AcqIndexer, stored string) string {
	def, err := s.definitionOf(x.DefinitionID)
	if err != nil {
		return "definition"
	}
	in := map[string]any{}
	if strings.TrimSpace(x.Config) != "" && json.Unmarshal([]byte(x.Config), &in) != nil {
		return "az indexer beállításai érvénytelenek"
	}
	old := map[string]any{}
	json.Unmarshal([]byte(stored), &old)
	out := map[string]any{}
	for _, st := range def.Settings {
		if st.Type == "info" {
			continue
		}
		v, present := in[st.Name]
		if !present {
			continue
		}
		if sv, ok := v.(string); ok && st.Secret() && strings.Contains(sv, maskChar) {
			v = old[st.Name]
		}
		out[st.Name] = v
	}
	b, _ := json.Marshal(out)
	x.Config = string(b)
	if x.Name == "" {
		x.Name = def.Name
	}
	x.URL = def.SiteLink()
	if u, ok := out["url"].(string); ok && strings.TrimSpace(u) != "" {
		x.URL = strings.TrimSpace(u)
	}
	return ""
}

func sortedBuiltins() []string {
	ids := make([]string, 0, len(sites.Builtin))
	for id := range sites.Builtin {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// maskPassword hides a password or cookie entirely (maskSecret keeps the last characters of API keys).
func maskPassword(v string) string {
	if v == "" {
		return ""
	}
	return strings.Repeat(maskChar, 8)
}

// ------------------------------------------------------------------ the catalogue

type catalogItem struct {
	*acquire.CatalogEntry
	Installed bool `json:"installed"` // a definition with this id is saved
	Outdated  bool `json:"outdated"`  // …and the catalogue has a different version
}

func (s *Server) handleAcqCatalog(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	m := s.acq(w)
	if m == nil {
		return
	}
	list, err := m.Catalog(r.Context(), r.URL.Query().Get("refresh") == "1")
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	have := map[string]string{}
	if defs, err := s.st.AcqDefinitions(); err == nil {
		for _, d := range defs {
			have[d.ID] = d.YAML
		}
	}
	out := make([]catalogItem, len(list))
	for i, e := range list {
		y, ok := have[e.ID]
		out[i] = catalogItem{CatalogEntry: e, Installed: ok, Outdated: ok && y != e.YAML}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleAcqCatalogInstall saves a catalogue definition (a newer version replaces the saved one).
func (s *Server) handleAcqCatalogInstall(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	m := s.acq(w)
	if m == nil {
		return
	}
	e, err := m.CatalogEntry(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	d := &store.AcqDefinition{ID: e.ID, Name: e.Name, Type: e.Type, YAML: e.YAML}
	if err := s.st.SaveAcqDefinition(d); err != nil {
		s.internalErr(w, r, err)
		return
	}
	info, _ := definitionSummary(d)
	writeJSON(w, http.StatusOK, info)
}

// handleAcqCatalogUpdate brings the saved definitions that came from the catalogue up to its version.
func (s *Server) handleAcqCatalogUpdate(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	m := s.acq(w)
	if m == nil {
		return
	}
	if _, err := m.Catalog(r.Context(), true); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	defs, _ := s.st.AcqDefinitions()
	n := 0
	for _, d := range defs {
		e, err := m.CatalogEntry(r.Context(), d.ID)
		if err != nil || e.YAML == d.YAML {
			continue
		}
		if s.st.SaveAcqDefinition(&store.AcqDefinition{ID: e.ID, Name: e.Name, Type: e.Type, YAML: e.YAML}) == nil {
			n++
		}
	}
	writeJSON(w, http.StatusOK, map[string]int{"updated": n})
}

func (s *Server) handleAcqFlareSolverrTest(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in struct {
		URL string `json:"url"`
	}
	if !decode(w, r, &in) {
		return
	}
	f := cardigann.NewFlareSolverr(in.URL)
	if f == nil {
		writeErr(w, http.StatusBadRequest, "add meg a FlareSolverr címét")
		return
	}
	v, err := f.Test(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"server": "FlareSolverr " + v})
}
