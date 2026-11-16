package server

import (
	"errors"
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
		"setup_needed": n == 0,
		"version":      Version,
		"server_name":  s.st.Setting("server_name"),
		"logged_in":    c.User != nil,
	})
}

type credentials struct {
	ServerName string `json:"server_name"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	Code       string `json:"code"`
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
	s.log.Info("setup complete", "admin", u.Username)
	s.startSession(w, r, u)
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, u *store.User) {
	tok, err := s.st.CreateSession(u.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	s.setSessionCookie(w, r, tok)
	writeJSON(w, http.StatusOK, map[string]any{"token": tok, "user": u})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in credentials
	if !decode(w, r, &in) {
		return
	}
	if !s.limiter.allow("login:"+clientIP(r), 10, 5*time.Minute) {
		writeErr(w, http.StatusTooManyRequests, "túl sok bejelentkezési kísérlet, próbáld újra pár perc múlva")
		return
	}
	u, err := s.st.Authenticate(strings.TrimSpace(in.Username), in.Password)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "hibás felhasználónév vagy jelszó")
		return
	}
	s.startSession(w, r, u)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	if c.Token != "" {
		s.st.DeleteSession(c.Token)
	}
	clearSessionCookie(w, r)
	ok(w)
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
	s.startSession(w, r, u)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	profiles, err := s.st.Profiles(c.User.ID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":        c.User,
		"profile":     c.Profile,
		"profiles":    profiles,
		"server_name": s.st.Setting("server_name"),
		"version":     Version,
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
	if !s.limiter.allow("login:"+clientIP(r), 10, 5*time.Minute) {
		writeErr(w, http.StatusTooManyRequests, "túl sok próbálkozás")
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
	if c.Profile != nil && c.Profile.IsKids {
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
	if c.Profile != nil && c.Profile.IsKids {
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
	if err != nil || p.UserID != c.User.ID {
		writeErr(w, http.StatusNotFound, "a profil nem található")
		return
	}
	if p.HasPIN {
		if !s.limiter.allow("pin:"+clientIP(r)+":"+r.PathValue("id"), 5, 5*time.Minute) {
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
	if err := s.st.SetSessionProfile(c.Token, 0); err != nil {
		s.internalErr(w, r, err)
		return
	}
	ok(w)
}
