package spawner

import (
	"strings"
	"testing"
)

func TestClaudeAdapter_BuildCommand_DisallowsInteractiveTools(t *testing.T) {
	adapter := &ClaudeAdapter{}

	opts := SpawnOptions{
		Model:         "opus_4_7",
		SessionID:     "test-session-id",
		PromptFile:    "/tmp/prompt.txt",
		InitialPrompt: "Do the task",
		WorkDir:       "/tmp",
	}

	cmd := adapter.BuildCommand(opts)
	args := strings.Join(cmd.Args, " ")

	// Interactive tools must be disallowed
	if !strings.Contains(args, "--disallowed-tools") {
		t.Errorf("Command args missing --disallowed-tools: %s", args)
	}
	for _, tool := range []string{"AskUserQuestion", "EnterPlanMode", "ExitPlanMode"} {
		if !strings.Contains(args, tool) {
			t.Errorf("Command args missing disallowed tool %q: %s", tool, args)
		}
	}

	// System prompt file and initial prompt must NOT appear (stdin adapter)
	if strings.Contains(args, "--append-system-prompt-file") {
		t.Errorf("Command args should not contain --append-system-prompt-file: %s", args)
	}
	if strings.Contains(args, "Do the task") {
		t.Errorf("Command args should not contain InitialPrompt text (stdin adapter): %s", args)
	}
}

