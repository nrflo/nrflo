package anthropic

import (
	"testing"
)

func TestAnthropicProvider_Name(t *testing.T) {
	p := New(Credentials{Value: "test-key", Method: MethodAPIKey})
	if got := p.Name(); got != "anthropic" {
		t.Errorf("Name() = %q, want %q", got, "anthropic")
	}
}

func TestMaxContext(t *testing.T) {
	p := New(Credentials{Value: "test-key", Method: MethodAPIKey})
	tests := []struct {
		model string
		want  int
	}{
		{"claude-opus-4-7", 200000},
		{"claude-opus-4-7[1m]", 1000000},
		{"claude-opus-4-6", 200000},
		{"claude-opus-4-6[1m]", 1000000},
		{"claude-sonnet-4-6", 200000},
		{"claude-sonnet-4-7", 200000},
		{"claude-haiku-4-5", 200000},
		{"unknown-model", 200000},
		{"", 200000},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			if got := p.MaxContext(tt.model); got != tt.want {
				t.Errorf("MaxContext(%q) = %d, want %d", tt.model, got, tt.want)
			}
		})
	}
}
