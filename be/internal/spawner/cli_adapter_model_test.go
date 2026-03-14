package spawner

import (
	"testing"
)

// TestOpencodeAdapterModelMapping tests that OpencodeAdapter correctly maps model aliases.
func TestOpencodeAdapterModelMapping(t *testing.T) {
	adapter := &OpencodeAdapter{}

	tests := []struct {
		input    string
		expected string
	}{
		// New predefined opencode models
		{"opencode_gpt_normal", "openai/gpt-5.3-codex"},
		{"opencode_gpt_high", "openai/gpt-5.3-codex"},

		// Already provider/model format (pass-through)
		{"anthropic/claude-opus-4-5", "anthropic/claude-opus-4-5"},
		{"openai/gpt-5.3-codex", "openai/gpt-5.3-codex"},
		{"custom/my-model", "custom/my-model"},

		// Unknown model (should default to anthropic provider)
		{"unknown-model", "anthropic/unknown-model"},
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

// TestCodexAdapterModelMapping tests that CodexAdapter correctly maps model aliases.
func TestCodexAdapterModelMapping(t *testing.T) {
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
	adapter := &ClaudeAdapter{}

	tests := []struct {
		input    string
		expected string
	}{
		{"opus", "opus"},
		{"opus_1m", "opus[1m]"},
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

// TestOpencodeReasoningEffort tests that OpencodeAdapter returns correct reasoning effort variants.
func TestOpencodeReasoningEffort(t *testing.T) {
	adapter := &OpencodeAdapter{}

	tests := []struct {
		model    string
		expected string
	}{
		{"opencode_gpt_normal", "high"},
		{"opencode_gpt_high", "high"},
		{"opus", ""},
		{"sonnet", ""},
		{"haiku", ""},
		{"unknown", ""},
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

// TestCodexReasoningEffort tests that CodexAdapter returns correct reasoning effort levels.
func TestCodexReasoningEffort(t *testing.T) {
	adapter := &CodexAdapter{}

	tests := []struct {
		model    string
		expected string
	}{
		{"codex_gpt_normal", "high"},
		{"codex_gpt_high", "high"},
		{"codex_gpt54_normal", "medium"},
		{"codex_gpt54_high", "high"},
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

// TestModelMappingRoundTrip tests that model names can be used in workflow definitions
// and correctly resolve to their provider-specific formats.
func TestModelMappingRoundTrip(t *testing.T) {
	// Each model should only be tested with its correct adapter
	tests := []struct {
		model   string
		adapter interface{ MapModel(string) string }
		name    string
	}{
		{"opus", &ClaudeAdapter{}, "ClaudeAdapter"},
		{"opus_1m", &ClaudeAdapter{}, "ClaudeAdapter"},
		{"sonnet", &ClaudeAdapter{}, "ClaudeAdapter"},
		{"haiku", &ClaudeAdapter{}, "ClaudeAdapter"},
		{"opencode_gpt_normal", &OpencodeAdapter{}, "OpencodeAdapter"},
		{"opencode_gpt_high", &OpencodeAdapter{}, "OpencodeAdapter"},
		{"codex_gpt_normal", &CodexAdapter{}, "CodexAdapter"},
		{"codex_gpt_high", &CodexAdapter{}, "CodexAdapter"},
		{"codex_gpt54_normal", &CodexAdapter{}, "CodexAdapter"},
		{"codex_gpt54_high", &CodexAdapter{}, "CodexAdapter"},
	}

	for _, tt := range tests {
		t.Run(tt.name+"_"+tt.model, func(t *testing.T) {
			result := tt.adapter.MapModel(tt.model)
			if result == "" {
				t.Errorf("%s.MapModel(%q) returned empty string", tt.name, tt.model)
			}
		})
	}
}

// TestUnsupportedModelHandling tests how adapters handle unsupported or unknown models.
func TestUnsupportedModelHandling(t *testing.T) {
	tests := []struct {
		name           string
		adapter        interface{ MapModel(string) string }
		model          string
		expectNonEmpty bool
	}{
		{
			name:           "OpencodeAdapter with unknown model defaults to anthropic",
			adapter:        &OpencodeAdapter{},
			model:          "unknown-xyz",
			expectNonEmpty: true,
		},
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
	tests := []struct {
		model    string
		expected string
	}{
		{"opus", "claude"},
		{"opus_1m", "claude"},
		{"sonnet", "claude"},
		{"haiku", "claude"},
		{"opencode_gpt_normal", "opencode"},
		{"opencode_gpt_high", "opencode"},
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

// TestAllSupportedModelsAreValid tests that all supported model aliases
// are properly handled by their respective adapters.
func TestAllSupportedModelsAreValid(t *testing.T) {
	tests := []struct {
		model   string
		adapter interface{ MapModel(string) string }
		name    string
	}{
		{"opus", &ClaudeAdapter{}, "ClaudeAdapter"},
		{"opus_1m", &ClaudeAdapter{}, "ClaudeAdapter"},
		{"sonnet", &ClaudeAdapter{}, "ClaudeAdapter"},
		{"haiku", &ClaudeAdapter{}, "ClaudeAdapter"},
		{"opencode_gpt_normal", &OpencodeAdapter{}, "OpencodeAdapter"},
		{"opencode_gpt_high", &OpencodeAdapter{}, "OpencodeAdapter"},
		{"codex_gpt_normal", &CodexAdapter{}, "CodexAdapter"},
		{"codex_gpt_high", &CodexAdapter{}, "CodexAdapter"},
		{"codex_gpt54_normal", &CodexAdapter{}, "CodexAdapter"},
		{"codex_gpt54_high", &CodexAdapter{}, "CodexAdapter"},
	}

	for _, tt := range tests {
		t.Run(tt.name+"_"+tt.model, func(t *testing.T) {
			result := tt.adapter.MapModel(tt.model)
			if result == "" {
				t.Errorf("%s.MapModel(%q) returned empty string", tt.name, tt.model)
			}
		})
	}
}
