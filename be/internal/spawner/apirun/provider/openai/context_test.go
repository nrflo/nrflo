package openai

import "testing"

func TestMaxContext(t *testing.T) {
	p := New(Credentials{Value: "test-key"})
	for model, want := range map[string]int{
		"gpt-5.2":       400000,
		"gpt-5.3-codex": 400000,
		"gpt-5.4-mini":  400000,
		"gpt-5.4":       1050000,
		"gpt-5.5":       1050000,
		"gpt-5.6-sol":   1050000,
		"gpt-5.6-terra": 1050000,
		"gpt-5.6-luna":  1050000,
		"unknown":       128000,
	} {
		if got := p.MaxContext(model); got != want {
			t.Errorf("MaxContext(%q) = %d, want %d", model, got, want)
		}
	}
}
