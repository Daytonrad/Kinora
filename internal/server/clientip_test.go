package server

import (
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	for _, tc := range []struct {
		peer, xff, want string
	}{
		{"203.0.113.9:4000", "1.2.3.4", "203.0.113.9"},                         // a public peer cannot claim another address
		{"127.0.0.1:4000", "", "127.0.0.1"},                                    // no proxy header
		{"172.18.0.3:4000", "198.51.100.7", "198.51.100.7"},                    // a proxy on the Docker network
		{"172.18.0.3:4000", "1.2.3.4, 198.51.100.7", "198.51.100.7"},           // a forged entry left of the proxy's own one
		{"172.18.0.3:4000", "1.2.3.4, 198.51.100.7, 10.0.0.2", "198.51.100.7"}, // two proxies in a row
		{"10.0.0.2:4000", "192.168.1.20", "192.168.1.20"},                      // a LAN client behind a LAN proxy
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = tc.peer
		if tc.xff != "" {
			r.Header.Set("X-Forwarded-For", tc.xff)
		}
		if got := clientIP(r); got != tc.want {
			t.Errorf("peer %s xff %q: got %s, want %s", tc.peer, tc.xff, got, tc.want)
		}
	}
}
