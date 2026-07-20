package tools_web

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ValidateFetchURLSyntax is the cheap, no-DNS guard applied to every
// agent-supplied fetch URL before any provider call. It rejects non-https
// schemes, userinfo, non-443 ports, and empty hosts; a literal IP host is
// checked against blockedIP directly (no resolution needed).
//
// It does NOT resolve hostnames: CheckResolvedIP (called by the pinning
// dialer in egress_dial.go, the single authoritative resolve) is what
// validates a hostname's actual IP at dial time — for every hop of a
// redirect, not just this initial syntax check — so there is no
// double-resolve/TOCTOU window between "looked fine here" and "dialed there".
func ValidateFetchURLSyntax(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("scheme %q not allowed (https only)", u.Scheme)
	}
	if u.User != nil {
		return fmt.Errorf("userinfo not allowed")
	}
	if p := u.Port(); p != "" && p != "443" {
		return fmt.Errorf("port %q not allowed (443 only)", p)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("empty host")
	}
	if lit := net.ParseIP(host); lit != nil {
		if blocked, why := blockedIP(lit); blocked {
			return fmt.Errorf("host resolves to %s address (%s)", why, lit)
		}
	}
	return nil
}

// CheckResolvedIP is the authoritative SSRF check applied to every candidate
// IP the pinning dialer resolves — rejecting loopback / unspecified /
// link-local (incl. cloud metadata 169.254.169.254) / multicast / private
// addresses. Because the dialer re-resolves and re-checks on every
// cross-host redirect hop, this defeats DNS rebinding (a name that resolves
// to a public IP when first checked but a private one when actually dialed).
func CheckResolvedIP(ip net.IP) error {
	if blocked, why := blockedIP(ip); blocked {
		return fmt.Errorf("resolves to %s address (%s)", why, ip)
	}
	return nil
}

func blockedIP(ip net.IP) (bool, string) {
	switch {
	case ip.IsLoopback():
		return true, "loopback"
	case ip.IsUnspecified():
		return true, "unspecified"
	case ip.IsLinkLocalUnicast(): // 169.254.0.0/16 (incl. metadata), fe80::/10
		return true, "link-local"
	case ip.IsLinkLocalMulticast(), ip.IsMulticast():
		return true, "multicast"
	case ip.IsPrivate(): // RFC1918 + RFC4193 fc00::/7
		return true, "private"
	}
	return false, ""
}
