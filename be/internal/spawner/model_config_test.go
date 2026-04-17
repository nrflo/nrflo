package spawner

import (
	"strings"
	"testing"
)

// =============================================================================
// cliForModel tests
// =============================================================================

func TestCLIForModel_DBConfigTakesPriority(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		configs map[string]ModelConfig
		want    string
	}{
		{
			name:  "returns DB CLIType for known model",
			model: "my-custom-model",
			configs: map[string]ModelConfig{
				"my-custom-model": {CLIType: "opencode"},
			},
			want: "opencode",
		},
		{
			name:  "DB codex type overrides claude default",
			model: "opus_4_7",
			configs: map[string]ModelConfig{
				"opus_4_7": {CLIType: "codex"},
			},
			want: "codex",
		},
		{
			name:  "DB claude type for opencode-prefixed model overrides default",
			model: "opencode_minimax_m25_free",
			configs: map[string]ModelConfig{
				"opencode_minimax_m25_free": {CLIType: "claude"},
			},
			want: "claude",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Spawner{config: Config{ModelConfigs: tt.configs}}
			got := s.cliForModel(tt.model)
			if got != tt.want {
				t.Errorf("cliForModel(%q) = %q, want %q", tt.model, got, tt.want)
			}
		})
	}
}

func TestCLIForModel_FallbackWhenNilMap(t *testing.T) {
	// nil map must not panic; should fall back to DefaultCLIForModel
	s := &Spawner{config: Config{}}

	tests := []struct {
		model string
		want  string
	}{
		{"opus_4_7", "claude"},
		{"opus_4_7_1m", "claude"},
		{"sonnet", "claude"},
		{"opencode_minimax_m25_free", "opencode"},
		{"opencode_qwen36_plus_free", "opencode"},
		{"opencode_gpt54", "opencode"},
		{"codex_gpt_normal", "codex"},
		{"codex_gpt54_high", "codex"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := s.cliForModel(tt.model)
			if got != tt.want {
				t.Errorf("cliForModel(%q) = %q, want %q (nil map fallback)", tt.model, got, tt.want)
			}
		})
	}
}

func TestCLIForModel_FallbackWhenEmptyCLIType(t *testing.T) {
	// Config entry present but CLIType is empty string → fallback
	s := &Spawner{config: Config{
		ModelConfigs: map[string]ModelConfig{
			"opus_4_7": {CLIType: "", ContextLength: 500000}, // CLIType deliberately empty
		},
	}}

	got := s.cliForModel("opus_4_7")
	if got != "claude" {
		t.Errorf("cliForModel(%q) = %q, want 'claude' (empty CLIType falls back)", "opus_4_7", got)
	}
}

func TestCLIForModel_FallbackWhenModelNotInMap(t *testing.T) {
	// Map populated but this specific model is absent
	s := &Spawner{config: Config{
		ModelConfigs: map[string]ModelConfig{
			"other-model": {CLIType: "opencode"},
		},
	}}

	got := s.cliForModel("codex_gpt_high")
	if got != "codex" {
		t.Errorf("cliForModel(%q) = %q, want 'codex' (model not in map)", "codex_gpt_high", got)
	}
}

// =============================================================================
// maxContextForModel tests
// =============================================================================

func TestMaxContextForModel_DBConfigTakesPriority(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		configs map[string]ModelConfig
		want    int
	}{
		{
			name:  "custom context length from DB",
			model: "opus_4_7",
			configs: map[string]ModelConfig{
				"opus_4_7": {ContextLength: 500000},
			},
			want: 500000,
		},
		{
			name:  "DB overrides opus_4_7_1m hardcoded 1M",
			model: "opus_4_7_1m",
			configs: map[string]ModelConfig{
				"opus_4_7_1m": {ContextLength: 2000000},
			},
			want: 2000000,
		},
		{
			name:  "DB value 200001 for custom model",
			model: "custom-model",
			configs: map[string]ModelConfig{
				"custom-model": {ContextLength: 200001},
			},
			want: 200001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Spawner{config: Config{ModelConfigs: tt.configs}}
			got := s.maxContextForModel(tt.model)
			if got != tt.want {
				t.Errorf("maxContextForModel(%q) = %d, want %d", tt.model, got, tt.want)
			}
		})
	}
}

