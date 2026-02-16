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
		// Standard Anthropic models
		{"opus", "anthropic/claude-opus-4-5"},
		{"sonnet", "anthropic/claude-sonnet-4-5"},
		{"haiku", "anthropic/claude-haiku-4-5"},

		// GPT models
		{"gpt_5.3", "openai/gpt-5.3"},
		{"gpt_max", "openai/gpt-5.2-codex"},
		{"gpt_high", "openai/gpt-5.2-codex"},
		{"gpt_medium", "openai/gpt-5.2-codex"},
		{"gpt_low", "openai/gpt-5.2-codex"},

		// Already provider/model format (pass-through)
		{"anthropic/claude-opus-4-5", "anthropic/claude-opus-4-5"},
		{"openai/gpt-5.3", "openai/gpt-5.3"},
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
		// Anthropic models mapped to gpt-5.2-codex
		{"opus", "gpt-5.2-codex"},
		{"sonnet", "gpt-5.2-codex"},
		{"haiku", "gpt-5.2-codex"},

		// GPT models
		{"gpt_5.3", "gpt-5.3"},
		{"gpt_xhigh", "gpt-5.2-codex"},
		{"gpt_high", "gpt-5.2-codex"},
		{"gpt_medium", "gpt-5.2-codex"},

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

// TestGPT53ModelSupport tests that gpt_5.3 is correctly mapped in both adapters.
func TestGPT53ModelSupport(t *testing.T) {
	tests := []struct {
		name     string
		adapter  interface{ MapModel(string) string }
		expected string
	}{
		{
			name:     "OpencodeAdapter maps gpt_5.3",
			adapter:  &OpencodeAdapter{},
			expected: "openai/gpt-5.3",
		},
		{
			name:     "CodexAdapter maps gpt_5.3",
			adapter:  &CodexAdapter{},
			expected: "gpt-5.3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.adapter.MapModel("gpt_5.3")
			if result != tt.expected {
				t.Errorf("%s: expected %q, got %q", tt.name, tt.expected, result)
			}
		})
	}
}

// TestClaudeAdapterModelMapping tests that ClaudeAdapter (if implemented) preserves
// the default Anthropic model names correctly.
func TestClaudeAdapterModelMapping(t *testing.T) {
	adapter := &ClaudeAdapter{}

	tests := []struct {
		input    string
		expected string
	}{
		{"opus", "opus"},
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
		{"gpt_max", "max"},
		{"gpt_high", "high"},
		{"gpt_medium", "medium"},
		{"gpt_low", "low"},
		{"opus", ""},
		{"sonnet", ""},
		{"haiku", ""},
		{"gpt_5.3", ""},
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
		{"gpt_xhigh", "xhigh"},
		{"gpt_high", "high"},
		{"gpt_medium", "medium"},
		{"opus", "high"},        // Codex maps opus to high
		{"sonnet", "medium"},     // Codex maps sonnet to medium
		{"haiku", "medium"},      // Codex maps haiku to medium
		{"gpt_5.3", "high"},      // Codex maps gpt_5.3 to high
		{"unknown", "medium"},    // Codex defaults to medium
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
	// This test simulates how the orchestrator would use model mappings
	// when spawning agents with different CLI adapters.

	models := []string{"opus", "sonnet", "haiku", "gpt_5.3"}

	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			// Opencode adapter
			opencodeAdapter := &OpencodeAdapter{}
			opencodeModel := opencodeAdapter.MapModel(model)

			// Should produce a valid provider/model format or known model name
			if opencodeModel == "" {
				t.Errorf("OpencodeAdapter.MapModel(%q) returned empty string", model)
			}

			// Codex adapter
			codexAdapter := &CodexAdapter{}
			codexModel := codexAdapter.MapModel(model)

			// Should produce a valid model name
			if codexModel == "" {
				t.Errorf("CodexAdapter.MapModel(%q) returned empty string", model)
			}

			// Claude adapter (pass-through for Anthropic models)
			claudeAdapter := &ClaudeAdapter{}
			claudeModel := claudeAdapter.MapModel(model)

			// Should preserve the input (or map to full name for Anthropic)
			if claudeModel == "" {
				t.Errorf("ClaudeAdapter.MapModel(%q) returned empty string", model)
			}
		})
	}
}

// TestUnsupportedModelHandling tests how adapters handle unsupported or unknown models.
func TestUnsupportedModelHandling(t *testing.T) {
	tests := []struct {
		name          string
		adapter       interface{ MapModel(string) string }
		model         string
		expectNonEmpty bool
	}{
		{
			name:          "OpencodeAdapter with unknown model defaults to anthropic",
			adapter:       &OpencodeAdapter{},
			model:         "unknown-xyz",
			expectNonEmpty: true,
		},
		{
			name:          "CodexAdapter with unknown model passes through",
			adapter:       &CodexAdapter{},
			model:         "unknown-xyz",
			expectNonEmpty: true,
		},
		{
			name:          "ClaudeAdapter with unknown model passes through",
			adapter:       &ClaudeAdapter{},
			model:         "unknown-xyz",
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

// TestDefaultCLIForModel tests that GPT models route to opencode and Anthropic models route to claude.
func TestDefaultCLIForModel(t *testing.T) {
	tests := []struct {
		model    string
		expected string
	}{
		{"opus", "claude"},
		{"sonnet", "claude"},
		{"haiku", "claude"},
		{"gpt_5.3", "opencode"},
		{"gpt_max", "opencode"},
		{"gpt_high", "opencode"},
		{"gpt_medium", "opencode"},
		{"gpt_low", "opencode"},
		{"gpt-5.2-codex", "opencode"},
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
// from the ticket (opus, sonnet, haiku, gpt_5.3) are properly handled.
func TestAllSupportedModelsAreValid(t *testing.T) {
	supportedModels := []string{"opus", "sonnet", "haiku", "gpt_5.3"}

	adapters := []struct {
		name    string
		adapter interface{ MapModel(string) string }
	}{
		{"OpencodeAdapter", &OpencodeAdapter{}},
		{"CodexAdapter", &CodexAdapter{}},
		{"ClaudeAdapter", &ClaudeAdapter{}},
	}

	for _, adapter := range adapters {
		for _, model := range supportedModels {
			t.Run(adapter.name+"_"+model, func(t *testing.T) {
				result := adapter.adapter.MapModel(model)
				if result == "" {
					t.Errorf("%s.MapModel(%q) returned empty string", adapter.name, model)
				}

				// For gpt_5.3, verify it doesn't get mapped to something else incorrectly
				if model == "gpt_5.3" {
					if adapter.name == "OpencodeAdapter" && result != "openai/gpt-5.3" {
						t.Errorf("%s.MapModel('gpt_5.3') = %q, expected 'openai/gpt-5.3'", adapter.name, result)
					}
					if adapter.name == "CodexAdapter" && result != "gpt-5.3" {
						t.Errorf("%s.MapModel('gpt_5.3') = %q, expected 'gpt-5.3'", adapter.name, result)
					}
				}
			})
		}
	}
}
