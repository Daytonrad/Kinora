package server

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/ScotyLucas/kinora/internal/push"
)

// handles are 32 random bytes, base64url without padding
var reHandle = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

// pushSession refuses requests that cannot own a pairing (stream links, no device).
func pushSession(w http.ResponseWriter, c *reqCtx) bool {
	if c.ViaStreamToken || c.DeviceID == "" {
		writeErr(w, http.StatusForbidden, "ehhez bejelentkezett eszköz kell")
		return false
	}
	return true
}

// handlePushIOS tells the app whether push is on and which relay and server id to
// register with. The server registers with the relay on first use.
func (s *Server) handlePushIOS(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	out := map[string]any{"enabled": false}
	if !s.Push.Relay.Enabled() {
		writeJSON(w, http.StatusOK, out)
		return
	}
	id, err := s.Push.Relay.ServerID(r.Context())
	if err != nil {
		s.log.Warn("push: relay registration", "err", err)
		writeErr(w, http.StatusBadGateway, "a push-közvetítő nem érhető el")
		return
	}
	out["enabled"], out["relay_url"], out["server_id"] = true, s.Push.Relay.URL(), id
	if c.DeviceID != "" {
		p, _ := s.st.PushIOSFor(c.DeviceID)
		out["paired"] = p != nil
	}
	writeJSON(w, http.StatusOK, out)
}

// handlePushIOSPair stores the handle the relay gave the app and the key the app made.
func (s *Server) handlePushIOSPair(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	if !pushSession(w, c) {
		return
	}
	if !s.Push.Relay.Enabled() {
		writeErr(w, http.StatusConflict, "a push nincs bekapcsolva ezen a szerveren")
		return
	}
	var in struct {
		Handle string `json:"handle"`
		Key    string `json:"key"`
	}
	if !decode(w, r, &in) {
		return
	}
	key, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(in.Key, "="))
	if err != nil {
		key, err = base64.StdEncoding.DecodeString(in.Key)
	}
	if !reHandle.MatchString(in.Handle) || err != nil || len(key) != 32 {
		writeErr(w, http.StatusBadRequest, "hibás handle vagy kulcs")
		return
	}
	old, _ := s.st.PushIOSFor(c.DeviceID)
	if err := s.st.SetPushIOS(c.DeviceID, c.User.ID, in.Handle, key); err != nil {
		s.internalErr(w, r, err)
		return
	}
	if old != nil && old.Handle != in.Handle {
		s.forgetHandle(old.Handle)
	}
	ok(w)
}

// handlePushIOSUnpair stops pushing to this device.
func (s *Server) handlePushIOSUnpair(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	if !pushSession(w, c) {
		return
	}
	h, err := s.st.DeletePushIOS(c.DeviceID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if h != "" {
		s.forgetHandle(h)
	}
	ok(w)
}

// handlePushIOSTest sends a test notification to this device.
func (s *Server) handlePushIOSTest(w http.ResponseWriter, r *http.Request, c *reqCtx) {
	if !pushSession(w, c) {
		return
	}
	p, err := s.st.PushIOSFor(c.DeviceID)
	if err != nil {
		s.internalErr(w, r, err)
		return
	}
	if p == nil {
		writeErr(w, http.StatusNotFound, "ez az eszköz nincs párosítva")
		return
	}
	err = s.Push.SendDevice(r.Context(), *p, push.Message{V: 1, Type: "test", Title: s.st.Setting("server_name"),
		Body: "Az értesítések működnek.", Tag: "test"})
	var re *push.RelayError
	switch {
	case err == nil:
		if still, _ := s.st.PushIOSFor(c.DeviceID); still == nil {
			writeErr(w, http.StatusGone, "a közvetítő már nem ismeri ezt az eszközt, párosítsd újra")
			return
		}
		ok(w)
	case errors.Is(err, push.ErrDisabled):
		writeErr(w, http.StatusConflict, "a push nincs bekapcsolva ezen a szerveren")
	case errors.As(err, &re) && re.Status == http.StatusTooManyRequests:
		writeErr(w, http.StatusTooManyRequests, "túl sok értesítés, próbáld később")
	default:
		s.log.Warn("push: test", "err", err)
		writeErr(w, http.StatusBadGateway, "a push-közvetítő nem fogadta az értesítést")
	}
}

// forgetHandle tells the relay to drop a handle, in the background.
func (s *Server) forgetHandle(h string) {
	go func() {
		ctx, cancel := context.WithTimeout(s.bgCtx, 20*time.Second)
		defer cancel()
		if err := s.Push.Relay.Delete(ctx, h); err != nil && !errors.Is(err, push.ErrGone) && !errors.Is(err, push.ErrDisabled) {
			s.log.Warn("push: relay delete", "err", err)
		}
	}()
}