func TestClaudeAdapter_BuildResumeCommand_DisallowsInteractiveTools(t *testing.T) {
	adapter := &ClaudeAdapter{}

	opts := ResumeOptions{
		SessionID: "test-session-id",
		Prompt:    "Continue",
		WorkDir:   "/tmp",
	}

	cmd := adapter.BuildResumeCommand(opts)
	args := strings.Join(cmd.Args, " ")

	if !strings.Contains(args, "--disallowed-tools") {
		t.Errorf("Resume command args missing --disallowed-tools: %s", args)
	}
	for _, tool := range []string{"AskUserQuestion", "EnterPlanMode", "ExitPlanMode"} {
		if !strings.Contains(args, tool) {
			t.Errorf("Resume command args missing disallowed tool %q: %s", tool, args)
		}
	}
	// Prompt must NOT appear in args — it's piped via stdin
	if strings.Contains(args, "Continue") {
		t.Errorf("Resume command args should not contain prompt text (stdin): %s", args)
	}
}

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
		"--model", "gpt-5.3-codex",
	}

	for _, arg := range requiredArgs {
		if !strings.Contains(args, arg) {
			t.Errorf("Command args missing %q: %s", arg, args)
		}
	}

	// Reasoning effort must have quoted value
	if !strings.Contains(args, `model_reasoning_effort="high"`) {
		t.Errorf("Command args missing quoted reasoning effort: %s", args)
	}

	// Removed flags must NOT be present
	removedFlags := []string{"--full-auto", "--sandbox", "--skip-git-repo-check"}
	for _, flag := range removedFlags {
		if strings.Contains(args, flag) {
			t.Errorf("Command args should not contain %q: %s", flag, args)
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

func TestOpencodeAdapter_GetReasoningEffort(t *testing.T) {
	adapter := &OpencodeAdapter{}

	tests := []struct {
		input string
		want  string
	}{
		{"opencode_gpt54", "high"},
		{"opencode_minimax_m25_free", ""},
		{"opencode_qwen36_plus_free", ""},
		{"opus_4_7", ""}, // Anthropic models don't use variant
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
		{"opencode_minimax_m25_free", "opencode/minimax-m2.5-free"},
		{"opencode_qwen36_plus_free", "opencode/qwen3.6-plus-free"},
		{"opencode_gpt54", "openai/gpt-5.4"},
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
		Model:         "opencode_gpt54",
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
		"--model", "openai/gpt-5.4",
		"--variant", "high",
	}

	for _, arg := range requiredArgs {
		if !strings.Contains(args, arg) {
			t.Errorf("Command args missing %q: %s", arg, args)
		}
	}

	// Prompt must appear as positional arg (opencode reads from args, not stdin)
	if !strings.Contains(args, "System prompt") {
		t.Errorf("Command args should contain prompt as positional arg: %s", args)
	}
	// InitialPrompt must NOT appear in args
	if strings.Contains(args, "Do the task") {
		t.Errorf("Command args should not contain InitialPrompt text: %s", args)
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

	// Prompt must appear as positional arg
	if !strings.Contains(args, "System prompt") {
		t.Errorf("Command args should contain prompt as positional arg: %s", args)
	}
}

func TestClaudeAdapter_MapModel(t *testing.T) {
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

func TestClaudeAdapter_BuildCommand_MapsModel(t *testing.T) {
	adapter := &ClaudeAdapter{}

	opts := SpawnOptions{
		Model:     "opus_4_7_1m",
		SessionID: "test-session",
		WorkDir:   "/tmp",
	}

	cmd := adapter.BuildCommand(opts)
	args := strings.Join(cmd.Args, " ")

	// Must contain the mapped model name, not the raw alias
	if !strings.Contains(args, "--model claude-opus-4-7[1m]") {
		t.Errorf("Expected --model claude-opus-4-7[1m], got: %s", args)
	}
	if strings.Contains(args, "--model opus_4_7_1m") {
		t.Errorf("Raw model name opus_4_7_1m should not appear in args: %s", args)
	}
}

func TestClaudeAdapter_BuildCommand_SettingsJSON(t *testing.T) {
	adapter := &ClaudeAdapter{}
	settingsJSON := `{"hooks":{"PreToolUse":[]}}`

	opts := SpawnOptions{
		Model:        "sonnet",
		SessionID:    "sess-1",
		WorkDir:      "/tmp",
		SettingsJSON: settingsJSON,
	}

	cmd := adapter.BuildCommand(opts)
	args := strings.Join(cmd.Args, " ")

	if !strings.Contains(args, "--settings") {
		t.Errorf("BuildCommand with SettingsJSON missing --settings flag: %s", args)
	}
	if !strings.Contains(args, settingsJSON) {
		t.Errorf("BuildCommand args missing settings JSON value: %s", args)
	}
}

func TestClaudeAdapter_BuildCommand_EmptySettingsJSON(t *testing.T) {
	adapter := &ClaudeAdapter{}

	opts := SpawnOptions{
		Model:        "sonnet",
		SessionID:    "sess-1",
		WorkDir:      "/tmp",
		SettingsJSON: "",
	}

	cmd := adapter.BuildCommand(opts)
	args := strings.Join(cmd.Args, " ")

	if strings.Contains(args, "--settings") {
		t.Errorf("BuildCommand with empty SettingsJSON should not contain --settings: %s", args)
	}
}

func TestClaudeAdapter_BuildResumeCommand_SettingsJSON(t *testing.T) {
	adapter := &ClaudeAdapter{}
	settingsJSON := `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[]}]}}`

	opts := ResumeOptions{
		SessionID:    "sess-resume",
		Prompt:       "Continue",
		WorkDir:      "/tmp",
		SettingsJSON: settingsJSON,
	}

	cmd := adapter.BuildResumeCommand(opts)
	args := strings.Join(cmd.Args, " ")

	if !strings.Contains(args, "--settings") {
		t.Errorf("BuildResumeCommand with SettingsJSON missing --settings flag: %s", args)
	}
	if !strings.Contains(args, settingsJSON) {
		t.Errorf("BuildResumeCommand args missing settings JSON value: %s", args)
	}
}

func TestClaudeAdapter_BuildResumeCommand_EmptySettingsJSON(t *testing.T) {
	adapter := &ClaudeAdapter{}

	opts := ResumeOptions{
		SessionID:    "sess-resume",
		WorkDir:      "/tmp",
		SettingsJSON: "",
	}

	cmd := adapter.BuildResumeCommand(opts)
	args := strings.Join(cmd.Args, " ")

	if strings.Contains(args, "--settings") {
		t.Errorf("BuildResumeCommand with empty SettingsJSON should not contain --settings: %s", args)
	}
}

func TestOpencodeAdapter_BuildCommand_IgnoresSettingsJSON(t *testing.T) {
	adapter := &OpencodeAdapter{}

	opts := SpawnOptions{
		Model:        "sonnet",
		WorkDir:      "/tmp",
		SettingsJSON: `{"hooks":{"PreToolUse":[]}}`,
	}

	cmd := adapter.BuildCommand(opts)
	args := strings.Join(cmd.Args, " ")

	if strings.Contains(args, "--settings") {
		t.Errorf("OpencodeAdapter.BuildCommand should ignore SettingsJSON (no --settings): %s", args)
	}
}

func TestCodexAdapter_BuildCommand_IgnoresSettingsJSON(t *testing.T) {
	adapter := &CodexAdapter{}

	opts := SpawnOptions{
		Model:        "codex_gpt_high",
		WorkDir:      "/tmp",
		SettingsJSON: `{"hooks":{"PreToolUse":[]}}`,
	}

	cmd := adapter.BuildCommand(opts)
	args := strings.Join(cmd.Args, " ")

	if strings.Contains(args, "--settings") {
		t.Errorf("CodexAdapter.BuildCommand should ignore SettingsJSON (no --settings): %s", args)
	}
}

// --- ReasoningEffort: Claude --effort flag ---

func TestClaudeAdapter_BuildCommand_ReasoningEffort(t *testing.T) {
	adapter := &ClaudeAdapter{}

	tests := []struct {
		name         string
		effort       string
		mappedModel  string
		wantContains string
		wantMissing  bool
	}{
		{name: "empty effort omits flag", effort: "", wantMissing: true},
		{name: "high effort", effort: "high", wantContains: "--effort high"},
		{name: "xhigh with opus 4.7", effort: "xhigh", mappedModel: "claude-opus-4-7", wantContains: "--effort xhigh"},
		{name: "xhigh with opus 4.7 1M", effort: "xhigh", mappedModel: "claude-opus-4-7[1m]", wantContains: "--effort xhigh"},
		{name: "max effort", effort: "max", wantContains: "--effort max"},
		{name: "low effort", effort: "low", wantContains: "--effort low"},
		{name: "medium effort", effort: "medium", wantContains: "--effort medium"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := SpawnOptions{
				Model:           "opus_4_7",
				SessionID:       "test-session",
				WorkDir:         "/tmp",
				ReasoningEffort: tt.effort,
				MappedModel:     tt.mappedModel,
			}
			cmd := adapter.BuildCommand(opts)
			args := strings.Join(cmd.Args, " ")

			if tt.wantMissing {
				if strings.Contains(args, "--effort") {
					t.Errorf("BuildCommand with empty ReasoningEffort should not contain --effort: %s", args)
				}
				return
			}
			if !strings.Contains(args, tt.wantContains) {
				t.Errorf("BuildCommand args missing %q: %s", tt.wantContains, args)
			}
		})
	}
}

func TestClaudeAdapter_BuildCommand_EffortAdjacentToModel(t *testing.T) {
	adapter := &ClaudeAdapter{}
	opts := SpawnOptions{
		Model:           "opus_4_7",
		SessionID:       "test-session",
		WorkDir:         "/tmp",
		ReasoningEffort: "high",
		SettingsJSON:    `{"hooks":{}}`,
	}
	cmd := adapter.BuildCommand(opts)
	args := cmd.Args

	// Locate --model, --effort, --settings indices.
	modelIdx, effortIdx, settingsIdx := -1, -1, -1
	for i, a := range args {
		switch a {
		case "--model":
			modelIdx = i
		case "--effort":
			effortIdx = i
		case "--settings":
			settingsIdx = i
		}
	}
	if modelIdx < 0 || effortIdx < 0 || settingsIdx < 0 {
		t.Fatalf("expected --model, --effort, --settings in args: %v", args)
	}
	// --effort must sit between --model and --settings so effort is not visually buried by large JSON.
	if !(modelIdx < effortIdx && effortIdx < settingsIdx) {
		t.Errorf("expected --model(%d) < --effort(%d) < --settings(%d) ordering: %v", modelIdx, effortIdx, settingsIdx, args)
	}
}

func TestClaudeAdapter_BuildResumeCommand_ReasoningEffort(t *testing.T) {
	adapter := &ClaudeAdapter{}

	tests := []struct {
		name         string
		effort       string
		wantContains string
		wantMissing  bool
	}{
		{name: "empty effort omits flag", effort: "", wantMissing: true},
		{name: "high effort", effort: "high", wantContains: "--effort high"},
		{name: "xhigh effort", effort: "xhigh", wantContains: "--effort xhigh"},
		{name: "max effort", effort: "max", wantContains: "--effort max"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := ResumeOptions{
				SessionID:       "sess-resume",
				Prompt:          "Continue",
				WorkDir:         "/tmp",
				ReasoningEffort: tt.effort,
			}
			cmd := adapter.BuildResumeCommand(opts)
			args := strings.Join(cmd.Args, " ")

			if tt.wantMissing {
				if strings.Contains(args, "--effort") {
					t.Errorf("BuildResumeCommand with empty ReasoningEffort should not contain --effort: %s", args)
				}
				return
			}
			if !strings.Contains(args, tt.wantContains) {
				t.Errorf("BuildResumeCommand args missing %q: %s", tt.wantContains, args)
			}
		})
	}
}

func TestOpencodeAdapter_BuildCommand_IgnoresReasoningEffortField(t *testing.T) {
	// OpencodeAdapter uses its own --variant logic driven by MapModel + GetReasoningEffort.
	// The ClaudeAdapter-specific --effort flag must not leak into opencode argv.
	adapter := &OpencodeAdapter{}
	opts := SpawnOptions{
		Model:           "sonnet",
		WorkDir:         "/tmp",
		ReasoningEffort: "xhigh", // intentionally a Claude-only value
	}
	cmd := adapter.BuildCommand(opts)
	args := strings.Join(cmd.Args, " ")

	if strings.Contains(args, "--effort") {
		t.Errorf("OpencodeAdapter.BuildCommand should not emit --effort: %s", args)
	}
}

func TestCodexAdapter_BuildCommand_IgnoresEffortFlag(t *testing.T) {
	// CodexAdapter exposes reasoning effort via -c model_reasoning_effort="..." not --effort.
	adapter := &CodexAdapter{}
	opts := SpawnOptions{
		Model:           "codex_gpt_high",
		WorkDir:         "/tmp",
		ReasoningEffort: "xhigh",
	}
	cmd := adapter.BuildCommand(opts)
	args := strings.Join(cmd.Args, " ")

	if strings.Contains(args, "--effort") {
		t.Errorf("CodexAdapter.BuildCommand should not emit --effort: %s", args)
	}
}

func TestUsesStdinPrompt(t *testing.T) {
	tests := []struct {
		cli  string
		want bool
	}{
		{"claude", true},
		{"opencode", false},
		{"codex", true},
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
