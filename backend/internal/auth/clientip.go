package auth

import (
	"net"
	"net/http"
	"net/netip"
)

// HeaderClientIP is set by Cloudflare to the real client address. See README.md#rate-limiting.
const HeaderClientIP = "CF-Connecting-IP"

// ClientIP returns the caller's address for rate limiting: CF-Connecting-IP if valid,
// otherwise the TCP peer. IPv6 addresses are reduced to their /64 network.
func ClientIP(r *http.Request) string {
	addr, err := netip.ParseAddr(r.Header.Get(HeaderClientIP))
	if err != nil {
		host, _, splitErr := net.SplitHostPort(r.RemoteAddr)
		if splitErr != nil {
			host = r.RemoteAddr
		}
		if addr, err = netip.ParseAddr(host); err != nil {
			return ""
		}
	}
	addr = addr.Unmap()
	if addr.Is6() {
		return netip.PrefixFrom(addr, 64).Masked().String()
	}
	return addr.String()
}
