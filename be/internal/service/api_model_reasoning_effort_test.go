package service

import (
	"fmt"
	"strings"
	"testing"

	"be/internal/types"
)

// TestAPIModel_CreateReasoningEffort tests reasoning_effort validation during
// API model creation. Capability is driven by the row's supported_efforts list,
// not the provider/mapped_model name.
func TestAPIModel_CreateReasoningEffort(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		provider  string
		supported []string
		effort    string
		wantErr   string
	}{
		{name: "empty effort", provider: "anthropic", effort: ""},
		{name: "low defaults list", provider: "anthropic", effort: "low"},
		{name: "high in list", provider: "anthropic", supported: []string{"low", "medium", "high"}, effort: "high"},
		{name: "xhigh in list", provider: "anthropic", supported: []string{"low", "high", "xhigh"}, effort: "xhigh"},
		{name: "max in openai list", provider: "openai", supported: []string{"low", "high", "max"}, effort: "max"},
		{name: "nonsense rejected", provider: "anthropic", effort: "nonsense", wantErr: "must be one of low, medium, high, xhigh, max"},
		{name: "uppercase rejected", provider: "anthropic", effort: "HIGH", wantErr: "invalid reasoning_effort"},
		{name: "xhigh outside list rejected", provider: "anthropic", supported: []string{"low", "medium", "high"}, effort: "xhigh", wantErr: "is not supported by this model"},
		{name: "invalid supported entry rejected", provider: "openai", supported: []string{"low", "zzz"}, effort: "low", wantErr: "invalid supported_efforts entry"},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, cleanup := setupAPIModelTestEnv(t)
			defer cleanup()

			req := types.APIModelCreateRequest{
				ID:               fmt.Sprintf("re-api-%d", i),
				Provider:         tt.provider,
				DisplayName:      "RE Test",
				MappedModel:      "some-model",
				ReasoningEffort:  tt.effort,
				SupportedEfforts: tt.supported,
			}
			m, err := svc.Create(req)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Create: unexpected error: %v", err)
				}
				if m.ReasoningEffort != tt.effort {
					t.Errorf("ReasoningEffort = %q, want %q", m.ReasoningEffort, tt.effort)
				}
				return
			}
			if err == nil {
				t.Fatalf("Create: expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}
