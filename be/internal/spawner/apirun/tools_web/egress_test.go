package tools_web

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// withMockLookup swaps lookupIP for the duration of the test.
func withMockLookup(t *testing.T, fn func(host string) ([]net.IP, error)) {
	t.Helper()
	orig := lookupIP
	lookupIP = fn
	t.Cleanup(func() { lookupIP = orig })
}

func TestPinningDialer_RejectsPrivateAndMetadata(t *testing.T) {
	cases := []struct {
		name string
		ip   string
	}{
		{"private 10/8", "10.0.0.1"},
		{"cloud metadata", "169.254.169.254"},
		{"loopback", "127.0.0.1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withMockLookup(t, func(host string) ([]net.IP, error) {
				return []net.IP{net.ParseIP(tc.ip)}, nil
			})
			forwardCalled := false
			pin := &pinningDialer{forward: func(ctx context.Context, network, addr string) (net.Conn, error) {
				forwardCalled = true
				return nil, nil
			}}
			_, err := pin.DialContext(context.Background(), "tcp", "blocked.example.test:443")
			if err == nil {
				t.Fatalf("DialContext() err = nil, want rejection for resolved IP %s", tc.ip)
			}
			if !strings.Contains(err.Error(), "resolves to no allowed address") {
				t.Errorf("err = %v, want the no-allowed-address message", err)
			}
			if forwardCalled {
				t.Error("forward was called for a blocked IP, want dial refused before connecting")
			}
		})
	}
}

func TestPinningDialer_AllowsPublicIP(t *testing.T) {
	withMockLookup(t, func(host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("8.8.8.8")}, nil
	})
	var capturedAddr string
	pin := &pinningDialer{forward: func(ctx context.Context, network, addr string) (net.Conn, error) {
		capturedAddr = addr
		return nil, nil
	}}
	if _, err := pin.DialContext(context.Background(), "tcp", "public.example.test:443"); err != nil {
		t.Fatalf("DialContext() err = %v, want nil for a public resolved IP", err)
	}
	if capturedAddr != "8.8.8.8:443" {
		t.Errorf("forward dialed %q, want the validated IP 8.8.8.8:443 (never the hostname)", capturedAddr)
	}
}

func TestPinningDialer_LiteralIPHostSkipsLookup(t *testing.T) {
	withMockLookup(t, func(host string) ([]net.IP, error) {
		t.Fatalf("lookupIP called for literal IP host %q, want no DNS lookup", host)
		return nil, nil
	})
	pin := &pinningDialer{forward: func(ctx context.Context, network, addr string) (net.Conn, error) {
		return nil, nil
	}}
	if _, err := pin.DialContext(context.Background(), "tcp", "1.1.1.1:443"); err != nil {
		t.Fatalf("DialContext() err = %v, want nil for an allowed literal public IP", err)
	}
	if _, err := pin.DialContext(context.Background(), "tcp", "127.0.0.1:443"); err == nil {
		t.Fatal("DialContext() err = nil, want rejection for a literal loopback IP")
	}
}

func TestPinningDialer_ResolveError(t *testing.T) {
	withMockLookup(t, func(host string) ([]net.IP, error) {
		return nil, fmt.Errorf("no such host")
	})
	pin := &pinningDialer{}
	if _, err := pin.DialContext(context.Background(), "tcp", "nxdomain.example.test:443"); err == nil {
		t.Fatal("DialContext() err = nil, want an error when resolution fails")
	}
}

func TestPinningDialer_ProxyAddrExemptFromCheck(t *testing.T) {
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("proxy-ok"))
	}))
	t.Cleanup(proxySrv.Close)
	proxyAddr := proxySrv.Listener.Addr().String() // 127.0.0.1:PORT — loopback

	pin := &pinningDialer{proxyAddr: proxyAddr}

	conn, err := pin.DialContext(context.Background(), "tcp", proxyAddr)
	if err != nil {
		t.Fatalf("DialContext() to the proxy's own loopback address err = %v, want nil (exempt)", err)
	}
	_ = conn.Close()

	// The target is never exempt, even though it's also loopback: blockedIP
	// runs on the target, never on the proxy address.
	if _, err := pin.DialContext(context.Background(), "tcp", "127.0.0.1:9"); err == nil {
		t.Fatal("DialContext() err = nil for a non-proxy loopback target, want rejection")
	}
}

func TestPinningDialer_CrossHostRedirectRevalidatesEachHop(t *testing.T) {
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("final content"))
	}))
	t.Cleanup(srv2.Close)
	srv2Port := srv2.Listener.Addr().(*net.TCPAddr).Port

	var srv1Port int
	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, fmt.Sprintf("http://hop2.example.test:%d/final", srv2Port), http.StatusFound)
	}))
	t.Cleanup(srv1.Close)
	srv1Port = srv1.Listener.Addr().(*net.TCPAddr).Port

	withMockLookup(t, func(host string) ([]net.IP, error) {
		switch host {
		case "hop1.example.test":
			return []net.IP{net.ParseIP("93.184.216.34")}, nil // public, allowed
		case "hop2.example.test":
			return []net.IP{net.ParseIP("169.254.169.254")}, nil // metadata, blocked
		}
		return nil, fmt.Errorf("unexpected host %q", host)
	})

	// forward tunnels a validated dial back to the real local test server on
	// the matching port — this keeps the test hermetic (no real network)
	// while still exercising the real pinning-dialer validation per hop.
	pin := &pinningDialer{forward: func(ctx context.Context, network, addr string) (net.Conn, error) {
		_, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		var d net.Dialer
		return d.DialContext(ctx, network, net.JoinHostPort("127.0.0.1", port))
	}}
	client := &http.Client{
		Transport:     &http.Transport{DialContext: pin.DialContext},
		CheckRedirect: allowCrossHostRedirects,
	}

	resp, err := client.Get(fmt.Sprintf("http://hop1.example.test:%d/", srv1Port))
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("client.Get() err = nil, want the second hop rejected by the pinning dialer")
	}
	if !strings.Contains(err.Error(), "resolves to no allowed address") {
		t.Errorf("err = %v, want it to name the hop-2 rejection", err)
	}
}

