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
	profiles, _ := s.st.AcqProfiles()
	libs, _ := s.st.Libraries()
	movieLibs, tvLibs := []*store.Library{}, []*store.Library{}
	for _, l := range libs {
		if l.Kind != "shows" {
			movieLibs = append(movieLibs, l)
		}
		if l.Kind != "movies" {
			tvLibs = append(tvLibs, l)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":      s.st.Setting("acq_enabled") == "1",
		"profile":      acquire.ParseProfile(s.st.Setting("acq_profile")),
		"import_mode":  s.st.Setting("acq_import_mode"),
		"auto_approve": s.st.Setting("acq_auto_approve"),
		"quota_week":   s.st.SettingInt("acq_quota_week", 0),
		"library_id":   s.st.SettingInt("acq_library", 0),
		"retry_hours":  s.st.SettingInt("acq_retry_hours", 6),
		"library_tv":   s.st.SettingInt("acq_library_tv", 0),
		"tv_libraries": tvLibs,
		"profiles":     profiles,
		"indexers":     outI,
		"clients":      outC,
		"libraries":    movieLibs,
		"tmdb":         strings.TrimSpace(s.st.Setting("tmdb_key")) != "",
	})
}

func (s *Server) handleAcqSettingsPut(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in struct {
		Enabled     *bool            `json:"enabled"`
		Profile     *acquire.Profile `json:"profile"`
		ImportMode  *string          `json:"import_mode"`
		AutoApprove *string          `json:"auto_approve"`
		QuotaWeek   *int             `json:"quota_week"`
		LibraryID   *int             `json:"library_id"`
		RetryHours  *int             `json:"retry_hours"`
		LibraryTV   *int             `json:"library_tv"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !s.saveRequestSettings(w, in.AutoApprove, in.QuotaWeek, in.LibraryID, in.RetryHours, in.LibraryTV) {
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
	if x.Kind == "" {
		x.Kind = "torznab"
	}
	if x.Kind != "torznab" && x.Kind != "newznab" {
		return "az indexer típusa torznab vagy newznab lehet"
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
	if x.Kind == "" {
		x.Kind = "qbittorrent"
	}
	name, known := clientNames[x.Kind]
	if !known {
		return "ismeretlen letöltőkliens"
	}
	if x.Name == "" {
		x.Name = name
	}
	if (x.Kind == "sabnzbd" || x.Kind == "deluge") && x.Password == "" {
		return "add meg a jelszót (SABnzbd-nél az API-kulcsot)"
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

var clientNames = map[string]string{"qbittorrent": "qBittorrent", "transmission": "Transmission", "deluge": "Deluge", "sabnzbd": "SABnzbd", "nzbget": "NZBGet"}

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
	if r.URL.Query().Get("kind") == "series" {
		res, err := m.LookupSeries(r.Context(), q)
		if err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, res)
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
	res, err := m.ReleasesProfile(r.Context(), id, int64(queryInt(r, "profile_id", 0)))
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
		RequestID int64           `json:"request_id"` // the film request this download is for (interactive search from Kérések)
		ProfileID int64           `json:"profile_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.TmdbID <= 0 {
		writeErr(w, http.StatusBadRequest, "hiányzik a tmdb_id")
		return
	}
	g, err := m.Grab(r.Context(), acquire.GrabInput{TmdbID: in.TmdbID, LibraryID: in.LibraryID, Release: in.Release, UserID: c.User.ID, RequestID: in.RequestID, ProfileID: in.ProfileID})
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

// ------------------------------------------------------------------ series (M3)

func (s *Server) handleAcqSeriesList(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	list, err := s.st.AcqSeriesList()
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type seriesBody struct {
	TmdbID    int64  `json:"tmdb_id"`
	ProfileID *int64 `json:"profile_id"`
	LibraryID int64  `json:"library_id"`
	Monitor   string `json:"monitor"`
	Seasons   []int  `json:"seasons"`
}

func (s *Server) handleAcqSeriesAdd(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	m := s.acqReady(w)
	if m == nil {
		return
	}
	var in seriesBody
	if !decode(w, r, &in) {
		return
	}
	if in.TmdbID <= 0 {
		writeErr(w, http.StatusBadRequest, "hiányzik a tmdb_id")
		return
	}
	x, err := m.AddSeries(r.Context(), acquire.SeriesInput{TmdbID: in.TmdbID, LibraryID: in.LibraryID, Monitor: in.Monitor, Seasons: in.Seasons, UserID: c.User.ID})
	if err != nil {
		writeErr(w, acqErrStatus(err), err.Error())
		return
	}
	if in.ProfileID != nil {
		s.st.SetSeriesProfile(x.ID, *in.ProfileID)
	}
	writeJSON(w, http.StatusOK, x)
}

func (s *Server) handleAcqSeriesGet(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	x, err := s.st.AcqSeriesByID(pathID(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "nincs ilyen figyelt sorozat")
		return
	}
	eps, err := s.st.Episodes(x.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"series": x, "episodes": eps})
}

func (s *Server) handleAcqSeriesPatch(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	m := s.acqReady(w)
	if m == nil {
		return
	}
	var in seriesBody
	if !decode(w, r, &in) {
		return
	}
	x, err := m.SetSeriesMonitor(r.Context(), pathID(r, "id"), in.Monitor, in.Seasons, in.LibraryID)
	if err != nil {
		writeErr(w, acqErrStatus(err), err.Error())
		return
	}
	if in.ProfileID != nil {
		s.st.SetSeriesProfile(x.ID, *in.ProfileID)
	}
	writeJSON(w, http.StatusOK, x)
}

func (s *Server) handleAcqSeriesSearch(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	m := s.acqReady(w)
	if m == nil {
		return
	}
	if err := m.SearchSeriesNow(pathID(r, "id")); err != nil {
		writeErr(w, acqErrStatus(err), err.Error())
		return
	}
	ok(w)
}

func (s *Server) handleAcqSeriesDelete(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	s.st.DeleteAcqSeries(pathID(r, "id"))
	ok(w)
}

// handleAcqSeriesReleases is the interactive search of a season (episode omitted) or of one episode.
func (s *Server) handleAcqSeriesReleases(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	m := s.acqReady(w)
	if m == nil {
		return
	}
	season := queryInt(r, "season", -1)
	if season < 0 {
		writeErr(w, http.StatusBadRequest, "hiányzik az évad")
		return
	}
	x, res, err := m.SeriesReleases(r.Context(), pathID(r, "id"), season, queryInt(r, "episode", -1))
	if err != nil {
		code := acqErrStatus(err)
		if errors.Is(err, acquire.ErrNoIndex) {
			code = http.StatusConflict
		}
		writeErr(w, code, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"series": x, "results": res})
}

func (s *Server) handleAcqSeriesGrab(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	m := s.acqReady(w)
	if m == nil {
		return
	}
	var in struct {
		Season  int             `json:"season"`
		Release *acquire.Result `json:"release"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Release == nil || in.Season < 0 {
		writeErr(w, http.StatusBadRequest, "hiányzik a kiadás vagy az évad")
		return
	}
	if err := m.GrabSeriesRelease(r.Context(), pathID(r, "id"), in.Season, *in.Release); err != nil {
		writeErr(w, acqErrStatus(err), err.Error())
		return
	}
	ok(w)
}

func (s *Server) handleAcqSeasonSearch(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	m := s.acqReady(w)
	if m == nil {
		return
	}
	if err := m.SearchSeason(pathID(r, "id"), queryInt(r, "season", -1)); err != nil {
		writeErr(w, acqErrStatus(err), err.Error())
		return
	}
	ok(w)
}

// ------------------------------------------------------------------ quality profiles, statistics (M4)

func (s *Server) handleAcqProfileSave(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in struct {
		Name      string          `json:"name"`
		IsDefault bool            `json:"is_default"`
		Profile   acquire.Profile `json:"profile"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		writeErr(w, http.StatusBadRequest, "add meg a profil nevét")
		return
	}
	if len(in.Profile.Resolutions) == 0 {
		writeErr(w, http.StatusBadRequest, "legalább egy felbontást engedélyezz")
		return
	}
	p := &store.AcqProfile{ID: pathID(r, "id"), Name: in.Name, IsDefault: in.IsDefault, JSON: acquire.ProfileJSON(in.Profile)}
	if p.ID > 0 {
		if _, err := s.st.AcqProfileByID(p.ID); err != nil {
			writeErr(w, http.StatusNotFound, "nincs ilyen profil")
			return
		}
	}
	if err := s.st.SaveAcqProfile(p); err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleAcqProfileDelete(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	s.st.DeleteAcqProfile(pathID(r, "id"))
	ok(w)
}

func (s *Server) handleAcqStats(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	st, err := s.st.AcqStatistics()
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}
