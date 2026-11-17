package server

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ScotyLucas/kinora/internal/store"
)

// ------------------------------------------------------------------ users

type adminUser struct {
	*store.User
	Profiles int `json:"profiles"`
}

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	users, err := s.st.ListUsers()
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	out := []adminUser{}
	for _, u := range users {
		ps, _ := s.st.Profiles(u.ID)
		out = append(out, adminUser{u, len(ps)})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleUserCreate(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
		IsAdmin  bool   `json:"is_admin"`
	}
	if !decode(w, r, &in) {
		return
	}
	if msg := validCredentials(in.Username, in.Password); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	u, err := s.st.CreateUser(strings.TrimSpace(in.Username), in.Password, in.IsAdmin)
	if errors.Is(err, store.ErrExists) {
		writeErr(w, http.StatusConflict, err.Error())
		return
	} else if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

func (s *Server) handleUserUpdate(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	id := pathID(r, "id")
	if _, err := s.st.UserByID(id); err != nil {
		writeErr(w, http.StatusNotFound, "a felhasználó nem található")
		return
	}
	var in struct {
		Password *string `json:"password"`
		IsAdmin  *bool   `json:"is_admin"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Password != nil {
		if msg := validCredentials("placeholder", *in.Password); msg != "" {
			writeErr(w, http.StatusBadRequest, msg)
			return
		}
		keep := ""
		if id == c.User.ID {
			keep = c.Token
		}
		if err := s.st.SetPassword(id, *in.Password, keep); err != nil {
			s.internalErr(w, r, err)
			return
		}
	}
	if in.IsAdmin != nil {
		if id == c.User.ID && !*in.IsAdmin {
			writeErr(w, http.StatusBadRequest, "saját magadtól nem veheted el az adminjogot")
			return
		}
		s.st.SetAdmin(id, *in.IsAdmin)
	}
	ok(w)
}

func (s *Server) handleUserDelete(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	id := pathID(r, "id")
	if id == c.User.ID {
		writeErr(w, http.StatusBadRequest, "saját magadat nem törölheted")
		return
	}
	if err := s.st.DeleteUser(id); err != nil {
		s.internalErr(w, r, err)
		return
	}
	ok(w)
}

// ------------------------------------------------------------------ invites

func (s *Server) handleInvites(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	inv, err := s.st.Invites()
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, inv)
}

func (s *Server) handleInviteCreate(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in struct {
		Note  string `json:"note"`
		Uses  int    `json:"uses"`
		Hours int    `json:"hours"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Uses > 100 {
		in.Uses = 100
	}
	inv, err := s.st.CreateInvite(c.User.ID, strings.TrimSpace(in.Note), in.Uses, in.Hours)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, inv)
}

func (s *Server) handleInviteDelete(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	s.st.DeleteInvite(r.PathValue("code"))
	ok(w)
}

// ------------------------------------------------------------------ settings

var settingKeys = []string{"server_name", "public_url", "scan_interval_min", "tmdb_key", "metadata_lang", "encoder",
	"quickconnect_enabled", "discovery_enabled", "brand_accent", "intro_detection", "intro_window_min"}

// SettingEnv maps settings to the environment variables that can force them.
var SettingEnv = map[string]string{
	"server_name":       "KINORA_SERVER_NAME",
	"public_url":        "KINORA_PUBLIC_URL",
	"scan_interval_min": "KINORA_SCAN_INTERVAL_MIN",
	"tmdb_key":          "KINORA_TMDB_KEY",
	"metadata_lang":     "KINORA_METADATA_LANG",
	"encoder":           "KINORA_ENCODER",
	"quickconnect_enabled": "KINORA_QUICKCONNECT",
	"discovery_enabled":    "KINORA_DISCOVERY",
	"brand_accent":         "KINORA_ACCENT",
	"intro_detection":      "KINORA_INTRO_DETECTION",
	"intro_window_min":     "KINORA_INTRO_WINDOW_MIN",
}

const maskChar = "•"

func maskSecret(v string) string {
	if v == "" {
		return ""
	}
	if len(v) <= 4 {
		return strings.Repeat(maskChar, 4)
	}
	return strings.Repeat(maskChar, 8) + v[len(v)-4:]
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	vals := map[string]string{}
	env := map[string]string{}
	for _, k := range settingKeys {
		v := s.st.Setting(k)
		if k == "tmdb_key" {
			v = maskSecret(v)
		}
		vals[k] = v
		if s.st.Overridden(k) {
			env[k] = SettingEnv[k]
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"values": vals, "env": env})
}

func (s *Server) handleSettingsPatch(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in map[string]string
	if !decode(w, r, &in) {
		return
	}
	var changed []string
	for _, k := range settingKeys {
		v, present := in[k]
		if !present {
			continue
		}
		v = strings.TrimSpace(v)
		if s.st.Overridden(k) {
			continue // environment wins; the UI shows the field as locked
		}
		switch k {
		case "tmdb_key":
			if strings.Contains(v, maskChar) {
				continue // masked value echoed back: keep the stored key
			}
		case "scan_interval_min":
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 || n > 10080 {
				writeErr(w, http.StatusBadRequest, "a szkennelési intervallum 0–10080 perc lehet (0 = kikapcsolva)")
				return
			}
		case "public_url":
			if v != "" && !strings.HasPrefix(v, "http://") && !strings.HasPrefix(v, "https://") {
				writeErr(w, http.StatusBadRequest, "a nyilvános URL http:// vagy https:// kezdetű legyen")
				return
			}
			v = strings.TrimRight(v, "/")
		case "intro_window_min":
			n, err := strconv.Atoi(v)
			if err != nil || n < 2 || n > 30 {
				writeErr(w, http.StatusBadRequest, "az intró keresési ablaka 2–30 perc lehet")
				return
			}
		case "quickconnect_enabled", "discovery_enabled", "intro_detection":
			if v != "0" && v != "1" {
				writeErr(w, http.StatusBadRequest, "a kapcsoló értéke 0 vagy 1 lehet")
				return
			}
		case "brand_accent":
			if v != "" && !reHexColor.MatchString(v) {
				writeErr(w, http.StatusBadRequest, "az akcentszín #rrggbb formátumú legyen")
				return
			}
		case "server_name":
			if v == "" {
				writeErr(w, http.StatusBadRequest, "a szerver neve nem lehet üres")
				return
			}
		}
		if err := s.st.SetSetting(k, v); err != nil {
			s.internalErr(w, r, err)
			return
		}
		changed = append(changed, k)
	}
	if len(changed) > 0 {
		s.log.Info("settings changed", "keys", strings.Join(changed, ","))
	}
	s.handleSettings(w, r, c)
}

// ------------------------------------------------------------------ backup

func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	tmp := filepath.Join(s.cfg.CacheDir, fmt.Sprintf("backup-%d.db", time.Now().UnixNano()))
	defer os.Remove(tmp)
	if err := s.st.Backup(r.Context(), tmp); err != nil {
		s.internalErr(w, r, fmt.Errorf("mentés sikertelen: %w", err))
		return
	}
	f, err := os.Open(tmp)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	defer f.Close()
	name := "kinora-backup-" + time.Now().Format("20060102-150405") + ".db"
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	http.ServeContent(w, r, name, time.Now(), f)
}

