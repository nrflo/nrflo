package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// shortTempSocket creates a short-path temp directory and returns a socket path within it.
// Unix socket paths are limited to 104 bytes on macOS; t.TempDir() paths are too long.
func shortTempSocket(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "nrf")
	if err != nil {
		t.Fatalf("create temp socket dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "s.sock")
}

// saveServeFlags captures current package-level serve vars and returns a restore func.
// Must be called (and restore deferred) at the start of any test that mutates these vars.
func saveServeFlags(t *testing.T) func() {
	t.Helper()
	dp, sm, sh, sp, ic, nt := DataPath, serveMode, serveHost, servePort, insecureCookies, noTray
	return func() {
		DataPath = dp
		serveMode = sm
		serveHost = sh
		servePort = sp
		insecureCookies = ic
		noTray = nt
	}
}

// cleanupSetupServer shuts down serverComponents returned by a successful setupServer call.
func cleanupSetupServer(sc *serverComponents) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = sc.socketServer.Stop(ctx)
	sc.pool.Close()
}

// TestSetupServer_InvalidMode verifies setupServer rejects unsupported mode strings.
func TestSetupServer_InvalidMode(t *testing.T) {
	restore := saveServeFlags(t)
	defer restore()

	serveMode = "invalid"

	sc, err := setupServer()
	if err == nil {
		t.Error("setupServer() with invalid mode should return an error, got nil")
		cleanupSetupServer(sc)
		return
	}
}

// TestSetupServer_ValidModes verifies setupServer accepts the two supported mode values.
func TestSetupServer_ValidModes(t *testing.T) {
	for _, mode := range []string{"cli", "api"} {
		t.Run("mode_"+mode, func(t *testing.T) {
			restore := saveServeFlags(t)
			defer restore()

			tmpHome := t.TempDir()
			t.Setenv("NRFLO_HOME", tmpHome)
			t.Setenv("NRFLO_SOCKET", shortTempSocket(t))

			DataPath = ""
			serveMode = mode

			sc, err := setupServer()
			if err != nil {
				t.Fatalf("setupServer(mode=%q) unexpected error: %v", mode, err)
			}
			defer cleanupSetupServer(sc)
		})
	}
}

// TestSetupServer_SDKInstalled verifies the Python SDK is written to <dataDir>/sdk/nrflo_sdk.py
// for both the default-path (empty DataPath + NRFLO_HOME) and the explicit-DataPath cases.
func TestSetupServer_SDKInstalled(t *testing.T) {
	cases := []struct {
		name    string
		setup   func(t *testing.T) (dataPath, wantSDK string)
	}{
		{
			name: "empty_datapath_uses_nrflo_home",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				tmpHome := t.TempDir()
				t.Setenv("NRFLO_HOME", tmpHome)
				return "", filepath.Join(tmpHome, "sdk", "nrflo_sdk.py")
			},
		},
		{
			name: "explicit_datapath_installs_next_to_db",
			setup: func(t *testing.T) (string, string) {
				t.Helper()
				tmpDir := t.TempDir()
				dp := filepath.Join(tmpDir, "custom.data")
				return dp, filepath.Join(tmpDir, "sdk", "nrflo_sdk.py")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			restore := saveServeFlags(t)
			defer restore()

			// Unique socket path per subtest prevents bind conflicts when running sequentially.
			t.Setenv("NRFLO_SOCKET", shortTempSocket(t))

			dp, wantSDK := tc.setup(t)
			DataPath = dp
			serveMode = "cli"

			sc, err := setupServer()
			if err != nil {
				t.Fatalf("setupServer() error: %v", err)
			}
			defer cleanupSetupServer(sc)

			if _, statErr := os.Stat(wantSDK); statErr != nil {
				t.Errorf("SDK not installed: want file at %s, got error: %v", wantSDK, statErr)
			}
		})
	}
}

// TestSetupServer_PoolPathMatchesResolvedDataPath verifies the pool.Path reflects the
// resolved data path (db.GetDBPath), not the raw DataPath flag.
func TestSetupServer_PoolPathMatchesResolvedDataPath(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("NRFLO_HOME", tmpHome)
	t.Setenv("NRFLO_SOCKET", shortTempSocket(t))

	restore := saveServeFlags(t)
	defer restore()

	DataPath = ""
	serveMode = "cli"

	sc, err := setupServer()
	if err != nil {
		t.Fatalf("setupServer() error: %v", err)
	}
	defer cleanupSetupServer(sc)

	wantPath := filepath.Join(tmpHome, "nrflo.data")
	if sc.pool.Path != wantPath {
		t.Errorf("pool.Path = %q, want %q", sc.pool.Path, wantPath)
	}
}
