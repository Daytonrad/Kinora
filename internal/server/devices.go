package server

import (
	"net/http"
	"strings"

	"github.com/ScotyLucas/kinora/internal/store"
)

// deviceFromRequest fills device info from the login body, falling back to a
// short description derived from the User-Agent for browsers.
func deviceFromRequest(r *http.Request, d store.DeviceInfo) store.DeviceInfo {
	d.IP = clientIP(r)
	if strings.TrimSpace(d.Name) == "" {
		d.Name = browserName(r.UserAgent())
	}
	return d.Clean()
}

func browserName(ua string) string {
	b, o := "Böngésző", ""
	switch {
	case strings.Contains(ua, "Edg/"):
		b = "Edge"
	case strings.Contains(ua, "OPR/"):
		b = "Opera"
	case strings.Contains(ua, "Firefox/"):
		b = "Firefox"
	case strings.Contains(ua, "Chrome/"):
		b = "Chrome"
	case strings.Contains(ua, "Safari/"):
		b = "Safari"
	}
	switch {
	case strings.Contains(ua, "Tizen"):
		o = "Samsung TV"
	case strings.Contains(ua, "Web0S"), strings.Contains(ua, "webOS"):
		o = "LG TV"
	case strings.Contains(ua, "Android"):
		o = "Android"
	case strings.Contains(ua, "iPhone"), strings.Contains(ua, "iPad"):
		o = "iOS"
	case strings.Contains(ua, "Windows"):
		o = "Windows"
	case strings.Contains(ua, "Mac OS"):
		o = "macOS"
	case strings.Contains(ua, "Linux"):
		o = "Linux"
	}
	if o == "" {
		return b
	}
	return b + " – " + o
}

func markCurrent(list []store.Device, current string) []store.Device {
	for i := range list {
		list[i].Current = list[i].ID == current
	}
	return list
}

// GET /api/devices – the caller's logged-in devices.
func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	list, err := s.st.Devices(c.User.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": markCurrent(list, c.DeviceID)})
}

// DELETE /api/devices/{id} – log out one of your devices (admins: any device).
func (s *Server) handleDeviceDelete(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	if c.ProfileLock > 0 || (c.Profile != nil && c.Profile.IsKids) {
		writeErr(w, http.StatusForbidden, "gyerekprofilból nem lehet eszközt kiléptetni")
		return
	}
	owner := c.User.ID
	if c.User.IsAdmin {
		owner = 0
	}
	found, err := s.st.DeleteDevice(owner, r.PathValue("id"))
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if !found {
		writeErr(w, http.StatusNotFound, "az eszköz nem található")
		return
	}
	ok(w)
}

// DELETE /api/devices – log out every other device (?all=1 includes this one).
func (s *Server) handleDevicesDelete(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	if c.ProfileLock > 0 || (c.Profile != nil && c.Profile.IsKids) {
		writeErr(w, http.StatusForbidden, "gyerekprofilból nem lehet eszközt kiléptetni")
		return
	}
	keep := c.DeviceID
	if r.URL.Query().Get("all") == "1" {
		keep = ""
	}
	n, err := s.st.DeleteDevices(c.User.ID, keep)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if keep == "" {
		clearSessionCookie(w, r)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "removed": n})
}

// GET /api/admin/devices – every device of every account.
func (s *Server) handleAdminDevices(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	list, err := s.st.AllDevices()
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": markCurrent(list, c.DeviceID)})
}