func TestMaxContextForModel_ZeroContextLengthFallsBack(t *testing.T) {
	// ContextLength of 0 means "not configured" — should fall back to hardcoded
	s := &Spawner{config: Config{
		ModelConfigs: map[string]ModelConfig{
			"opus_4_7_1m": {ContextLength: 0, CLIType: "claude"},
			"opus_4_7":    {ContextLength: 0},
		},
	}}

	if got := s.maxContextForModel("opus_4_7_1m"); got != 1000000 {
		t.Errorf("maxContextForModel(opus_4_7_1m) = %d, want 1000000 (zero ContextLength falls back)", got)
	}
	if got := s.maxContextForModel("opus_4_7"); got != 200000 {
		t.Errorf("maxContextForModel(opus_4_7) = %d, want 200000 (zero ContextLength falls back)", got)
	}
}

func TestMaxContextForModel_HardcodedFallback(t *testing.T) {
	// nil ModelConfigs → pure hardcoded logic
	s := &Spawner{config: Config{}}

	tests := []struct {
		model string
		want  int
	}{
		{"opus_4_7_1m", 1000000},
		{"opus_4_6_1m", 1000000},
		{"opus_4_7", 200000},
		{"opus_4_6", 200000},
		{"sonnet", 200000},
		{"haiku", 200000},
		{"opencode_minimax_m25_free", 200000},
		{"codex_gpt_high", 200000},
		{"unknown-model", 200000},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := s.maxContextForModel(tt.model)
			if got != tt.want {
				t.Errorf("maxContextForModel(%q) = %d, want %d (hardcoded fallback)", tt.model, got, tt.want)
			}
		})
	}
}

// =============================================================================
// Adapter BuildCommand with opts.MappedModel / opts.ReasoningEffort
// =============================================================================

func TestClaudeAdapter_BuildCommand_UsesMappedModelFromOpts(t *testing.T) {
	adapter := &ClaudeAdapter{}

	t.Run("opts.MappedModel used instead of MapModel", func(t *testing.T) {
		opts := SpawnOptions{
			Model:       "opus_4_7_1m",
			MappedModel: "claude-opus-4-7[1m]",
			SessionID:   "s1",
			WorkDir:     "/tmp",
		}
		cmd := adapter.BuildCommand(opts)
		args := strings.Join(cmd.Args, " ")

		if !strings.Contains(args, "--model claude-opus-4-7[1m]") {
			t.Errorf("expected --model claude-opus-4-7[1m], got: %s", args)
		}
	})

	t.Run("opts.MappedModel overrides MapModel result with custom name", func(t *testing.T) {
		opts := SpawnOptions{
			Model:       "opus_4_7_1m",
			MappedModel: "claude-opus-db-override",
			SessionID:   "s2",
			WorkDir:     "/tmp",
		}
		cmd := adapter.BuildCommand(opts)
		args := strings.Join(cmd.Args, " ")

		if !strings.Contains(args, "--model claude-opus-db-override") {
			t.Errorf("expected --model claude-opus-db-override, got: %s", args)
		}
		if strings.Contains(args, "claude-opus-4-7[1m]") {
			t.Errorf("MapModel result should not appear when MappedModel is set: %s", args)
		}
	})

	t.Run("empty MappedModel falls back to MapModel", func(t *testing.T) {
		opts := SpawnOptions{
			Model:       "opus_4_7_1m",
			MappedModel: "",
			SessionID:   "s3",
			WorkDir:     "/tmp",
		}
		cmd := adapter.BuildCommand(opts)
		args := strings.Join(cmd.Args, " ")

		// MapModel("opus_4_7_1m") → "claude-opus-4-7[1m]"
		if !strings.Contains(args, "--model claude-opus-4-7[1m]") {
			t.Errorf("expected --model claude-opus-4-7[1m] from MapModel fallback, got: %s", args)
		}
	})
}

