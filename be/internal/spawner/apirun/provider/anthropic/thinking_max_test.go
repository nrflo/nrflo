package anthropic

import (
	"strings"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"

	"be/internal/spawner/apirun/provider"
)

// TestTranslateRequest_MaxEffort_AdaptiveAndBudget verifies the "max" effort
// maps to the SDK OutputConfig effort "max" on an adaptive model and to a
// 32768-token budget on Haiku 4.5 (budget path).
func TestTranslateRequest_MaxEffort_AdaptiveAndBudget(t *testing.T) {
	t.Run("adaptive_opus-4-8", func(t *testing.T) {
		params, err := translateRequest(provider.Request{
			Model:           "claude-opus-4-8",
			MaxTokens:       1000,
			ReasoningEffort: "max",
		})
		if err != nil {
			t.Fatalf("translateRequest: %v", err)
		}
		if params.OutputConfig.Effort != sdk.OutputConfigEffortMax {
			t.Errorf("OutputConfig.Effort = %q, want max", params.OutputConfig.Effort)
		}
		out := string(mustMarshal(t, params))
		if !strings.Contains(out, `"effort":"max"`) {
			t.Errorf("expected effort max on the wire; body=%s", out)
		}
	})
	t.Run("budget_haiku_4_5", func(t *testing.T) {
		params, err := translateRequest(provider.Request{
			Model:           "claude-haiku-4-5",
			MaxTokens:       1000,
			ReasoningEffort: "max",
		})
		if err != nil {
			t.Fatalf("translateRequest: %v", err)
		}
		if params.MaxTokens != 32768+4096 {
			t.Errorf("MaxTokens = %d, want %d (budget 32768 + 4096)", params.MaxTokens, 32768+4096)
		}
		out := string(mustMarshal(t, params))
		if !strings.Contains(out, `"budget_tokens":32768`) {
			t.Errorf("expected budget_tokens 32768 on the wire; body=%s", out)
		}
	})
}
