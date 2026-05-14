package spawner

import (
	"testing"
)

// =============================================================================
// cliForModel tests
// =============================================================================

func TestCLIForModel_DBConfigTakesPriority(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

