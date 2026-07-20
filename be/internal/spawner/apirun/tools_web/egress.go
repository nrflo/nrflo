package tools_web

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/proxy"
)

// testRootCAs, when non-nil, is applied to both egress clients' TLS config.
// It exists purely so tests can point at an httptest.NewTLSServer's
// certificate (via srv.Certificate()); production code never sets it, so
// real-world TLS verification is untouched.
var testRootCAs *x509.CertPool

// RoundTripper is a nil-in-v1 seam. Setting it would let a future version
// swap in a TLS/h2 fingerprinting transport (the anti-bot ceiling rotating
// headers.go can't clear) without touching any provider call site. v1 stays
// pure Go — no utls/oohttp/tls-client — so this is always nil today.
var RoundTripper http.RoundTripper

// Egress builds the two http.Clients every web tool provider draws from:
// Guarded, for agent-supplied fetch targets (SSRF pinning dialer — the
// threat model is the agent itself), and Trusted, for operator-configured
// infra (the SearXNG instance, and reaching the proxy). Both are proxy-aware
// per WEB_PROXY_URL; only Guarded pins the dialed IP.
type Egress struct {
	guarded  *http.Client
	trusted  *http.Client
	maxBytes int64
}

// newEgress builds an Egress from resolver config (WEB_PROXY_URL, empty =
// direct egress; web_fetch_max_bytes).
func newEgress(r *Resolver) (*Egress, error) {
	forward, proxyAddr, httpProxyURL, err := buildProxyDialer(strings.TrimSpace(r.ProxyURL()))
	if err != nil {
		return nil, err
	}

	pin := &pinningDialer{forward: forward, proxyAddr: proxyAddr}
	guardedTransport := &http.Transport{DialContext: pin.DialContext}
	trustedTransport := &http.Transport{}
	if forward != nil {
		// SOCKS5: the trusted client is still proxy-aware, just unguarded.
		trustedTransport.DialContext = forward
	}
	if httpProxyURL != nil {
		guardedTransport.Proxy = http.ProxyURL(httpProxyURL)
		trustedTransport.Proxy = http.ProxyURL(httpProxyURL)
	}
	if testRootCAs != nil {
		tlsCfg := &tls.Config{RootCAs: testRootCAs}
		guardedTransport.TLSClientConfig = tlsCfg
		trustedTransport.TLSClientConfig = tlsCfg
	}

	return &Egress{
		guarded:  &http.Client{Transport: guardedTransport, CheckRedirect: allowCrossHostRedirects},
		trusted:  &http.Client{Transport: trustedTransport, CheckRedirect: allowCrossHostRedirects},
		maxBytes: int64(r.MaxBytes()),
	}, nil
}

// Guarded is the SSRF-pinning client for agent-supplied fetch targets.
func (e *Egress) Guarded() *http.Client { return e.guarded }

// Trusted is the proxy-aware, unguarded client for operator-configured infra.
func (e *Egress) Trusted() *http.Client { return e.trusted }

// MaxBytes is the configured response body cap (web_fetch_max_bytes).
func (e *Egress) MaxBytes() int64 { return e.maxBytes }

// buildProxyDialer parses WEB_PROXY_URL and returns the wiring newEgress
// needs: for socks5://, a forward DialContext that routes an already-pinned
// dial through the SOCKS5 proxy; for http(s)://, the proxy URL for
// Transport.Proxy plus its host:port so the pinning dialer can recognize
// (and exempt) the direct dial to the proxy itself. Empty input is a no-op
// (direct egress, no proxy).
func buildProxyDialer(raw string) (
	forward func(ctx context.Context, network, addr string) (net.Conn, error),
	proxyAddr string,
	httpProxyURL *url.URL,
	err error,
) {
	if raw == "" {
		return nil, "", nil, nil
	}
	u, perr := url.Parse(raw)
	if perr != nil {
		return nil, "", nil, fmt.Errorf("WEB_PROXY_URL: invalid: %w", perr)
	}
	switch u.Scheme {
	case "socks5", "socks5h":
		d, derr := proxy.SOCKS5("tcp", u.Host, nil, nil)
		if derr != nil {
			return nil, "", nil, fmt.Errorf("WEB_PROXY_URL: socks5 dialer: %w", derr)
		}
		cd, ok := d.(proxy.ContextDialer)
		if !ok {
			return nil, "", nil, fmt.Errorf("WEB_PROXY_URL: socks5 dialer missing DialContext")
		}
		return cd.DialContext, "", nil, nil
	case "http", "https":
		return nil, u.Host, u, nil
	default:
		return nil, "", nil, fmt.Errorf("WEB_PROXY_URL: unsupported scheme %q (want socks5:// or http://)", u.Scheme)
	}
}
