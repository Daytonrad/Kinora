package server

import (
	"errors"
	"net"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ScotyLucas/kinora/internal/store"
)

func validCredentials(username, password string) string {
	u := strings.TrimSpace(username)
	if utf8.RuneCountInString(u) < 2 || utf8.RuneCountInString(u) > 32 {
		return "a felhasználónév 2–32 karakter lehet"
	}
	if strings.ContainsAny(u, "/\\<>\"'") {
		return "a felhasználónév nem tartalmazhat speciális karaktereket (/ \\ < > \" ')"
	}
	if len(password) < 6 {
		return "a jelszó legalább 6 karakter legyen"
	}
	if len(password) > 256 {
		return "a jelszó túl hosszú"
	}
	return ""
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	n, err := s.st.CountUsers()
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"product":              "kinora",
		"setup_needed":         n == 0,
		"setup_blocked":        n == 0 && !s.setupAllowed(r), // the setup page explains it before the form
		"version":              Version,
		"api_version":          APIVersion,
		"min_client_api":       MinClientAPI,
		"server_id":            s.serverID(),
		"server_name":          s.st.Setting("server_name"),
		"public_url":           s.st.Setting("public_url"), // apps switch to it outside the home network
		"logo_url":             s.logoURL(),
		"quickconnect_enabled": s.qcEnabled(),
		"logged_in":            c.User != nil,
	})
}

// errSetupRemote is shown on the setup page (translated by the web client's srv table).
const errSetupRemote = "a kezdő beállítás csak a helyi hálózatról végezhető el (vagy KINORA_ALLOW_REMOTE_SETUP=1 beállítással)"

// setupAllowed: the first admin can be created only from the local network (private, loopback, link-local),
// or over Tailscale, unless KINORA_ALLOW_REMOTE_SETUP=1. A server exposed to the internet before its setup can't
// be claimed by a stranger. The address is clientIP: a reverse proxy on the local network passes the real one.
// Where every client arrives from a private address (some Docker setups), this check can't tell them apart:
// it is a safety net, not a reason to expose a server before its setup.
func (s *Server) setupAllowed(r *http.Request) bool {
	if s.cfg.AllowRemoteSetup {
		return true
	}
	ip := net.ParseIP(clientIP(r))
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || isTailscaleIP(ip))
}

