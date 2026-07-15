package cli

import (
	"context"
	"strings"
	"testing"
)

// TestRunConsole_RemoteNoToken errors before any network call when a remote
// server is targeted without a service token — the socket (local-trust) path
// is unavailable for a non-loopback server.
func TestRunConsole_RemoteNoToken(t *testing.T) {
	fakeBinDir(t, "claude")
	setConsoleFlags(t, "claude", "", "p1", "https://nrflo.example.com", "")

	_, err := runConsole(context.Background())
	if err == nil {
		t.Fatal("expected an error for a remote server without a token")
	}
	if !strings.Contains(err.Error(), "service token required for a remote server") {
		t.Errorf("error = %q, want remote-token message", err.Error())
	}
}
