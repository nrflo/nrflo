package spawner

import (
	"strings"
	"testing"
)

func TestGetCLIAdapter_Codex(t *testing.T) {
	t.Parallel()
	adapter, err := GetCLIAdapter("codex")
	if err != nil {
		t.Fatalf("GetCLIAdapter('codex') returned error: %v", err)
	}
	if adapter.Name() != "codex" {
		t.Errorf("adapter.Name() = %q, want 'codex'", adapter.Name())
	}
}

func TestCodexAdapter_Capabilities(t *testing.T) {
	t.Parallel()
	adapter, _ := GetCLIAdapter("codex")

	if adapter.SupportsSessionID() {
		t.Error("SupportsSessionID() should be false")
	}
	if adapter.SupportsSystemPromptFile() {
		t.Error("SupportsSystemPromptFile() should be false")
	}
}

func TestClaudeAdapter_BuildInteractiveCommand_OmitsPartialMessages(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}

	opts := InteractiveSpawnOptions{
		Model:     "opus-4-7",
		SessionID: "sess-interactive-partial-1",
		WorkDir:   "/tmp",
	}

	cmd := adapter.BuildInteractiveCommand(opts)
	args := strings.Join(cmd.Args, " ")

	if strings.Contains(args, "--include-partial-messages") {
		t.Errorf("BuildInteractiveCommand args should NOT contain --include-partial-messages: %s", args)
	}
}
