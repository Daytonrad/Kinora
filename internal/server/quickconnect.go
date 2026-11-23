package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ScotyLucas/kinora/internal/jfcompat"
	"github.com/ScotyLucas/kinora/internal/store"
)

// Quick Connect: a TV (or phone) asks for a 6-digit code, shows it, and polls
// with a secret. A user who is already logged in elsewhere types the code,
// picks a profile and approves; the next poll returns a fresh device token.
// Codes live only in memory: they are short lived (5 minutes) and single use.

const (
	qcTTL        = 5 * time.Minute
	qcMaxPending = 2000
	qcAttempts   = 10 // code entries per IP and per user ...
	qcWindow     = 5 * time.Minute
)

type qcEntry struct {
	Code       string
	secretHash string
	Device     store.DeviceInfo
	Expires    time.Time
	Status     string // pending | approved | denied
	UserID     int64
	ProfileID  int64
	Lock       bool
	NeedsPIN   bool
	ApprovedBy string
	issued     map[string]any // the approved response, re-sent if the first answer got lost
}

type quickConnect struct {
	mu       sync.Mutex
	byCode   map[string]*qcEntry
	bySecret map[string]*qcEntry
}

func newQuickConnect() *quickConnect {
	return &quickConnect{byCode: map[string]*qcEntry{}, bySecret: map[string]*qcEntry{}}
}

func sha(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

func randomDigits(n int) string {
	max := big.NewInt(1)
	for i := 0; i < n; i++ {
		max.Mul(max, big.NewInt(10))
	}
	v, _ := rand.Int(rand.Reader, max)
	return fmt.Sprintf("%0*d", n, v)
}

func (q *quickConnect) purgeLocked(now time.Time) {
	for code, e := range q.byCode {
		if now.After(e.Expires) {
			delete(q.byCode, code)
			delete(q.bySecret, e.secretHash)
		}
	}
}

func (q *quickConnect) purge() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.purgeLocked(time.Now())
}

// initiate creates a pending code; codes never collide with an active one.
func (q *quickConnect) initiate(d store.DeviceInfo) (*qcEntry, string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := time.Now()
	q.purgeLocked(now)
	if len(q.byCode) >= qcMaxPending {
		return nil, "", fmt.Errorf("túl sok függő belépési kód, próbáld újra később")
	}
	var code string
	for {
		code = randomDigits(6)
		if _, taken := q.byCode[code]; !taken {
			break
		}
	}
	secret := store.RandomToken(32)
	e := &qcEntry{Code: code, secretHash: sha(secret), Device: d, Expires: now.Add(qcTTL), Status: "pending"}
	q.byCode[code] = e
	q.bySecret[e.secretHash] = e
	return e, secret, nil
}

// pending returns a live, still pending entry by code.
func (q *quickConnect) pending(code string) *qcEntry {
	q.mu.Lock()
	defer q.mu.Unlock()
	e := q.byCode[code]
	if e == nil || time.Now().After(e.Expires) || e.Status != "pending" {
		return nil
	}
	cp := *e
	return &cp
}

// decide approves or denies a pending code (exactly once).
func (q *quickConnect) decide(code string, approve bool, userID, profileID int64, lock, needsPIN bool, by string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	e := q.byCode[code]
	if e == nil || time.Now().After(e.Expires) || e.Status != "pending" {
		return false
	}
	if approve {
		e.Status, e.UserID, e.ProfileID, e.Lock, e.NeedsPIN, e.ApprovedBy = "approved", userID, profileID, lock, needsPIN, by
	} else {
		e.Status = "denied"
	}
	return true
}

// poll returns the entry for a secret. A denied entry is removed by the first poll
// that sees it. An approved one is claimed ("issuing") so concurrent polls cannot
// create two sessions; issue() or release() finishes the claim.
func (q *quickConnect) poll(secret string) (*qcEntry, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	h := sha(secret)
	e := q.bySecret[h]
	if e == nil {
		return nil, false
	}
	if time.Now().After(e.Expires) {
		delete(q.byCode, e.Code)
		delete(q.bySecret, h)
		return nil, false
	}
	cp := *e
	switch e.Status {
	case "denied":
		delete(q.byCode, e.Code)
		delete(q.bySecret, h)
	case "approved":
		e.Status = "issuing"
	case "issuing":
		cp.Status = "pending" // another poll is creating the session right now
	}
	return &cp, true
}