// ------------------------------------------------------------------ connect

func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	_, port, _ := net.SplitHostPort(s.cfg.Addr)
	if port == "" {
		port = "8096"
	}
	type addr struct {
		Iface string `json:"iface"`
		URL   string `json:"url"`
		Kind  string `json:"kind"` // lan | tailscale | other
	}
	addrs := []addr{}
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		as, _ := ifc.Addrs()
		for _, a := range as {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP.To4() == nil || ipn.IP.IsLoopback() {
				continue
			}
			kind := "other"
			ip := ipn.IP.To4()
			switch {
			case ip[0] == 100 && ip[1]&0xC0 == 64: // 100.64.0.0/10 CGNAT = Tailscale
				kind = "tailscale"
			case ip.IsPrivate():
				kind = "lan"
			}
			addrs = append(addrs, addr{Iface: ifc.Name, URL: "http://" + ip.String() + ":" + port, Kind: kind})
		}
	}
	sort.SliceStable(addrs, func(i, j int) bool { return addrs[i].Kind < addrs[j].Kind })
	writeJSON(w, http.StatusOK, map[string]any{
		"public_url":   s.st.Setting("public_url"),
		"addresses":    addrs,
		"port":         port,
		"in_container": fileExists("/.dockerenv"),
	})
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// ------------------------------------------------------------------ folder picker

func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	p := r.URL.Query().Get("path")
	if p == "" {
		p = s.cfg.MediaRoot
		if (p == "" || !fileExists(p)) && installMode() != "docker" {
			p = "/" // native install: the whole file system, shortcuts lead to /home, /mnt, /media, /srv
		}
		if p == "" || !fileExists(p) {
			p, _ = os.UserHomeDir()
		}
		if p == "" {
			p = "/"
		}
	}
	p = filepath.Clean(p)
	entries, err := os.ReadDir(p)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "a mappa nem olvasható: "+err.Error())
		return
	}
	dirs := []string{}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.IsDir() || (e.Type()&os.ModeSymlink != 0 && isDir(filepath.Join(p, e.Name()))) {
			dirs = append(dirs, e.Name())
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return strings.ToLower(dirs[i]) < strings.ToLower(dirs[j]) })
	parent := filepath.Dir(p)
	if parent == p {
		parent = ""
	}
	// folders the service user cannot enter are shown locked, so the admin knows before adding them
	locked := []string{}
	for _, d := range dirs {
		if f, err := os.Open(filepath.Join(p, d)); err != nil {
			locked = append(locked, d)
		} else {
			f.Close()
		}
	}
	mode := installMode()
	writeJSON(w, http.StatusOK, map[string]any{"path": p, "parent": parent, "dirs": dirs, "locked": locked, "sep": string(filepath.Separator),
		"mode": mode, "shortcuts": browseShortcuts(mode, s.cfg.MediaRoot), "host_path": hostPath(mode, p)})
}

func isDir(p string) bool { st, err := os.Stat(p); return err == nil && st.IsDir() }

// ------------------------------------------------------------------ dashboard

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	d := map[string]any{
		"version":    Version,
		"go":         runtime.Version(),
		"os":         runtime.GOOS + "/" + runtime.GOARCH,
		"uptime_sec": int(time.Since(s.started).Seconds()),
		"started_at": s.started.Unix(),
		"schema":     s.st.SchemaVersion(),
	}
	s.dashboardExtras(d)
	writeJSON(w, http.StatusOK, d)
}
