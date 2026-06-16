package spawner

import (
	"strings"
	"testing"
)

// TestClaudeBuildInteractiveCommand_FallbackModel verifies the fallback chain is
// passed as a single comma-separated --fallback-model arg, and omitted when unset.
func TestClaudeBuildInteractiveCommand_FallbackModel(t *testing.T) {
	a := &ClaudeAdapter{}

	cmd := a.BuildInteractiveCommand(InteractiveSpawnOptions{
		SessionID:      "s1",
		Model:          "claude-opus-4-8",
		FallbackModels: "claude-opus-4-7,claude-sonnet-4-6",
	})
	args := strings.Join(cmd.Args, " ")
	if !strings.Contains(args, "--fallback-model claude-opus-4-7,claude-sonnet-4-6") {
		t.Errorf("expected --fallback-model with chain, got: %s", args)
	}

	cmd2 := a.BuildInteractiveCommand(InteractiveSpawnOptions{SessionID: "s2", Model: "claude-sonnet-4-6"})
	if strings.Contains(strings.Join(cmd2.Args, " "), "--fallback-model") {
		t.Errorf("did not expect --fallback-model when unset, got: %s", strings.Join(cmd2.Args, " "))
	}
}
