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
		{"gpt_xhigh", "gpt-5.2-codex"},
		{"gpt_high", "gpt-5.2-codex"},
		{"gpt_medium", "gpt-5.2-codex"},
		{"opus", "gpt-5.2-codex"},
		{"sonnet", "gpt-5.2-codex"},
		{"haiku", "gpt-5.2-codex"},
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
		{"gpt_xhigh", "xhigh"},
		{"gpt_high", "high"},
		{"opus", "high"},
		{"gpt_medium", "medium"},
		{"sonnet", "medium"},
		{"haiku", "medium"},
		{"custom", "medium"},
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
		Model:         "gpt_high",
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
		"--model", "gpt-5.2-codex",
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
		{"gpt_max", "max"},
		{"gpt_high", "high"},
		{"gpt_medium", "medium"},
		{"gpt_low", "low"},
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
		{"opus", "anthropic/claude-opus-4-5"},
		{"sonnet", "anthropic/claude-sonnet-4-5"},
		{"haiku", "anthropic/claude-haiku-4-5"},
		{"gpt_max", "openai/gpt-5.2-codex"},
		{"gpt_high", "openai/gpt-5.2-codex"},
		{"gpt_medium", "openai/gpt-5.2-codex"},
		{"gpt_low", "openai/gpt-5.2-codex"},
		{"openai/gpt-4o", "openai/gpt-4o"}, // Already in provider/model format
		{"custom", "anthropic/custom"},     // Unknown defaults to anthropic
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
		Model:         "gpt_high",
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
		"--model", "openai/gpt-5.2-codex",
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
	if !strings.Contains(args, "anthropic/claude-sonnet-4-5") {
		t.Errorf("Command args missing anthropic/claude-sonnet-4-5: %s", args)
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