func TestPinningDialer_CrossHostRedirectAllowedWhenBothHopsPublic(t *testing.T) {
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("final content"))
	}))
	t.Cleanup(srv2.Close)
	srv2Port := srv2.Listener.Addr().(*net.TCPAddr).Port

	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, fmt.Sprintf("http://hop2.example.test:%d/final", srv2Port), http.StatusFound)
	}))
	t.Cleanup(srv1.Close)
	srv1Port := srv1.Listener.Addr().(*net.TCPAddr).Port

	withMockLookup(t, func(host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	})

	pin := &pinningDialer{forward: func(ctx context.Context, network, addr string) (net.Conn, error) {
		_, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		var d net.Dialer
		return d.DialContext(ctx, network, net.JoinHostPort("127.0.0.1", port))
	}}
	client := &http.Client{
		Transport:     &http.Transport{DialContext: pin.DialContext},
		CheckRedirect: allowCrossHostRedirects,
	}

	resp, err := client.Get(fmt.Sprintf("http://hop1.example.test:%d/", srv1Port))
	if err != nil {
		t.Fatalf("client.Get() err = %v, want a followed cross-host redirect to succeed", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.Request.URL.Host != fmt.Sprintf("hop2.example.test:%d", srv2Port) {
		t.Errorf("final URL host = %q, want the redirect to have been followed to hop2", resp.Request.URL.Host)
	}
}

func TestBuildProxyDialer(t *testing.T) {
	t.Run("empty is a no-op", func(t *testing.T) {
		forward, proxyAddr, httpProxyURL, err := buildProxyDialer("")
		if err != nil || forward != nil || proxyAddr != "" || httpProxyURL != nil {
			t.Fatalf("buildProxyDialer(\"\") = (forward-nil=%v, %q, %v, %v), want all zero", forward == nil, proxyAddr, httpProxyURL, err)
		}
	})

	t.Run("socks5", func(t *testing.T) {
		forward, proxyAddr, httpProxyURL, err := buildProxyDialer("socks5://127.0.0.1:1080")
		if err != nil {
			t.Fatalf("buildProxyDialer() err = %v, want nil", err)
		}
		if forward == nil {
			t.Error("forward = nil, want a SOCKS5 dial func")
		}
		if proxyAddr != "" {
			t.Errorf("proxyAddr = %q, want empty for socks5 (no exemption needed)", proxyAddr)
		}
		if httpProxyURL != nil {
			t.Errorf("httpProxyURL = %v, want nil for socks5", httpProxyURL)
		}
	})

	t.Run("http", func(t *testing.T) {
		forward, proxyAddr, httpProxyURL, err := buildProxyDialer("http://127.0.0.1:8080")
		if err != nil {
			t.Fatalf("buildProxyDialer() err = %v, want nil", err)
		}
		if forward != nil {
			t.Error("forward != nil, want nil for an http proxy (Transport.Proxy handles it)")
		}
		if proxyAddr != "127.0.0.1:8080" {
			t.Errorf("proxyAddr = %q, want 127.0.0.1:8080", proxyAddr)
		}
		if httpProxyURL == nil || httpProxyURL.Host != "127.0.0.1:8080" {
			t.Errorf("httpProxyURL = %v, want host 127.0.0.1:8080", httpProxyURL)
		}
	})

	t.Run("malformed scheme", func(t *testing.T) {
		if _, _, _, err := buildProxyDialer("ftp://127.0.0.1:21"); err == nil {
			t.Fatal("buildProxyDialer() err = nil, want error for unsupported scheme")
		}
	})
}

func TestNewEgress_InvalidProxyURL(t *testing.T) {
	t.Setenv("WEB_PROXY_URL", "ftp://bad-scheme")
	t.Cleanup(func() { os.Unsetenv("WEB_PROXY_URL") })

	r := NewResolver(nil, "")
	if _, err := newEgress(r); err == nil {
		t.Fatal("newEgress() err = nil, want an error for an unsupported WEB_PROXY_URL scheme")
	}
}

func TestNewEgress_DirectByDefault(t *testing.T) {
	t.Setenv("WEB_PROXY_URL", "")
	r := NewResolver(nil, "")
	eg, err := newEgress(r)
	if err != nil {
		t.Fatalf("newEgress() err = %v, want nil", err)
	}
	if eg.Guarded() == nil || eg.Trusted() == nil {
		t.Fatal("newEgress() returned an Egress with a nil client")
	}
}
