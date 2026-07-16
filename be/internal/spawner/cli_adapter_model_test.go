package spawner

import (
	"testing"
)

func TestCodexAdapterModelPassthrough(t *testing.T) {
	t.Parallel()
	adapter := &CodexAdapter{}

	tests := []struct {
		input    string
		expected string
	}{
		{"gpt-5.6-sol", "gpt-5.6-sol"},
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
		{"claude-opus-4-8", "claude-opus-4-8"},
		{"claude-opus-4-8[1m]", "claude-opus-4-8[1m]"},
		{"claude-sonnet-5", "claude-sonnet-5"},
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
