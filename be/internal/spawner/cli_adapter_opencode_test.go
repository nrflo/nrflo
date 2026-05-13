package spawner

import "testing"

// TestOpencodeAdapter_SupportsInteractive_False locks in that opencode does
// not support cli_interactive mode. opencode 1.14.48 does not surface chat
// events through any observable channel (SSE, REST poll, or storage), so PTY
// runs would produce 0 agent_messages rows and silently mask agent activity.
// Flipping this back to true requires confirming opencode publishes events.
// See backlog.md "Opencode `cli_interactive` not supported".
func TestOpencodeAdapter_SupportsInteractive_False(t *testing.T) {
	t.Parallel()
	a := &OpencodeAdapter{}
	if a.SupportsInteractive() {
		t.Error("OpencodeAdapter.SupportsInteractive() = true, want false")
	}
}
