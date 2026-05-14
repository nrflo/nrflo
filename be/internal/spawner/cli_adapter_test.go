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

func TestCodexAdapter_MapModel(t *testing.T) {
	t.Parallel()
	adapter := &CodexAdapter{}

	tests := []struct {
		input string
		want  string
	}{
		{"codex_gpt_normal", "gpt-5.3-codex"},
		{"codex_gpt_high", "gpt-5.3-codex"},
		{"custom-model", "custom-model"},
	}

	for _, tt := range tests {
		got := adapter.MapModel(tt.input)
		if got != tt.want {
			t.Errorf("MapModel(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCodexAdapter_GetReasoningEffort(t *testing.T) {
	t.Parallel()
	adapter := &CodexAdapter{}

	tests := []struct {
		input string
		want  string
	}{
		{"codex_gpt_normal", "high"},
		{"codex_gpt_high", "high"},
		{"custom", "high"},
	}

	for _, tt := range tests {
		got := adapter.GetReasoningEffort(tt.input)
		if got != tt.want {
			t.Errorf("GetReasoningEffort(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestOpencodeAdapter_GetReasoningEffort(t *testing.T) {
	t.Parallel()
	adapter := &OpencodeAdapter{}

	tests := []struct {
		input string
		want  string
	}{
		{"opencode_gpt54", "high"},
		{"opencode_minimax_m25_free", ""},
		{"opencode_qwen36_plus_free", ""},
		{"opus_4_7", ""},
		{"sonnet", ""},
		{"haiku", ""},
		{"custom", ""},
	}

	for _, tt := range tests {
		got := adapter.GetReasoningEffort(tt.input)
		if got != tt.want {
			t.Errorf("GetReasoningEffort(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestOpencodeAdapter_MapModel(t *testing.T) {
	t.Parallel()
	adapter := &OpencodeAdapter{}

	tests := []struct {
		input string
		want  string
	}{
		{"opencode_minimax_m25_free", "opencode/minimax-m2.5-free"},
		{"opencode_qwen36_plus_free", "opencode/qwen3.6-plus-free"},
		{"opencode_gpt54", "openai/gpt-5.4"},
		{"openai/gpt-4o", "openai/gpt-4o"},
		{"custom", "anthropic/custom"},
	}

	for _, tt := range tests {
		got := adapter.MapModel(tt.input)
		if got != tt.want {
			t.Errorf("MapModel(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestClaudeAdapter_BuildInteractiveCommand_OmitsPartialMessages(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}

	opts := InteractiveSpawnOptions{
		Model:     "opus_4_7",
		SessionID: "sess-interactive-partial-1",
		WorkDir:   "/tmp",
	}

	cmd := adapter.BuildInteractiveCommand(opts)
	args := strings.Join(cmd.Args, " ")

	if strings.Contains(args, "--include-partial-messages") {
		t.Errorf("BuildInteractiveCommand args should NOT contain --include-partial-messages: %s", args)
	}
}

func TestClaudeAdapter_MapModel(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}

	tests := []struct {
		input string
		want  string
	}{
		{"opus_4_6", "claude-opus-4-6"},
		{"opus_4_6_1m", "claude-opus-4-6[1m]"},
		{"opus_4_7", "claude-opus-4-7"},
		{"opus_4_7_1m", "claude-opus-4-7[1m]"},
		{"sonnet", "sonnet"},
		{"haiku", "haiku"},
	}

	for _, tt := range tests {
		got := adapter.MapModel(tt.input)
		if got != tt.want {
			t.Errorf("MapModel(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
