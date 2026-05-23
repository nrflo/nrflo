package service

import (
	"fmt"
	"strings"
	"testing"

	"be/internal/types"
)

// TestAPIModel_CreateReasoningEffort tests all validation branches for reasoning_effort
// during API model creation, including xhigh restrictions.
//
// Note: unlike CLI models (where xhigh is allowed for non-claude types like codex),
// the API model implementation currently rejects xhigh for any non-anthropic-opus-4.7
// model. This differs from the CLI model behavior and should be reviewed.
func TestAPIModel_CreateReasoningEffort(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		provider    string
		mappedModel string
		effort      string
		wantErr     string
	}{
		{name: "empty effort anthropic", provider: "anthropic", mappedModel: "claude-sonnet-4-5", effort: ""},
		{name: "low effort anthropic", provider: "anthropic", mappedModel: "claude-sonnet-4-5", effort: "low"},
		{name: "medium effort anthropic", provider: "anthropic", mappedModel: "claude-sonnet-4-5", effort: "medium"},
		{name: "high effort anthropic opus 4.7", provider: "anthropic", mappedModel: "claude-opus-4-7", effort: "high"},
		{name: "max effort anthropic sonnet", provider: "anthropic", mappedModel: "claude-sonnet-4-5", effort: "max"},
		{name: "xhigh on anthropic opus 4.7 ok", provider: "anthropic", mappedModel: "claude-opus-4-7", effort: "xhigh"},
		{name: "xhigh on anthropic opus 4.7 1m ok", provider: "anthropic", mappedModel: "claude-opus-4-7[1m]", effort: "xhigh"},
		{name: "max on openai ok", provider: "openai", mappedModel: "gpt-5.4", effort: "max"},
		{name: "high on openai ok", provider: "openai", mappedModel: "gpt-5.3-codex", effort: "high"},
		// xhigh for openai is currently rejected (api validation requires anthropic+opus-4.7).
		{name: "xhigh on openai rejected", provider: "openai", mappedModel: "gpt-5.3-codex", effort: "xhigh", wantErr: "only supported on Anthropic Opus 4.7"},
		{name: "nonsense rejected", provider: "anthropic", mappedModel: "claude-opus-4-7", effort: "nonsense", wantErr: "must be one of low, medium, high, xhigh, max"},
		{name: "uppercase rejected", provider: "anthropic", mappedModel: "claude-opus-4-7", effort: "HIGH", wantErr: "invalid reasoning_effort"},
		{name: "xhigh on anthropic sonnet rejected", provider: "anthropic", mappedModel: "claude-sonnet-4-5", effort: "xhigh", wantErr: "only supported on Anthropic Opus 4.7"},
		{name: "xhigh on anthropic opus 4.6 rejected", provider: "anthropic", mappedModel: "claude-opus-4-6", effort: "xhigh", wantErr: "only supported on Anthropic Opus 4.7"},
		{name: "xhigh on anthropic opus 4.6 1m rejected", provider: "anthropic", mappedModel: "claude-opus-4-6[1m]", effort: "xhigh", wantErr: "only supported on Anthropic Opus 4.7"},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, cleanup := setupAPIModelTestEnv(t)
			defer cleanup()

			req := types.APIModelCreateRequest{
				ID:              fmt.Sprintf("re-api-%d", i),
				Provider:        tt.provider,
				DisplayName:     "RE Test",
				MappedModel:     tt.mappedModel,
				ReasoningEffort: tt.effort,
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
