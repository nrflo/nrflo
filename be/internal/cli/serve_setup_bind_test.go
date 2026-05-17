package cli

import (
	"strings"
	"testing"
)

// TestSetupServer_SocketBindFailure verifies that setupServer returns a non-nil error
// containing the resolved socket path when BindListener cannot bind the Unix socket.
// /dev/null/x is guaranteed to fail: /dev/null is a file, not a directory, so
// MkdirAll cannot create a parent, and net.Listen("unix", ...) will also fail.
func TestSetupServer_SocketBindFailure(t *testing.T) {
	restore := saveServeFlags(t)
	defer restore()

	tmpHome := t.TempDir()
	t.Setenv("NRFLO_HOME", tmpHome)
	t.Setenv("NRFLO_SOCKET", "/dev/null/x")

	DataPath = ""

	sc, err := setupServer()
	if err == nil {
		cleanupSetupServer(sc)
		t.Fatal("setupServer() want error for unbindable socket path, got nil")
	}
	if !strings.Contains(err.Error(), "/dev/null/x") {
		t.Errorf("setupServer() error = %q, want it to contain the resolved socket path", err.Error())
	}
}
