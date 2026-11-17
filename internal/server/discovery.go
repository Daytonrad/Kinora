package server

import (
	"context"
	"encoding/json"
	"net"
	"strconv"
	"strings"
	"time"
)

// LAN discovery: apps broadcast "who is KinoraServer?" to UDP port 7359 and
// every Kinora server on the network answers with a JSON description. Inside
// Docker this needs network_mode: host (broadcasts do not cross the bridge),
// or the advertised address must be overridden with KINORA_PUBLIC_URL.

// DiscoveryQuery is the probe message apps send.
const DiscoveryQuery = "who is KinoraServer?"

// DefaultDiscoveryPort is the UDP port apps probe.
const DefaultDiscoveryPort = 7359

type discoveryReply struct {
	Product string `json:"product"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
	Public  string `json:"public_url,omitempty"`
	Version string `json:"version"`
}

func (s *Server) httpPort() int {
	_, p, err := net.SplitHostPort(s.cfg.Addr)
	if n, _ := strconv.Atoi(p); err == nil && n > 0 {
		return n
	}
	return 8096
}

// localIPFor returns the local address used to reach peer (the interface the
// probe came in on), via a connected UDP socket (no packet is sent).
func localIPFor(peer *net.UDPAddr) string {
	c, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: peer.IP, Port: 9})
	if err != nil {
		return ""
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).IP.String()
}

func (s *Server) discoveryReply(peer *net.UDPAddr) []byte {
	ip := localIPFor(peer)
	addr := ""
	if ip != "" {
		addr = "http://" + net.JoinHostPort(ip, strconv.Itoa(s.httpPort()))
	}
	b, _ := json.Marshal(discoveryReply{Product: "kinora", ID: s.serverID(), Name: s.st.Setting("server_name"),
		Address: addr, Public: s.st.Setting("public_url"), Version: Version})
	return b
}

// RunDiscovery answers discovery probes until ctx is done. It is a no-op when
// the port is 0; the admin switch (discovery_enabled) is read per packet.
func (s *Server) RunDiscovery(ctx context.Context) {
	port := s.cfg.DiscoveryPort
	if port <= 0 {
		return
	}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: port})
	if err != nil {
		s.log.Warn("LAN discovery unavailable", "port", port, "err", err)
		return
	}
	s.log.Info("LAN discovery listening", "udp", port)
	go func() { <-ctx.Done(); conn.Close() }()
	buf := make([]byte, 512)
	for {
		n, peer, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if s.st.Setting("discovery_enabled") == "0" {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(string(buf[:n])), DiscoveryQuery) {
			continue
		}
		if !s.limiter.allow("disc:"+peer.IP.String(), 30, time.Minute) {
			continue
		}
		conn.WriteToUDP(s.discoveryReply(peer), peer)
	}
}
