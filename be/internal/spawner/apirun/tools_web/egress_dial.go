package tools_web

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// lookupIP is overridable in tests to exercise resolve/rebind cases without
// real DNS. This is the SINGLE authoritative resolve for guarded egress — no
// other code path in this package looks up a hostname.
var lookupIP = net.LookupIP

const (
	// maxRedirectHops caps guarded/trusted client redirect following.
	maxRedirectHops = 10
	dialTimeout     = 10 * time.Second
)

// pinningDialer is the guarded client's DialContext: it resolves the target
// host, validates every candidate IP via CheckResolvedIP, and dials the
// first validated IP — never the hostname — which is what defeats DNS
// rebinding (a name that answers differently between check and connect).
//
// It doubles as the redirect re-validator: net/http calls DialContext again
// for every redirect hop before issuing the request, so a benign host that
// redirects to a private address is caught on the hop, not just the origin.
type pinningDialer struct {
	// forward, when set, dials the validated IP through a SOCKS5 proxy
	// (the proxy itself is reached by the SOCKS5 client's own connection,
	// never through this dialer). nil means dial the validated IP directly.
	forward func(ctx context.Context, network, addr string) (net.Conn, error)

	// proxyAddr is the host:port of an HTTP-proxy this dialer must also be
	// able to reach directly. When Transport.Proxy is set, net/http dials
	// the proxy's own address through this DialContext (to establish the
	// CONNECT tunnel) rather than the agent-supplied target — that dial is
	// to trusted, operator-configured infra, so it is exempt from
	// CheckResolvedIP (e.g. a loopback proxy is expected and fine).
	proxyAddr string
}

func (d *pinningDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if d.proxyAddr != "" && addr == d.proxyAddr {
		nd := &net.Dialer{Timeout: dialTimeout}
		return nd.DialContext(ctx, network, addr)
	}

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("egress: split host:port %q: %w", addr, err)
	}

	var candidates []net.IP
	if lit := net.ParseIP(host); lit != nil {
		candidates = []net.IP{lit}
	} else {
		resolved, lerr := lookupIP(host)
		if lerr != nil {
			return nil, fmt.Errorf("egress: resolve %q: %w", host, lerr)
		}
		candidates = resolved
	}

	var validated net.IP
	for _, ip := range candidates {
		if verr := CheckResolvedIP(ip); verr == nil {
			validated = ip
			break
		}
	}
	if validated == nil {
		return nil, fmt.Errorf("egress: %q resolves to no allowed address", host)
	}
	dialAddr := net.JoinHostPort(validated.String(), port)

	if d.forward != nil {
		return d.forward(ctx, network, dialAddr)
	}
	nd := &net.Dialer{Timeout: dialTimeout}
	return nd.DialContext(ctx, network, dialAddr)
}

// allowCrossHostRedirects is the CheckRedirect policy shared by both egress
// clients: cross-host redirects are ALLOWED (capped at maxRedirectHops)
// because safety comes from re-dialing (and therefore re-validating) every
// hop through pinningDialer above, not from blocking the host change.
func allowCrossHostRedirects(_ *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirectHops {
		return fmt.Errorf("stopped after %d redirects", maxRedirectHops)
	}
	return nil
}

// limitedBody caps a response body read at n bytes so a malicious or
// oversized response can't exhaust memory; used by direct.go before
// buffering a fetched page for readability parsing.
func limitedBody(body io.Reader, n int64) io.Reader {
	return io.LimitReader(body, n)
}
