package spawner

import "testing"

func TestCLIForModel_DerivesFromProvider(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		model    string
		configs  map[string]ModelConfig
		expected string
	}{
		{"anthropic", "opus-4-8", map[string]ModelConfig{"opus-4-8": {Provider: "anthropic"}}, "claude"},
		{"openai", "gpt-5.6-sol", map[string]ModelConfig{"gpt-5.6-sol": {Provider: "openai"}}, "codex"},
		{"raw passthrough", "custom-model", nil, "claude"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Spawner{config: Config{ModelConfigs: tt.configs}}
			if got := s.cliForModel(tt.model); got != tt.expected {
				t.Fatalf("cliForModel(%q) = %q, want %q", tt.model, got, tt.expected)
			}
		})
	}
}

func TestMaxContextForModel_UsesCLIContext(t *testing.T) {
	t.Parallel()
	s := &Spawner{config: Config{ModelConfigs: map[string]ModelConfig{
		"opus-4-8-1m": {CLIContext: 1000000},
		"gpt-5.6-sol": {CLIContext: 272000},
	}}}
	if got := s.maxContextForModel("opus-4-8-1m"); got != 1000000 {
		t.Fatalf("max context = %d, want 1000000", got)
	}
	if got := s.maxContextForModel("gpt-5.6-sol"); got != 272000 {
		t.Fatalf("max context = %d, want 272000", got)
	}
	if got := s.maxContextForModel("raw-model"); got != 200000 {
		t.Fatalf("raw max context = %d, want 200000", got)
	}
}
