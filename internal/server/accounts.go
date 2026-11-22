package server

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ScotyLucas/kinora/internal/store"
)

// Remembered accounts: a browser keeps the sessions of everyone who signed in on it, so the
// profile picker can switch between them with one tap instead of a new sign-in each time.
// The active session stays in kinora_session; the others wait in kinora_accounts (HttpOnly,
// the same protection as the session cookie). Native apps keep their tokens themselves.

const (
	accountsCookie = "kinora_accounts"
	maxAccounts    = 8
)

// rememberedTokens returns the parked session tokens of this browser.
func rememberedTokens(r *http.Request) []string {
	c, err := r.Cookie(accountsCookie)
	if err != nil {
		return nil
	}
	var out []string
	for _, t := range strings.Split(c.Value, ".") {
		if t = strings.TrimSpace(t); t != "" && len(out) < maxAccounts {
			out = append(out, t)
		}
	}
	return out
}

func setAccountsCookie(w http.ResponseWriter, r *http.Request, toks []string) {
	if len(toks) > maxAccounts {
		toks = toks[:maxAccounts]
	}
	ck := &http.Cookie{Name: accountsCookie, Value: strings.Join(toks, "."), Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, Secure: isHTTPS(r), MaxAge: int(store.SessionTTL.Seconds())}
	if len(toks) == 0 {
		ck.Value, ck.MaxAge = "", -1
	}
	http.SetCookie(w, ck)
}

// parkedAccount is a remembered session that still works.
type parkedAccount struct {
	token string
	state *store.SessionState
}

// liveAccounts resolves the parked tokens: expired ones, duplicates and the active user drop out.
// changed reports whether the cookie should be rewritten.
func (s *Server) liveAccounts(r *http.Request, activeUser int64) (live []parkedAccount, changed bool) {
	seen := map[int64]bool{activeUser: true}
	toks := rememberedTokens(r)
	for _, t := range toks {
		ss, err := s.st.SessionFull(t, "")
		if err != nil || ss.ProfileLock > 0 || seen[ss.User.ID] {
			if err == nil && seen[ss.User.ID] {
				s.st.DeleteSession(t) // the same account signed in again: the old session is spare
			}
			changed = true
			continue
		}
		seen[ss.User.ID] = true
		live = append(live, parkedAccount{t, ss})
	}
	return live, changed
}

func tokensOf(list []parkedAccount) []string {
	out := make([]string, 0, len(list))
	for _, a := range list {
		out = append(out, a.token)
	}
	return out
}

// rememberActive parks the browser's current session before newTok replaces it, when it belongs
// to someone else; a session of the same account is just ended. Kids devices (profile-locked
// sessions) never take part.
func (s *Server) rememberActive(w http.ResponseWriter, r *http.Request, newTok string) {
	old := ""
	if c, err := r.Cookie(cookieName); err == nil {
		old = c.Value
	}
	if old == "" || old == newTok {
		return
	}
	cur, err := s.st.SessionFull(newTok, "")
	if err != nil {
		return
	}
	prev, err := s.st.SessionFull(old, "")
	if err != nil {
		return
	}
	live, changed := s.liveAccounts(r, cur.User.ID)
	switch {
	case cur.ProfileLock > 0 || prev.ProfileLock > 0:
		// keep the cookie list as it is
	case prev.User.ID == cur.User.ID:
		s.st.DeleteSession(old)
	default:
		live = append([]parkedAccount{{old, prev}}, live...)
		changed = true
	}
	if changed {
		setAccountsCookie(w, r, tokensOf(live))
	}
}

type accountProfile struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Color  string `json:"color"`
	HasPIN bool   `json:"has_pin"`
	IsKids bool   `json:"is_kids"`
}

type accountOut struct {
	UserID   int64            `json:"user_id"`
	Username string           `json:"username"`
	Current  bool             `json:"current"`
	Profiles []accountProfile `json:"profiles"`
}

func (s *Server) accountOf(u *store.User, current bool) (accountOut, error) {
	ps, err := s.st.Profiles(u.ID)
	if err != nil {
		return accountOut{}, err
	}
	a := accountOut{UserID: u.ID, Username: u.Username, Current: current, Profiles: []accountProfile{}}
	for _, p := range ps {
		a.Profiles = append(a.Profiles, accountProfile{p.ID, p.Name, p.Color, p.HasPIN, p.IsKids})
	}
	return a, nil
}