// issue stores the approved response for a short grace period: if the device never
// received it (timeout, flaky Wi-Fi), the next poll with the same secret gets it again.
func (q *quickConnect) issue(secret string, resp map[string]any) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if e := q.bySecret[sha(secret)]; e != nil {
		e.Status, e.issued = "issued", resp
		delete(q.byCode, e.Code) // the code can no longer be approved or looked up
		if g := time.Now().Add(60 * time.Second); g.Before(e.Expires) {
			e.Expires = g
		}
	}
}

// release undoes a claim when creating the session failed, so the device can retry.
func (q *quickConnect) release(secret string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if e := q.bySecret[sha(secret)]; e != nil && e.Status == "issuing" {
		e.Status = "approved"
	}
}

func (s *Server) qcEnabled() bool { return s.st.Setting("quickconnect_enabled") != "0" }

func (s *Server) handleQCInitiate(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	if !s.qcEnabled() {
		writeErr(w, http.StatusForbidden, "a belépés kóddal (Quick Connect) ki van kapcsolva ezen a szerveren")
		return
	}
	var d store.DeviceInfo
	if !decode(w, r, &d) {
		return
	}
	// per client IP, and per TCP peer (X-Forwarded-For can be forged from the LAN)
	if !s.limiter.allow("qcinit:"+clientIP(r), 30, qcWindow) || !s.limiter.allow("qcinit-peer:"+peerIP(r), 300, qcWindow) {
		writeErr(w, http.StatusTooManyRequests, "túl sok kódkérés, próbáld újra pár perc múlva")
		return
	}
	e, secret, err := s.qc.initiate(deviceFromRequest(r, d)) // a browser gets a name from its user agent
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code": e.Code, "secret": secret, "expires_in": int(qcTTL.Seconds()), "poll_interval": 2,
		"approve_path": "/#/quickconnect?code=" + e.Code,
	})
}

func (s *Server) handleQCStatus(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	secret := r.URL.Query().Get("secret")
	if secret == "" {
		secret = r.Header.Get("X-QuickConnect-Secret")
	}
	if !s.limiter.allow("qcpoll:"+clientIP(r), 240, time.Minute) {
		writeErr(w, http.StatusTooManyRequests, "túl gyakori lekérdezés")
		return
	}
	e, found := s.qc.poll(secret)
	if !found {
		writeJSON(w, http.StatusGone, map[string]any{"status": "expired", "error": "a kód lejárt, kérj újat"})
		return
	}
	switch e.Status {
	case "issued":
		if tok, _ := e.issued["token"].(string); tok != "" {
			s.webSessionCookie(w, r, tok) // the web login page signs in with the cookie
		}
		writeJSON(w, http.StatusOK, e.issued)
	case "pending":
		writeJSON(w, http.StatusOK, map[string]any{"status": "pending", "expires_in": int(time.Until(e.Expires).Seconds())})
	case "denied":
		writeJSON(w, http.StatusOK, map[string]any{"status": "denied", "error": "a csatlakozást elutasították"})
	case "approved":
		u, err := s.st.UserByID(e.UserID)
		if err != nil {
			s.qc.release(secret)
			writeJSON(w, http.StatusGone, map[string]any{"status": "expired", "error": "a fiók már nem létezik"})
			return
		}
		pre, lock := e.ProfileID, int64(0)
		if e.NeedsPIN {
			pre = 0 // the PIN is entered on the new device
		}
		if e.Lock {
			lock = e.ProfileID
		}
		d := e.Device
		d.IP = clientIP(r)
		tok, dev, err := s.st.CreateDeviceSession(u.ID, d, pre, lock)
		if err != nil {
			s.qc.release(secret)
			s.internalErr(w, r, err)
			return
		}
		s.log.Info("quick connect approved", "user", u.Username, "device", d.Name, "platform", d.Platform, "by", e.ApprovedBy)
		resp := map[string]any{
			"status": "approved", "token": tok, "device_id": dev, "user": u,
			"profile_id": e.ProfileID, "needs_pin": e.NeedsPIN, "profile_locked": e.Lock,
		}
		s.qc.issue(secret, resp)
		s.webSessionCookie(w, r, tok)
		writeJSON(w, http.StatusOK, resp)
	}
}