func TestOpencodeAdapter_BuildCommand_UsesMappedModelAndEffortFromOpts(t *testing.T) {
	adapter := &OpencodeAdapter{}

	t.Run("opts.MappedModel and opts.ReasoningEffort override adapter methods", func(t *testing.T) {
		opts := SpawnOptions{
			Model:           "opencode_gpt54",
			MappedModel:     "openai/gpt-custom",
			ReasoningEffort: "medium",
			WorkDir:         "/tmp",
		}
		cmd := adapter.BuildCommand(opts)
		args := strings.Join(cmd.Args, " ")

		if !strings.Contains(args, "--model openai/gpt-custom") {
			t.Errorf("expected --model openai/gpt-custom, got: %s", args)
		}
		if !strings.Contains(args, "--variant medium") {
			t.Errorf("expected --variant medium, got: %s", args)
		}
		if strings.Contains(args, "openai/gpt-5.3-codex") {
			t.Errorf("hardcoded model should not appear when MappedModel is set: %s", args)
		}
	})

	t.Run("empty opts fields fall back to adapter methods", func(t *testing.T) {
		opts := SpawnOptions{
			Model:   "opencode_minimax_m25_free",
			WorkDir: "/tmp",
		}
		cmd := adapter.BuildCommand(opts)
		args := strings.Join(cmd.Args, " ")

		// Fallback: MapModel → opencode/minimax-m2.5-free, GetReasoningEffort → "" (no variant)
		if !strings.Contains(args, "--model opencode/minimax-m2.5-free") {
			t.Errorf("expected --model opencode/minimax-m2.5-free from MapModel fallback, got: %s", args)
		}
		if strings.Contains(args, "--variant") {
			t.Errorf("expected no --variant for minimax free model, got: %s", args)
		}
	})

	t.Run("MappedModel set but ReasoningEffort empty uses adapter GetReasoningEffort", func(t *testing.T) {
		opts := SpawnOptions{
			Model:       "opencode_gpt54",
			MappedModel: "openai/gpt-custom",
			WorkDir:     "/tmp",
		}
		cmd := adapter.BuildCommand(opts)
		args := strings.Join(cmd.Args, " ")

		if !strings.Contains(args, "--model openai/gpt-custom") {
			t.Errorf("expected --model openai/gpt-custom, got: %s", args)
		}
		// GetReasoningEffort("opencode_gpt54") returns "high"
		if !strings.Contains(args, "--variant high") {
			t.Errorf("expected --variant high from GetReasoningEffort, got: %s", args)
		}
	})
}

func TestCodexAdapter_BuildCommand_UsesMappedModelAndEffortFromOpts(t *testing.T) {
	adapter := &CodexAdapter{}

	t.Run("opts.MappedModel and opts.ReasoningEffort override adapter methods", func(t *testing.T) {
		opts := SpawnOptions{
			Model:           "codex_gpt_normal",
			MappedModel:     "gpt-db-override",
			ReasoningEffort: "low",
			WorkDir:         "/tmp",
		}
		cmd := adapter.BuildCommand(opts)
		args := strings.Join(cmd.Args, " ")

		if !strings.Contains(args, "--model gpt-db-override") {
			t.Errorf("expected --model gpt-db-override, got: %s", args)
		}
		if !strings.Contains(args, `model_reasoning_effort="low"`) {
			t.Errorf(`expected model_reasoning_effort="low", got: %s`, args)
		}
		if strings.Contains(args, "gpt-5.3-codex") {
			t.Errorf("hardcoded model should not appear when MappedModel is set: %s", args)
		}
	})

	t.Run("empty opts fields fall back to adapter methods", func(t *testing.T) {
		opts := SpawnOptions{
			Model:   "codex_gpt_high",
			WorkDir: "/tmp",
		}
		cmd := adapter.BuildCommand(opts)
		args := strings.Join(cmd.Args, " ")

		// Fallback: MapModel → gpt-5.3-codex, GetReasoningEffort → high
		if !strings.Contains(args, "--model gpt-5.3-codex") {
			t.Errorf("expected --model gpt-5.3-codex from MapModel fallback, got: %s", args)
		}
		if !strings.Contains(args, `model_reasoning_effort="high"`) {
			t.Errorf(`expected model_reasoning_effort="high" from GetReasoningEffort fallback, got: %s`, args)
		}
	})

	t.Run("opts.ReasoningEffort empty string with non-empty MappedModel uses GetReasoningEffort", func(t *testing.T) {
		opts := SpawnOptions{
			Model:       "codex_gpt54_normal",
			MappedModel: "gpt-5.4",
			WorkDir:     "/tmp",
		}
		cmd := adapter.BuildCommand(opts)
		args := strings.Join(cmd.Args, " ")

		// GetReasoningEffort("codex_gpt54_normal") returns "medium"
		if !strings.Contains(args, `model_reasoning_effort="medium"`) {
			t.Errorf(`expected model_reasoning_effort="medium", got: %s`, args)
		}
	})
}

// =============================================================================
// SpawnOptions zero-value safety (no panics with empty struct)
// =============================================================================

func TestSpawnOptions_ZeroValueDoesNotPanic(t *testing.T) {
	// Ensure BuildCommand on each adapter handles zero-value SpawnOptions without panicking.
	for _, cli := range []string{"claude", "opencode", "codex"} {
		adapter, err := GetCLIAdapter(cli)
		if err != nil {
			t.Fatalf("GetCLIAdapter(%q): %v", cli, err)
		}
		var opts SpawnOptions
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s.BuildCommand(zero SpawnOptions) panicked: %v", cli, r)
				}
			}()
			_ = adapter.BuildCommand(opts)
		}()
	}
}
