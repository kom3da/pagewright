package api

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
	"unicode"
	"unicode/utf8"
)

// clientKey identifies the client for rate limits. Behind a trusted proxy
// it reads X-Forwarded-For, taking the right-most address the proxies did
// not add. IPv6 addresses are cut to their /64, which one host usually has.
func (s *Server) clientKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	if s.trusted(ip) {
		hops := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
		for i := len(hops) - 1; i >= 0; i-- {
			hop, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
			if err != nil {
				break
			}
			ip = hop
			if !s.trusted(hop) {
				break
			}
		}
	}
	ip = ip.Unmap()
	if ip.Is6() {
		p, _ := ip.Prefix(64)
		return p.String()
	}
	return ip.String()
}

func (s *Server) trusted(ip netip.Addr) bool {
	for _, p := range s.TrustedProxies {
		if p.Contains(ip.Unmap()) {
			return true
		}
	}
	return false
}

// plainText reports whether s is valid UTF-8 without control characters
// other than newlines and tabs (allowed only when multiline).
func plainText(s string, multiline bool) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if multiline && (r == '\n' || r == '\r' || r == '\t') {
			continue
		}
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
