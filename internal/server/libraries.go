package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ScotyLucas/kinora/internal/scanner"
)

func (s *Server) handleLibraries(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	libs, err := s.st.Libraries()
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, libs)
}

func (s *Server) handleLibraryCreate(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Kind string `json:"kind"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Kind != "movies" && in.Kind != "shows" && in.Kind != "mixed" {
		writeErr(w, http.StatusBadRequest, "a könyvtár típusa movies, shows vagy mixed lehet")
		return
	}
	if in.Path == "" || !filepath.IsAbs(in.Path) {
		writeErr(w, http.StatusBadRequest, "adj meg egy abszolút mappaútvonalat")
		return
	}
	p := filepath.Clean(in.Path)
	st, err := os.Stat(p)
	if err != nil || !st.IsDir() {
		writeErr(w, http.StatusBadRequest, "a mappa nem létezik vagy nem olvasható: "+p)
		return
	}
	if in.Name == "" {
		in.Name = filepath.Base(p)
	}
	lib, err := s.st.CreateLibrary(in.Name, p, in.Kind)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.log.Info("library added", "name", lib.Name, "path", lib.Path, "kind", lib.Kind)
	s.Scanner.Trigger(s.bgCtx, lib.ID)
	writeJSON(w, http.StatusCreated, lib)
}

func (s *Server) handleLibraryDelete(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	id := pathID(r, "id")
	if _, err := s.st.Library(id); err != nil {
		writeErr(w, http.StatusNotFound, "a könyvtár nem található")
		return
	}
	s.Scanner.Cancel(id)
	ids, err := s.st.DeleteLibrary(id)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.Scanner.Forget(id)
	for _, iid := range ids {
		os.RemoveAll(scanner.ImageDir(s.ImgDir, iid))
	}
	ok(w)
}

func (s *Server) handleLibraryScan(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	id := pathID(r, "id")
	if _, err := s.st.Library(id); err != nil {
		writeErr(w, http.StatusNotFound, "a könyvtár nem található")
		return
	}
	started := s.Scanner.Trigger(s.bgCtx, id)
	writeJSON(w, http.StatusAccepted, map[string]bool{"started": started})
}

func (s *Server) handleScanStatus(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	writeJSON(w, http.StatusOK, s.Scanner.Statuses())
}

// dashboardExtras adds library/scan/transcode state to the admin dashboard.
func (s *Server) dashboardExtras(d map[string]any) {
	d["library"] = s.st.Stats()
	d["scans"] = s.Scanner.Statuses()
	d["transcodes"] = s.Media.Sessions()
	res, at := s.Encoders.Results()
	d["encoders"] = res
	d["encoders_tested_at"] = at.Unix()
	d["encoder_selected"] = s.Encoders.Pick(s.encoderSetting())
	d["tonemap"] = s.Encoders.ToneMap()
	d["downloads_active"] = s.Downloads.Active()
	rooms, members := s.Party.Count()
	d["parties"] = map[string]int{"rooms": rooms, "members": members}
}
