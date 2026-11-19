package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/ScotyLucas/kinora/internal/acquire"
	"github.com/ScotyLucas/kinora/internal/store"
)

// Admin → Beszerzés: the built-in acquisition module (internal/acquire). Admin only in M1.

func (s *Server) acq(w http.ResponseWriter) *acquire.Manager {
	if s.Acquire == nil {
		writeErr(w, http.StatusServiceUnavailable, "a beszerzés nem érhető el ezen a szerveren")
	}
	return s.Acquire
}

func maskIndexer(x *store.AcqIndexer) *store.AcqIndexer {
	c := *x
	c.APIKey = maskSecret(c.APIKey)
	return &c
}

func maskClient(x *store.AcqClient) *store.AcqClient {
	c := *x
	c.Password = maskSecret(c.Password)
	return &c
}

// keepSecret returns the stored value when the masked one came back from the UI.
func keepSecret(in, stored string) string {
	if strings.Contains(in, maskChar) {
		return stored
	}
	return in
}

func (s *Server) handleAcqSettings(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	ixs, err := s.st.AcqIndexers()
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	cls, err := s.st.AcqClients()
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	outI := make([]*store.AcqIndexer, len(ixs))
	for i, x := range ixs {
		outI[i] = maskIndexer(x)
	}
	outC := make([]*store.AcqClient, len(cls))
	for i, x := range cls {
		outC[i] = maskClient(x)
	}
	libs, _ := s.st.Libraries()
	movieLibs := []*store.Library{}
	for _, l := range libs {
		if l.Kind != "shows" {
			movieLibs = append(movieLibs, l)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":     s.st.Setting("acq_enabled") == "1",
		"profile":     acquire.ParseProfile(s.st.Setting("acq_profile")),
		"import_mode": s.st.Setting("acq_import_mode"),
		"indexers":    outI,
		"clients":     outC,
		"libraries":   movieLibs,
		"tmdb":        strings.TrimSpace(s.st.Setting("tmdb_key")) != "",
	})
}

