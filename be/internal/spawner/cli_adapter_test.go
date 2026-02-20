package spawner

import (
	"strings"
	"testing"
)

func TestGetCLIAdapter_Codex(t *testing.T) {
	adapter, err := GetCLIAdapter("codex")
	if err != nil {
		t.Fatalf("GetCLIAdapter('codex') returned error: %v", err)
	}
	if adapter.Name() != "codex" {
		t.Errorf("adapter.Name() = %q, want 'codex'", adapter.Name())
	}
}

func TestCodexAdapter_Capabilities(t *testing.T) {
	adapter, _ := GetCLIAdapter("codex")

	if adapter.SupportsSessionID() {
		t.Error("SupportsSessionID() should be false")
	}
	if adapter.SupportsSystemPromptFile() {
		t.Error("SupportsSystemPromptFile() should be false")
	}
}

func TestCodexAdapter_MapModel(t *testing.T) {
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

func TestCodexAdapter_BuildCommand(t *testing.T) {
	adapter := &CodexAdapter{}

	opts := SpawnOptions{
		Model:         "codex_gpt_high",
		Prompt:        "System prompt",
		InitialPrompt: "Do the task",
		WorkDir:       "/tmp",
	}

	cmd := adapter.BuildCommand(opts)

	// Check command name
	if !strings.HasSuffix(cmd.Path, "codex") && cmd.Args[0] != "codex" {
		t.Errorf("Expected codex command, got %s", cmd.Path)
	}

	// Check required args are present
	args := strings.Join(cmd.Args, " ")
	requiredArgs := []string{
		"exec",
		"--json",
		"--full-auto",
		"--sandbox", "danger-full-access",
		"--skip-git-repo-check",
		"--model", "gpt-5.3-codex",
		"model_reasoning_effort=high",
	}

	for _, arg := range requiredArgs {
		if !strings.Contains(args, arg) {
			t.Errorf("Command args missing %q: %s", arg, args)
		}
	}

	// Check working directory
	if cmd.Dir != "/tmp" {
		t.Errorf("cmd.Dir = %q, want '/tmp'", cmd.Dir)
	}
}

func TestOpencodeAdapter_GetReasoningEffort(t *testing.T) {
	adapter := &OpencodeAdapter{}

	tests := []struct {
		input string
		want  string
	}{
		{"opencode_gpt_normal", "high"},
		{"opencode_gpt_high", "high"},
		{"opus", ""},   // Anthropic models don't use variant
		{"sonnet", ""}, // Anthropic models don't use variant
		{"haiku", ""},  // Anthropic models don't use variant
		{"custom", ""}, // Unknown models default to no variant
	}

	for _, tt := range tests {
		got := adapter.GetReasoningEffort(tt.input)
		if got != tt.want {
			t.Errorf("GetReasoningEffort(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestOpencodeAdapter_MapModel(t *testing.T) {
	adapter := &OpencodeAdapter{}

	tests := []struct {
		input string
		want  string
	}{
		{"opencode_gpt_normal", "openai/gpt-5.3-codex"},
		{"opencode_gpt_high", "openai/gpt-5.3-codex"},
		{"openai/gpt-4o", "openai/gpt-4o"}, // Already in provider/model format
		{"custom", "anthropic/custom"},       // Unknown defaults to anthropic
	}

	for _, tt := range tests {
		got := adapter.MapModel(tt.input)
		if got != tt.want {
			t.Errorf("MapModel(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestOpencodeAdapter_BuildCommand_WithVariant(t *testing.T) {
	adapter := &OpencodeAdapter{}

	opts := SpawnOptions{
		Model:         "opencode_gpt_high",
		Prompt:        "System prompt",
		InitialPrompt: "Do the task",
		WorkDir:       "/tmp",
	}

	cmd := adapter.BuildCommand(opts)

	// Check command name
	if !strings.HasSuffix(cmd.Path, "opencode") && cmd.Args[0] != "opencode" {
		t.Errorf("Expected opencode command, got %s", cmd.Path)
	}

	// Check required args are present
	args := strings.Join(cmd.Args, " ")
	requiredArgs := []string{
		"run",
		"--format", "json",
		"--model", "openai/gpt-5.3-codex",
		"--variant", "high",
	}

	for _, arg := range requiredArgs {
		if !strings.Contains(args, arg) {
			t.Errorf("Command args missing %q: %s", arg, args)
		}
	}

	// Prompt must NOT appear in args — it's piped via stdin
	if strings.Contains(args, "System prompt") || strings.Contains(args, "Do the task") {
		t.Errorf("Command args should not contain prompt text (stdin adapter): %s", args)
	}

	// Check working directory
	if cmd.Dir != "/tmp" {
		t.Errorf("cmd.Dir = %q, want '/tmp'", cmd.Dir)
	}
}

func TestOpencodeAdapter_BuildCommand_WithoutVariant(t *testing.T) {
	adapter := &OpencodeAdapter{}

	opts := SpawnOptions{
		Model:         "sonnet",
		Prompt:        "System prompt",
		InitialPrompt: "Do the task",
		WorkDir:       "/tmp",
	}

	cmd := adapter.BuildCommand(opts)

	args := strings.Join(cmd.Args, " ")

	// Should NOT contain --variant for Anthropic models
	if strings.Contains(args, "--variant") {
		t.Errorf("Command args should not contain --variant for Anthropic models: %s", args)
	}

	// Should contain correct model
	if !strings.Contains(args, "anthropic/sonnet") {
		t.Errorf("Command args missing anthropic/sonnet: %s", args)
	}

	// Prompt must NOT appear in args
	if strings.Contains(args, "System prompt") || strings.Contains(args, "Do the task") {
		t.Errorf("Command args should not contain prompt text (stdin adapter): %s", args)
	}
}

func TestUsesStdinPrompt(t *testing.T) {
	tests := []struct {
		cli  string
		want bool
	}{
		{"claude", false},
		{"opencode", true},
		{"codex", false},
	}

	for _, tt := range tests {
		adapter, err := GetCLIAdapter(tt.cli)
		if err != nil {
			t.Fatalf("GetCLIAdapter(%q) error: %v", tt.cli, err)
		}
		if got := adapter.UsesStdinPrompt(); got != tt.want {
			t.Errorf("%s.UsesStdinPrompt() = %v, want %v", tt.cli, got, tt.want)
		}
	}
}