type credentials struct {
	ServerName string `json:"server_name"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	Code       string `json:"code"`
	store.DeviceInfo
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in credentials
	if !decode(w, r, &in) {
		return
	}
	if n, err := s.st.CountUsers(); err != nil {
		s.internalErr(w, r, err)
		return
	} else if n > 0 { // fast path; CreateFirstAdmin re-checks atomically
		writeErr(w, http.StatusConflict, store.ErrSetupDone.Error())
		return
	}
	if !s.setupAllowed(r) {
		s.log.Warn("setup refused from a public address", "ip", clientIP(r))
		writeErr(w, http.StatusForbidden, errSetupRemote)
		return
	}
	if msg := validCredentials(in.Username, in.Password); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	u, err := s.st.CreateFirstAdmin(strings.TrimSpace(in.Username), in.Password)
	if errors.Is(err, store.ErrSetupDone) {
		writeErr(w, http.StatusConflict, err.Error())
		return
	} else if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if name := strings.TrimSpace(in.ServerName); name != "" && !s.st.Overridden("server_name") {
		s.st.SetSetting("server_name", name)
	}
	s.st.SetSetting(settingWizard, "0") // a new install: the first-run wizard follows (setupwizard.go)
	s.log.Info("setup complete", "admin", u.Username)
	s.startSession(w, r, u, in.DeviceInfo)
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, u *store.User, d store.DeviceInfo) {
	tok, dev, err := s.st.CreateDeviceSession(u.ID, deviceFromRequest(r, d), 0, 0)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.webSessionCookie(w, r, tok)
	writeJSON(w, http.StatusOK, map[string]any{"token": tok, "device_id": dev, "user": u})
}

// webSessionCookie sets the session cookie for the web client; native apps
// (their own origin) keep the token from the response instead.
func (s *Server) webSessionCookie(w http.ResponseWriter, r *http.Request, tok string) {
	if o := r.Header.Get("Origin"); o == "" || sameOrigin(r, o) || !s.isAppOrigin(o) {
		s.rememberActive(w, r, tok) // someone else signed in here: keep them one tap away
		s.setSessionCookie(w, r, tok)
	}
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in credentials
	if !decode(w, r, &in) {
		return
	}
	if !s.loginAllowed(r) {
		writeErr(w, http.StatusTooManyRequests, "túl sok bejelentkezési kísérlet, próbáld újra pár perc múlva")
		return
	}
	u, err := s.st.Authenticate(strings.TrimSpace(in.Username), in.Password)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "hibás felhasználónév vagy jelszó")
		return
	}
	s.startSession(w, r, u, in.DeviceInfo)
}

// loginAllowed rate-limits password attempts per client IP, and also per TCP peer: behind a
// reverse proxy every remote client shares the proxy's peer address, so the peer budget caps
// what a client that forges its forwarding headers could still try.
func (s *Server) loginAllowed(r *http.Request) bool {
	return s.limiter.allow("login:"+clientIP(r), 10, 5*time.Minute) &&
		s.limiter.allow("login-peer:"+peerIP(r), 60, 5*time.Minute)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	if c.Token != "" {
		s.st.DeleteSession(c.Token)
	}
	// another account remembered on this browser takes over; otherwise the browser is signed out
	switched := s.afterLogout(w, r)
	if !switched {
		clearSessionCookie(w, r)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "switched": switched})
}

func (s *Server) handleInviteInfo(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	inv, err := s.st.Invite(r.PathValue("code"))
	if err != nil || !inv.Valid() {
		writeErr(w, http.StatusNotFound, store.ErrInviteInvalid.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": true, "server_name": s.st.Setting("server_name"), "note": inv.Note})
}

func (s *Server) handleJoin(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in credentials
	if !decode(w, r, &in) {
		return
	}
	if !s.limiter.allow("join:"+clientIP(r), 10, 10*time.Minute) {
		writeErr(w, http.StatusTooManyRequests, "túl sok próbálkozás, próbáld újra később")
		return
	}
	if msg := validCredentials(in.Username, in.Password); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	u, err := s.st.RedeemInvite(in.Code, strings.TrimSpace(in.Username), in.Password)
	switch {
	case errors.Is(err, store.ErrInviteInvalid):
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, store.ErrExists):
		writeErr(w, http.StatusConflict, err.Error())
		return
	case err != nil:
		s.internalErr(w, r, err)
		return
	}
	s.log.Info("user joined via invite", "user", u.Username)
	s.startSession(w, r, u, in.DeviceInfo)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	profiles, err := s.st.Profiles(c.User.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if c.ProfileLock > 0 { // a kids device only ever sees its own profile
		var only []*store.Profile
		for _, p := range profiles {
			if p.ID == c.ProfileLock {
				only = append(only, p)
			}
		}
		profiles = only
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":           c.User,
		"profile":        c.Profile,
		"profiles":       profiles,
		"profile_locked": c.ProfileLock > 0,
		"device_id":      c.DeviceID,
		"media_token":    s.mintStreamToken(c, 0), // artwork for <img> without headers
		"server_name":    s.st.Setting("server_name"),
		"version":        Version,
		"api_version":    APIVersion,
		// Felfedezés: films can be requested (the acquisition module is on and this is not a kids profile)
		"can_request": s.Acquire != nil && s.Acquire.Enabled() && !(c.Profile != nil && c.Profile.IsKids),
	})
}

func (s *Server) handlePassword(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !s.loginAllowed(r) {
		writeErr(w, http.StatusTooManyRequests, "túl sok próbálkozás")
		return
	}
	if c.ProfileLock > 0 {
		writeErr(w, http.StatusForbidden, "ezen az eszközön nem lehet jelszót módosítani")
		return
	}
	if !s.st.CheckUserPassword(c.User, in.Current) {
		writeErr(w, http.StatusBadRequest, "a jelenlegi jelszó hibás")
		return
	}
	if msg := validCredentials(c.User.Username, in.New); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.st.SetPassword(c.User.ID, in.New, c.Token); err != nil {
		s.internalErr(w, r, err)
		return
	}
	ok(w)
}

// ------------------------------------------------------------------ profiles

// ownProfile loads a profile of the current account and enforces edit rules:
// kids profiles cannot change anything, PIN-protected profiles can only be
// edited while selected.
func (s *Server) editableProfile(w http.ResponseWriter, c *reqCtx, id int64) *store.Profile {
	if (c.Profile != nil && c.Profile.IsKids) || c.ProfileLock > 0 {
		writeErr(w, http.StatusForbidden, "gyerekprofilból nem lehet profilt módosítani")
		return nil
	}
	p, err := s.st.Profile(id)
	if err != nil || p.UserID != c.User.ID {
		writeErr(w, http.StatusNotFound, "a profil nem található")
		return nil
	}
	if p.HasPIN && (c.Profile == nil || c.Profile.ID != p.ID) {
		writeErr(w, http.StatusForbidden, "a PIN-nel védett profilt csak belépve lehet szerkeszteni")
		return nil
	}
	return p
}

func (s *Server) handleProfileCreate(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	if (c.Profile != nil && c.Profile.IsKids) || c.ProfileLock > 0 {
		writeErr(w, http.StatusForbidden, "gyerekprofilból nem lehet profilt létrehozni")
		return
	}
	var in store.ProfileInput
	if !decode(w, r, &in) {
		return
	}
	if err := in.Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	p, err := s.st.CreateProfile(c.User.ID, in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handleProfileUpdate(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	p := s.editableProfile(w, c, pathID(r, "id"))
	if p == nil {
		return
	}
	var in store.ProfileInput
	if !decode(w, r, &in) {
		return
	}
	if err := in.Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.st.UpdateProfile(p.ID, in); err != nil {
		s.internalErr(w, r, err)
		return
	}
	p, _ = s.st.Profile(p.ID)
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleProfileDelete(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	p := s.editableProfile(w, c, pathID(r, "id"))
	if p == nil {
		return
	}
	if err := s.st.DeleteProfile(p); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ok(w)
}

func (s *Server) handleProfileSelect(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in struct {
		PIN string `json:"pin"`
	}
	if !decode(w, r, &in) {
		return
	}
	p, err := s.st.Profile(pathID(r, "id"))
	if err != nil || p.UserID != c.User.ID || (c.ProfileLock > 0 && p.ID != c.ProfileLock) {
		writeErr(w, http.StatusNotFound, "a profil nem található")
		return
	}
	if p.HasPIN {
		if !s.limiter.allow("pin:"+clientIP(r)+":"+r.PathValue("id"), 5, 5*time.Minute) ||
			!s.limiter.allow("pin-profile:"+r.PathValue("id"), 15, 5*time.Minute) {
			writeErr(w, http.StatusTooManyRequests, "túl sok hibás PIN, várj pár percet")
			return
		}
		if !p.CheckPIN(in.PIN) {
			writeErr(w, http.StatusForbidden, "hibás PIN")
			return
		}
	}
	if err := s.st.SetSessionProfile(c.Token, p.ID); err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleProfileLeave(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	if c.ProfileLock > 0 {
		writeErr(w, http.StatusForbidden, "ez az eszköz egy profilhoz van kötve")
		return
	}
	if err := s.st.SetSessionProfile(c.Token, 0); err != nil {
		s.internalErr(w, r, err)
		return
	}
	ok(w)
}
