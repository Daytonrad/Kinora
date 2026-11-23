package server

import (
	"encoding/json"
	"fmt"
	"net"

	"github.com/ScotyLucas/kinora/internal/jfcompat"
	"github.com/ScotyLucas/kinora/internal/store"
)

// The Jellyfin-compatible API (internal/jfcompat) runs on its own port; this file connects it to the server:
// Quick Connect codes approved in Kinora, and the LAN discovery answer for Jellyfin clients.

// JFCompatQC returns Kinora's Quick Connect as the Jellyfin API sees it.
func (s *Server) JFCompatQC() jfcompat.QuickConnect { return jfQC{s} }

type jfQC struct{ s *Server }

func (q jfQC) Enabled() bool { return q.s.qcEnabled() }

func (q jfQC) Initiate(d store.DeviceInfo) (string, string, error) {
	e, secret, err := q.s.qc.initiate(d)
	if err != nil {
		return "", "", err
	}
	return e.Code, secret, nil
}

func (q jfQC) State(secret string) string {
	q.s.qc.mu.Lock()
	defer q.s.qc.mu.Unlock()
	e := q.s.qc.bySecret[sha(secret)]
	if e == nil {
		return "expired"
	}
	switch e.Status {
	case "approved":
		return "approved"
	case "denied":
		return "denied"
	case "pending":
		return "pending"
	}
	return "expired"
}

// Claim consumes an approved code exactly once.
func (q jfQC) Claim(secret string) (int64, int64, bool) {
	e, found := q.s.qc.poll(secret)
	if !found || e.Status != "approved" {
		if found && e.Status == "issuing" {
			q.s.qc.release(secret)
		}
		return 0, 0, false
	}
	q.s.qc.issue(secret, map[string]any{"status": "approved"})
	q.s.log.Info("quick connect approved (Jellyfin-compatible client)", "device", e.Device.Name, "by", e.ApprovedBy)
	return e.UserID, e.ProfileID, true
}

// jfApproveProfile decides the profile of a Jellyfin client's Quick Connect: one must be chosen (the client
// cannot pick or enter a PIN later), and a profile with a PIN needs it here.
func (s *Server) jfApproveProfile(c *reqCtx, profileID int64, pin string) (int64, error) {
	var p *store.Profile
	if profileID > 0 {
		x, err := s.st.Profile(profileID)
		if err != nil || x.UserID != c.User.ID {
			return 0, errProfileNotFound
		}
		p = x
	} else {
		return 0, errProfileRequired
	}
	if p.HasPIN && !p.CheckPIN(pin) {
		return 0, errWrongPIN
	}
	return p.ID, nil
}

var (
	errProfileNotFound = fmt.Errorf("a profil nem található")
	errProfileRequired = fmt.Errorf("válassz profilt ehhez az eszközhöz")
	errWrongPIN        = fmt.Errorf("hibás PIN")
)

// JFCompatAddr is set by main when the Jellyfin-compatible listener runs (":8097"); empty otherwise.
var JFCompatAddr string

// JFCompatServerID is the server id the Jellyfin-compatible API reports (set by main).
var JFCompatServerID func() string

// jfDiscoveryReply answers Jellyfin clients' "who is JellyfinServer?" probe with the compatible port.
func (s *Server) jfDiscoveryReply(peer *net.UDPAddr) []byte {
	_, port, err := net.SplitHostPort(JFCompatAddr)
	if err != nil || port == "" {
		port = "8097"
	}
	ip := localIPFor(peer)
	if ip == "" {
		return nil
	}
	name := s.st.Setting("server_name")
	if name == "" {
		name = "Kinora"
	}
	id := ""
	if JFCompatServerID != nil {
		id = JFCompatServerID()
	}
	b, _ := json.Marshal(map[string]any{"Address": "http://" + net.JoinHostPort(ip, port), "Id": id, "Name": name, "EndpointAddress": nil})
	return b
}

// jfDiscoveryQuery is the probe Jellyfin clients broadcast.
const jfDiscoveryQuery = "who is JellyfinServer?"
