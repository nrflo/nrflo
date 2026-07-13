package spawner

import (
	"testing"
)

// TestCodexAdapterModelMapping tests that CodexAdapter correctly maps model aliases.
func TestCodexAdapterModelMapping(t *testing.T) {
	t.Parallel()
	adapter := &CodexAdapter{}

	tests := []struct {
		input    string
		expected string
	}{
		// Predefined codex models
		{"codex_gpt_normal", "gpt-5.3-codex"},
		{"codex_gpt_high", "gpt-5.3-codex"},
		{"codex_gpt54_normal", "gpt-5.4"},
		{"codex_gpt54_high", "gpt-5.4"},
		{"codex_gpt56_sol_normal", "gpt-5.6-sol"},
		{"codex_gpt56_sol_high", "gpt-5.6-sol"},
		{"codex_gpt56_terra_normal", "gpt-5.6-terra"},
		{"codex_gpt56_terra_high", "gpt-5.6-terra"},
		{"codex_gpt56_luna_low", "gpt-5.6-luna"},

		// Unknown model (pass-through)
		{"custom-model", "custom-model"},
		{"gpt-4", "gpt-4"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := adapter.MapModel(tt.input)
			if result != tt.expected {
				t.Errorf("MapModel(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestClaudeAdapterModelMapping tests that ClaudeAdapter preserves
// the default Anthropic model names correctly.
func TestClaudeAdapterModelMapping(t *testing.T) {
	t.Parallel()
	adapter := &ClaudeAdapter{}

	tests := []struct {
		input    string
		expected string
	}{
		{"opus_4_6", "claude-opus-4-6"},
		{"opus_4_6_1m", "claude-opus-4-6[1m]"},
		{"opus_4_7", "claude-opus-4-7"},
		{"opus_4_7_1m", "claude-opus-4-7[1m]"},
		{"sonnet", "sonnet"},
		{"haiku", "haiku"},
		{"claude-opus-4-5", "claude-opus-4-5"},
		{"custom-model", "custom-model"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := adapter.MapModel(tt.input)
			if result != tt.expected {
				t.Errorf("MapModel(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestCodexReasoningEffort tests that CodexAdapter returns correct reasoning effort levels.
func TestCodexReasoningEffort(t *testing.T) {
	t.Parallel()
	adapter := &CodexAdapter{}

	tests := []struct {
		model    string
		expected string
	}{
		{"codex_gpt_normal", "high"},
		{"codex_gpt_high", "high"},
		{"codex_gpt54_normal", "medium"},
		{"codex_gpt54_high", "high"},
		{"codex_gpt56_sol_normal", "medium"},
		{"codex_gpt56_sol_high", "high"},
		{"codex_gpt56_terra_normal", "medium"},
		{"codex_gpt56_terra_high", "high"},
		{"codex_gpt56_luna_low", "low"},
		{"unknown", "high"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			result := adapter.GetReasoningEffort(tt.model)
			if result != tt.expected {
				t.Errorf("GetReasoningEffort(%q) = %q, expected %q", tt.model, result, tt.expected)
			}
		})
	}
}

// TestUnsupportedModelHandling tests how adapters handle unsupported or unknown models.
func TestUnsupportedModelHandling(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		adapter        interface{ MapModel(string) string }
		model          string
		expectNonEmpty bool
	}{
		{
			name:           "CodexAdapter with unknown model passes through",
			adapter:        &CodexAdapter{},
			model:          "unknown-xyz",
			expectNonEmpty: true,
		},
		{
			name:           "ClaudeAdapter with unknown model passes through",
			adapter:        &ClaudeAdapter{},
			model:          "unknown-xyz",
			expectNonEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.adapter.MapModel(tt.model)
			if tt.expectNonEmpty && result == "" {
				t.Errorf("expected non-empty result for unknown model, got empty string")
			}
		})
	}
}

// TestDefaultCLIForModel tests routing of model names to CLI adapters.
func TestDefaultCLIForModel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		model    string
		expected string
	}{
		{"opus_4_6", "claude"},
		{"opus_4_6_1m", "claude"},
		{"opus_4_7", "claude"},
		{"opus_4_7_1m", "claude"},
		{"sonnet", "claude"},
		{"haiku", "claude"},
		{"codex_gpt_normal", "codex"},
		{"codex_gpt_high", "codex"},
		{"codex_gpt54_normal", "codex"},
		{"codex_gpt54_high", "codex"},
		{"unknown", "claude"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			result := DefaultCLIForModel(tt.model)
			if result != tt.expected {
				t.Errorf("DefaultCLIForModel(%q) = %q, expected %q", tt.model, result, tt.expected)
			}
		})
	}
}