// qcGuard runs the shared checks of code lookup/approval: feature on, rate
// limits per IP and per user, no kids or profile-locked session.
func (s *Server) qcGuard(w http.ResponseWriter, r *http.Request, c *reqCtx) bool {
	if !s.qcEnabled() {
		writeErr(w, http.StatusForbidden, "a belépés kóddal (Quick Connect) ki van kapcsolva ezen a szerveren")
		return false
	}
	if (c.Profile != nil && c.Profile.IsKids) || c.ProfileLock > 0 {
		writeErr(w, http.StatusForbidden, "gyerekprofilból nem lehet eszközt csatlakoztatni")
		return false
	}
	// only wrong codes use up the budget (see qcFailed), so normal approvals never hit it
	if s.limiter.count("qccode:ip:"+clientIP(r), qcWindow) >= qcAttempts ||
		s.limiter.count(fmt.Sprintf("qccode:user:%d", c.User.ID), qcWindow) >= qcAttempts {
		writeErr(w, http.StatusTooManyRequests, "túl sok kódbeírási kísérlet, várj pár percet")
		return false
	}
	return true
}

// qcFailed records a wrong / expired code against the IP and the user.
func (s *Server) qcFailed(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	s.limiter.allow("qccode:ip:"+clientIP(r), qcAttempts+1, qcWindow)
	s.limiter.allow(fmt.Sprintf("qccode:user:%d", c.User.ID), qcAttempts+1, qcWindow)
	writeErr(w, http.StatusNotFound, "ismeretlen vagy lejárt kód")
}

func normCode(code string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, code)
}

func (s *Server) handleQCLookup(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &in) || !s.qcGuard(w, r, c) {
		return
	}
	e := s.qc.pending(normCode(in.Code))
	if e == nil {
		s.qcFailed(w, r, c)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code": e.Code, "device_name": e.Device.Name, "platform": e.Device.Platform, "app_version": e.Device.AppVersion,
		"ip": e.Device.IP, "expires_in": int(time.Until(e.Expires).Seconds()),
	})
}

func (s *Server) handleQCApprove(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	var in struct {
		Code      string `json:"code"`
		ProfileID int64  `json:"profile_id"`
		Action    string `json:"action"` // approve (default) | deny
		PIN       string `json:"pin"`    // a Jellyfin-compatible client's PIN profile: entered here
	}
	if !decode(w, r, &in) || !s.qcGuard(w, r, c) {
		return
	}
	code := normCode(in.Code)
	if in.Action == "deny" {
		if !s.qc.decide(code, false, 0, 0, false, false, c.User.Username) {
			s.qcFailed(w, r, c)
			return
		}
		ok(w)
		return
	}
	var lock, needsPIN bool
	if e := s.qc.pending(code); e != nil && e.Device.Platform == jfcompat.Platform {
		// the Jellyfin client signs in as exactly this profile and has no way to ask for its PIN
		pid, err := s.jfApproveProfile(c, in.ProfileID, in.PIN)
		if err != nil {
			if err == errWrongPIN { // counts like a wrong code: the PIN cannot be tried through here either
				s.limiter.allow("qccode:ip:"+clientIP(r), qcAttempts+1, qcWindow)
				s.limiter.allow(fmt.Sprintf("qccode:user:%d", c.User.ID), qcAttempts+1, qcWindow)
				writeErr(w, http.StatusForbidden, err.Error())
				return
			}
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if !s.qc.decide(code, true, c.User.ID, pid, true, false, c.User.Username) {
			s.qcFailed(w, r, c)
			return
		}
		ok(w)
		return
	}
	if in.ProfileID > 0 {
		p, err := s.st.Profile(in.ProfileID)
		if err != nil || p.UserID != c.User.ID { // only into your own account
			writeErr(w, http.StatusNotFound, "a profil nem található")
			return
		}
		lock, needsPIN = p.IsKids, p.HasPIN
	}
	if !s.qc.decide(code, true, c.User.ID, in.ProfileID, lock, needsPIN, c.User.Username) {
		s.qcFailed(w, r, c)
		return
	}
	ok(w)
}
