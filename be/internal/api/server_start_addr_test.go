package api

import (
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"be/internal/config"
	"be/internal/db"
)

// findFreePort finds an available TCP port on 127.0.0.1.
func findFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("findFreePort: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

// newStartTestServer creates a minimal Server backed by a temp DB.
func newStartTestServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "srv_start_test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("newStartTestServer: pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	cfg := config.DefaultConfig()
	return NewServer(cfg, dbPath, t.TempDir(), pool)
}

// waitHTTP polls baseURL until it returns a non-error response or deadline passes.
func waitHTTP(t *testing.T, baseURL string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/api/v1/projects")
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("server at %s did not become ready within %s", baseURL, timeout)
}

// TestServerStart_AddrFormat verifies that Start(host, port) sets httpServer.Addr
// to "host:port" for each combination.
func TestServerStart_AddrFormat(t *testing.T) {
	cases := []struct {
		name string
		host string
	}{
		{"localhost", "127.0.0.1"},
		{"all interfaces", "0.0.0.0"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			port := findFreePort(t)
			wantAddr := fmt.Sprintf("%s:%d", tc.host, port)

			srv := newStartTestServer(t)
			go func() { _ = srv.Start(tc.host, port) }()

			waitHTTP(t, fmt.Sprintf("http://127.0.0.1:%d", port), 3*time.Second)
			t.Cleanup(func() { srv.Stop(nil) })

			if srv.httpServer == nil {
				t.Fatal("httpServer is nil after Start()")
			}
			if srv.httpServer.Addr != wantAddr {
				t.Errorf("httpServer.Addr = %q, want %q", srv.httpServer.Addr, wantAddr)
			}
		})
	}
}

// TestServerStart_DefaultHostBindsLocalhost verifies the server is reachable
// on 127.0.0.1 when started with the default host.
func TestServerStart_DefaultHostBindsLocalhost(t *testing.T) {
	port := findFreePort(t)
	srv := newStartTestServer(t)

	go func() { _ = srv.Start("127.0.0.1", port) }()

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitHTTP(t, baseURL, 3*time.Second)
	t.Cleanup(func() { srv.Stop(nil) })

	resp, err := http.Get(baseURL + "/api/v1/projects")
	if err != nil {
		t.Fatalf("GET /api/v1/projects: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}