// GET /api/accounts – the signed-in account first, then the others remembered on this browser.
func (s *Server) handleAccounts(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	cur, err := s.accountOf(c.User, true)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	out := []accountOut{cur}
	if c.ProfileLock == 0 {
		live, changed := s.liveAccounts(r, c.User.ID)
		if changed {
			setAccountsCookie(w, r, tokensOf(live))
		}
		for _, a := range live {
			o, err := s.accountOf(a.state.User, false)
			if err != nil {
				s.internalErr(w, r, err)
				return
			}
			out = append(out, o)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"accounts": out})
}

// POST /api/accounts/{id}/switch {profile_id, pin} – makes a remembered account the active one
// (and picks its profile, PIN rules as on the profile picker). The current account is remembered.
func (s *Server) handleAccountSwitch(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in struct {
		ProfileID int64  `json:"profile_id"`
		PIN       string `json:"pin"`
	}
	if !decode(w, r, &in) {
		return
	}
	if c.ProfileLock > 0 {
		writeErr(w, http.StatusForbidden, "ez az eszköz egy profilhoz van kötve")
		return
	}
	uid, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	live, _ := s.liveAccounts(r, c.User.ID)
	idx := -1
	for i, a := range live {
		if a.state.User.ID == uid {
			idx = i
		}
	}
	if idx < 0 {
		writeErr(w, http.StatusNotFound, "ez a fiók már nincs megjegyezve ezen az eszközön, lépj be újra")
		return
	}
	target := live[idx]
	if in.ProfileID > 0 {
		p, err := s.st.Profile(in.ProfileID)
		if err != nil || p.UserID != uid {
			writeErr(w, http.StatusNotFound, "a profil nem található")
			return
		}
		if p.HasPIN {
			if !s.limiter.allow("pin:"+clientIP(r)+":"+strconv.FormatInt(p.ID, 10), 5, 5*time.Minute) ||
				!s.limiter.allow("pin-profile:"+strconv.FormatInt(p.ID, 10), 15, 5*time.Minute) {
				writeErr(w, http.StatusTooManyRequests, "túl sok hibás PIN, várj pár percet")
				return
			}
			if !p.CheckPIN(in.PIN) {
				writeErr(w, http.StatusForbidden, "hibás PIN")
				return
			}
		}
		if err := s.st.SetSessionProfile(target.token, p.ID); err != nil {
			s.internalErr(w, r, err)
			return
		}
	} else if err := s.st.SetSessionProfile(target.token, 0); err != nil {
		s.internalErr(w, r, err)
		return
	}
	rest := append([]parkedAccount{{c.Token, nil}}, append(live[:idx:idx], live[idx+1:]...)...)
	setAccountsCookie(w, r, tokensOf(rest))
	s.setSessionCookie(w, r, target.token)
	ok(w)
}

// DELETE /api/accounts/{id} – signs a remembered account out of this browser.
func (s *Server) handleAccountForget(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	uid, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	live, changed := s.liveAccounts(r, c.User.ID)
	keep := live[:0:0]
	for _, a := range live {
		if a.state.User.ID == uid {
			s.st.DeleteSession(a.token)
			changed = true
			continue
		}
		keep = append(keep, a)
	}
	if changed {
		setAccountsCookie(w, r, tokensOf(keep))
	}
	ok(w)
}

// afterLogout makes the next remembered account active once the current one signed out,
// so the others on this browser stay one tap away. Reports whether one took over.
func (s *Server) afterLogout(w http.ResponseWriter, r *http.Request) bool {
	live, changed := s.liveAccounts(r, 0)
	if len(live) == 0 {
		if changed || len(rememberedTokens(r)) > 0 {
			setAccountsCookie(w, r, nil)
		}
		return false
	}
	next := live[0]
	s.st.SetSessionProfile(next.token, 0)
	s.setSessionCookie(w, r, next.token)
	setAccountsCookie(w, r, tokensOf(live[1:]))
	return true
}
