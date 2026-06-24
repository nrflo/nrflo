package tools_web

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// lookupIP is overridable in tests to exercise hostname cases without DNS.
var lookupIP = net.LookupIP

// ValidateFetchURL is the SSRF guard applied to every agent-supplied fetch URL
// before any provider call. It rejects non-https schemes, userinfo, non-443
// ports, and hosts that resolve to loopback / link-local (incl. cloud metadata
// 169.254.169.254) / private / unspecified / multicast addresses.
//
// Note: validating the resolved IP defeats name-based attacks but not active
// DNS-rebinding; a dial-time re-check is the hardened phase-2 form.
func ValidateFetchURL(raw string) error {
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

	var ips []net.IP
	if lit := net.ParseIP(host); lit != nil {
		ips = []net.IP{lit}
	} else {
		resolved, lerr := lookupIP(host)
		if lerr != nil {
			return fmt.Errorf("dns resolve %q: %w", host, lerr)
		}
		ips = resolved
	}
	for _, ip := range ips {
		if blocked, why := blockedIP(ip); blocked {
			return fmt.Errorf("host resolves to %s address (%s)", why, ip)
		}
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
