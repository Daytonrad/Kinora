package server

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ScotyLucas/kinora/internal/scanner"
	"github.com/ScotyLucas/kinora/internal/store"
)

func (s *Server) handleLibraries(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	libs, err := s.visibleLibraries(c.Profile)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, libs)
}

// visibleLibraries lists the libraries the profile's account may see.
func (s *Server) visibleLibraries(p *store.Profile) ([]*store.Library, error) {
	libs, err := s.st.Libraries()
	if err != nil {
		return nil, err
	}
	out := libs[:0]
	for _, l := range libs {
		if p.LibraryAllowed(l.ID) {
			out = append(out, l)
		}
	}
	return out, nil
}

func (s *Server) handleLibraryCreate(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in struct {
		Name  string   `json:"name"`
		Path  string   `json:"path"`
		Paths []string `json:"paths"` // several folders; overrides path
		Kind  string   `json:"kind"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Kind != "movies" && in.Kind != "shows" && in.Kind != "mixed" {
		writeErr(w, http.StatusBadRequest, "a könyvtár típusa movies, shows vagy mixed lehet")
		return
	}
	if len(in.Paths) == 0 {
		in.Paths = []string{in.Path}
	}
	paths, ok := s.libraryFolders(w, in.Paths)
	if !ok {
		return
	}
	if in.Name == "" {
		in.Name = filepath.Base(paths[0])
	}
	lib, err := s.st.CreateLibraryPaths(in.Name, paths, in.Kind)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.log.Info("library added", "name", lib.Name, "paths", lib.Paths, "kind", lib.Kind)
	s.Scanner.Trigger(s.bgCtx, lib.ID)
	writeJSON(w, http.StatusCreated, lib)
}

// libraryFolders validates the folders of a library (empty entries are dropped): each must pass
// libraryFolder, and none may repeat or lie inside another (its files would be scanned twice).
func (s *Server) libraryFolders(w http.ResponseWriter, in []string) ([]string, bool) {
	var out []string
	for _, raw := range in {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		p, ok := s.libraryFolder(w, strings.TrimSpace(raw))
		if !ok {
			return nil, false
		}
		for _, q := range out {
			if samePath(p, q) || within(p, q) || within(q, p) {
				writeErr(w, http.StatusBadRequest, "a mappák nem ismétlődhetnek és nem lehetnek egymáson belül: "+q+" – "+p)
				return nil, false
			}
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		writeErr(w, http.StatusBadRequest, "adj meg egy abszolút mappaútvonalat")
		return nil, false
	}
	return out, true
}

// within reports whether p is inside dir.
func within(p, dir string) bool {
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func samePath(a, b string) bool {
	if os.PathSeparator == '\\' {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// libraryFolder validates a library folder: absolute, an existing directory, readable by the
// server. It answers the request itself and returns false when the folder is not usable.
func (s *Server) libraryFolder(w http.ResponseWriter, path string) (string, bool) {
	if path == "" || !filepath.IsAbs(path) {
		writeErr(w, http.StatusBadRequest, "adj meg egy abszolút mappaútvonalat")
		return "", false
	}
	p := filepath.Clean(path)
	st, err := os.Stat(p)
	if err != nil && !isPerm(err) || err == nil && !st.IsDir() {
		writeErr(w, http.StatusBadRequest, "a mappa nem létezik: "+p)
		return "", false
	}
	// an unreadable folder would just scan empty: say why and how to fix it
	// (on Windows the server first grants itself the rights)
	if rep, err := s.fixPerms(p); err == nil && !rep.OK {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": rep.Message, "perm": rep})
		return "", false
	}
	return p, true
}

// handleLibraryUpdate renames a library or points it at another folder. Its items stay: on the
// next scan files found again in the new folder (same size) keep their id and watch state.
func (s *Server) handleLibraryUpdate(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	id := pathID(r, "id")
	lib, err := s.st.Library(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "a könyvtár nem található")
		return
	}
	var in struct {
		Name  *string   `json:"name"`
		Path  *string   `json:"path"`
		Paths *[]string `json:"paths"` // all folders; overrides path
	}
	if !decode(w, r, &in) {
		return
	}
	name, paths := lib.Name, lib.Paths
	if in.Name != nil {
		if name = strings.TrimSpace(*in.Name); name == "" || len([]rune(name)) > 60 {
			writeErr(w, http.StatusBadRequest, "a könyvtár neve 1–60 karakter lehet")
			return
		}
	}
	if in.Paths == nil && in.Path != nil { // older clients: replace the main folder only
		p := append([]string{*in.Path}, lib.Paths[1:]...)
		in.Paths = &p
	}
	if in.Paths != nil && !slices.Equal(cleanAll(*in.Paths), lib.Paths) {
		p, ok := s.libraryFolders(w, *in.Paths)
		if !ok {
			return
		}
		paths = p
	}
	if err := s.st.UpdateLibrary(id, name, paths); err != nil {
		s.internalErr(w, r, err)
		return
	}
	if !slices.Equal(paths, lib.Paths) {
		s.log.Info("library folders changed", "name", name, "from", lib.Paths, "to", paths)
		s.Scanner.Cancel(id)
		go func() { // a cancelled scan of the old folder needs a moment to let go of the library
			for i := 0; i < 300 && !s.Scanner.Trigger(s.bgCtx, id); i++ {
				time.Sleep(100 * time.Millisecond)
			}
		}()
	}
	updated, err := s.st.Library(id)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
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
	lib, err := s.st.Library(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "a könyvtár nem található")
		return
	}
	if canGrantRead { // Windows: a folder the service cannot read gets the rights before the scan
		for _, p := range lib.Paths {
			if rep, err := s.fixPerms(p); err == nil && !rep.OK {
				s.log.Warn("library not readable", "path", p, "user", rep.User)
			}
		}
	}
	started := s.Scanner.Trigger(s.bgCtx, id)
	writeJSON(w, http.StatusAccepted, map[string]bool{"started": started})
}

func cleanAll(in []string) []string {
	out := []string{}
	for _, p := range in {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, filepath.Clean(p))
		}
	}
	return out
}

func (s *Server) handleScanStatus(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	writeJSON(w, http.StatusOK, s.Scanner.Statuses())
}

// dashboardExtras adds library/scan/transcode state to the admin dashboard.
func (s *Server) dashboardExtras(d map[string]any) {
	d["library"] = s.st.Stats()
	d["party_reports"] = s.st.CountPartyReports()
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