func (s *Server) handleAcqSettingsPut(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in struct {
		Enabled    *bool            `json:"enabled"`
		Profile    *acquire.Profile `json:"profile"`
		ImportMode *string          `json:"import_mode"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Enabled != nil {
		v := "0"
		if *in.Enabled {
			v = "1"
		}
		s.st.SetSetting("acq_enabled", v)
	}
	if in.Profile != nil {
		p := *in.Profile
		if len(p.Resolutions) == 0 {
			writeErr(w, http.StatusBadRequest, "legalább egy felbontást engedélyezz")
			return
		}
		if p.MinSizeGB < 0 || p.MaxSizeGB < 0 || (p.MaxSizeGB > 0 && p.MaxSizeGB < p.MinSizeGB) || p.MinSeeders < 0 {
			writeErr(w, http.StatusBadRequest, "hibás méret- vagy seederkorlát")
			return
		}
		b, _ := json.Marshal(p)
		s.st.SetSetting("acq_profile", string(b))
	}
	if in.ImportMode != nil {
		if *in.ImportMode != "hardlink" && *in.ImportMode != "copy" {
			writeErr(w, http.StatusBadRequest, "az importálás módja hardlink vagy copy lehet")
			return
		}
		s.st.SetSetting("acq_import_mode", *in.ImportMode)
	}
	s.handleAcqSettings(w, r, c)
}

// ------------------------------------------------------------------ indexers

func validIndexer(x *store.AcqIndexer) string {
	x.Name, x.URL = strings.TrimSpace(x.Name), strings.TrimSpace(x.URL)
	if x.Name == "" {
		return "add meg az indexer nevét"
	}
	if !strings.HasPrefix(x.URL, "http://") && !strings.HasPrefix(x.URL, "https://") {
		return "az indexer címe http:// vagy https:// kezdetű legyen"
	}
	if x.Priority <= 0 {
		x.Priority = 25
	}
	return ""
}

func (s *Server) handleAcqIndexerSave(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var x store.AcqIndexer
	if !decode(w, r, &x) {
		return
	}
	x.ID = pathID(r, "id")
	if x.ID > 0 {
		old, err := s.st.AcqIndexer(x.ID)
		if err != nil {
			writeErr(w, http.StatusNotFound, "nincs ilyen indexer")
			return
		}
		x.APIKey = keepSecret(x.APIKey, old.APIKey)
	}
	if msg := validIndexer(&x); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.st.SaveAcqIndexer(&x); err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, maskIndexer(&x))
}

func (s *Server) handleAcqIndexerDelete(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	s.st.DeleteAcqIndexer(pathID(r, "id"))
	ok(w)
}

func (s *Server) handleAcqIndexerTest(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	m := s.acq(w)
	if m == nil {
		return
	}
	var x store.AcqIndexer
	if !decode(w, r, &x) {
		return
	}
	if x.ID > 0 {
		if old, err := s.st.AcqIndexer(x.ID); err == nil {
			x.APIKey = keepSecret(x.APIKey, old.APIKey)
		}
	}
	if msg := validIndexer(&x); msg != "" && !strings.Contains(msg, "nevét") {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	name, err := m.TestIndexer(r.Context(), acquire.Indexer{ID: x.ID, Name: x.Name, URL: x.URL, APIKey: x.APIKey, Categories: x.Categories})
	if x.ID > 0 {
		e := ""
		if err != nil {
			e = err.Error()
		}
		s.st.MarkAcqIndexer(x.ID, e)
	}
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"server": name})
}

// ------------------------------------------------------------------ download clients

func validClient(x *store.AcqClient) string {
	x.Name, x.URL = strings.TrimSpace(x.Name), strings.TrimSpace(x.URL)
	if x.Name == "" {
		x.Name = "qBittorrent"
	}
	if x.Kind == "" {
		x.Kind = "qbittorrent"
	}
	if x.Kind != "qbittorrent" {
		return "egyelőre csak a qBittorrent támogatott"
	}
	if !strings.HasPrefix(x.URL, "http://") && !strings.HasPrefix(x.URL, "https://") {
		return "a letöltőkliens címe http:// vagy https:// kezdetű legyen"
	}
	x.Category = strings.TrimSpace(x.Category)
	x.PathRemote, x.PathLocal = strings.TrimSpace(x.PathRemote), strings.TrimSpace(x.PathLocal)
	if (x.PathRemote == "") != (x.PathLocal == "") {
		return "az útvonal-leképezéshez mindkét mezőt töltsd ki (vagy egyiket se)"
	}
	return ""
}

func clientConfig(x *store.AcqClient) acquire.ClientConfig {
	return acquire.ClientConfig{ID: x.ID, Kind: x.Kind, URL: x.URL, Username: x.Username, Password: x.Password, Category: x.Category,
		SavePath: x.SavePath, PathRemote: x.PathRemote, PathLocal: x.PathLocal}
}

func (s *Server) handleAcqClientSave(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var x store.AcqClient
	if !decode(w, r, &x) {
		return
	}
	x.ID = pathID(r, "id")
	if x.ID > 0 {
		old, err := s.st.AcqClient(x.ID)
		if err != nil {
			writeErr(w, http.StatusNotFound, "nincs ilyen letöltőkliens")
			return
		}
		x.Password = keepSecret(x.Password, old.Password)
	}
	if msg := validClient(&x); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.st.SaveAcqClient(&x); err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, maskClient(&x))
}

func (s *Server) handleAcqClientDelete(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	s.st.DeleteAcqClient(pathID(r, "id"))
	ok(w)
}

func (s *Server) handleAcqClientTest(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	m := s.acq(w)
	if m == nil {
		return
	}
	var x store.AcqClient
	if !decode(w, r, &x) {
		return
	}
	if x.ID > 0 {
		if old, err := s.st.AcqClient(x.ID); err == nil {
			x.Password = keepSecret(x.Password, old.Password)
		}
	}
	if msg := validClient(&x); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	v, err := m.TestClient(r.Context(), clientConfig(&x))
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"server": v})
}

// ------------------------------------------------------------------ search, grab, queue

func (s *Server) acqReady(w http.ResponseWriter) *acquire.Manager {
	m := s.acq(w)
	if m == nil {
		return nil
	}
	if !m.Enabled() {
		writeErr(w, http.StatusConflict, acquire.ErrDisabled.Error())
		return nil
	}
	return m
}

func (s *Server) handleAcqLookup(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	m := s.acqReady(w)
	if m == nil {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	res, err := m.Lookup(r.Context(), q, queryInt(r, "year", 0))
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleAcqReleases(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	m := s.acqReady(w)
	if m == nil {
		return
	}
	id := int64(queryInt(r, "tmdb_id", 0))
	if id <= 0 {
		writeErr(w, http.StatusBadRequest, "hiányzik a tmdb_id")
		return
	}
	res, err := m.Releases(r.Context(), id)
	if err != nil {
		code := http.StatusBadGateway
		if errors.Is(err, acquire.ErrNoIndex) {
			code = http.StatusConflict
		}
		writeErr(w, code, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleAcqGrab(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	m := s.acqReady(w)
	if m == nil {
		return
	}
	var in struct {
		TmdbID    int64           `json:"tmdb_id"`
		LibraryID int64           `json:"library_id"`
		Release   *acquire.Result `json:"release"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.TmdbID <= 0 {
		writeErr(w, http.StatusBadRequest, "hiányzik a tmdb_id")
		return
	}
	g, err := m.Grab(r.Context(), acquire.GrabInput{TmdbID: in.TmdbID, LibraryID: in.LibraryID, Release: in.Release, UserID: c.User.ID})
	if err != nil {
		code := http.StatusBadGateway
		if errors.Is(err, acquire.ErrNoClient) || errors.Is(err, acquire.ErrNoIndex) || errors.Is(err, acquire.ErrDisabled) {
			code = http.StatusConflict
		}
		writeErr(w, code, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) handleAcqQueue(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	list, err := s.st.AcqGrabs(queryInt(r, "limit", 100))
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleAcqRetry(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	m := s.acq(w)
	if m == nil {
		return
	}
	if err := m.Retry(r.Context(), pathID(r, "id")); err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	ok(w)
}

func (s *Server) handleAcqRemove(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	m := s.acq(w)
	if m == nil {
		return
	}
	if err := m.Remove(r.Context(), pathID(r, "id"), r.URL.Query().Get("files") == "1"); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "nincs ilyen letöltés")
			return
		}
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	ok(w)
}
