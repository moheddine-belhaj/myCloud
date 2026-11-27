package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	tests := []struct {
		name, header, remote, want string
	}{
		{"cloudflare ipv4", "203.0.113.7", "10.42.0.5:4321", "203.0.113.7"},
		{"cloudflare ipv6 is grouped by /64", "2001:db8:1:2:aaaa:bbbb:cccc:dddd", "10.42.0.5:4321", "2001:db8:1:2::/64"},
		{"ipv4-mapped ipv6", "::ffff:203.0.113.7", "10.42.0.5:4321", "203.0.113.7"},
		{"invalid header falls back to peer", "not-an-ip", "10.42.0.5:4321", "10.42.0.5"},
		{"no header uses peer", "", "192.0.2.1:1234", "192.0.2.1"},
		{"nothing usable", "", "garbage", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.remote
			if tc.header != "" {
				r.Header.Set(HeaderClientIP, tc.header)
			}
			if got := ClientIP(r); got != tc.want {
				t.Errorf("ClientIP = %q, want %q", got, tc.want)
			}
		})
	}
}
