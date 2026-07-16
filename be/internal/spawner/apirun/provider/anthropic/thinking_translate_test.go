package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"

	"be/internal/spawner/apirun/provider"
)

// TestThinkingBudget_MappingTable verifies all documented effort → budget values.
func TestThinkingBudget_MappingTable(t *testing.T) {
	cases := []struct {
		effort string
		want   int
	}{
		{"", 0},
		{"low", 4096},
		{"medium", 8192},
		{"high", 16384},
		{"xhigh", 24576},
		{"max", 32768},
		{"unknown-non-empty", 8192}, // falls through to medium default
		{"extreme", 8192},           // unknown non-empty → medium
	}
	for _, tc := range cases {
		t.Run(tc.effort, func(t *testing.T) {
			got := thinkingBudget(tc.effort)
			if got != tc.want {
				t.Errorf("thinkingBudget(%q) = %d, want %d", tc.effort, got, tc.want)
			}
		})
	}
}

// TestThinkingBudget_TinyValueFloor verifies a small explicit numeric-string goes through
// the unknown branch → medium (8192). The floor-to-1024 behavior only applies when
// the SDK clamps internally; our switch just uses the default for unknown strings.
func TestThinkingBudget_UnknownNonEmpty(t *testing.T) {
	got := thinkingBudget("tiny")
	if got != 8192 {
		t.Errorf("thinkingBudget(tiny) = %d, want 8192 (unknown → medium)", got)
	}
}

// Budget (enabled) thinking path: Haiku 4.5 and older have no adaptive mode and
// reject the effort parameter, so they keep thinking:{type:"enabled",budget_tokens}.

// TestTranslateRequest_BudgetThinking_LowEffort verifies budget=4096 is set and
// MaxTokens is raised to budget+4096 when below the floor (Haiku path).
func TestTranslateRequest_BudgetThinking_LowEffort(t *testing.T) {
	req := provider.Request{
		Model:           "claude-haiku-4-5",
		MaxTokens:       100, // below budget+4096 = 8192
		ReasoningEffort: "low",
	}
	params, err := translateRequest(req)
	if err != nil {
		t.Fatalf("translateRequest: %v", err)
	}

	body, _ := json.Marshal(params)
	out := string(body)
	if !strings.Contains(out, `"budget_tokens"`) {
		t.Errorf("expected budget_tokens in thinking params; body=%s", out)
	}
	if strings.Contains(out, `"adaptive"`) {
		t.Errorf("Haiku must not use adaptive thinking; body=%s", out)
	}
	if params.MaxTokens != 8192 {
		t.Errorf("MaxTokens = %d, want 8192 (budget 4096 + 4096)", params.MaxTokens)
	}
}

// TestTranslateRequest_BudgetThinking_HighEffort verifies budget=16384 and
// MaxTokens raised to 20480 (16384+4096) when below that (Haiku path).
func TestTranslateRequest_BudgetThinking_HighEffort(t *testing.T) {
	req := provider.Request{
		Model:           "claude-haiku-4-5",
		MaxTokens:       1000,
		ReasoningEffort: "high",
	}
	params, err := translateRequest(req)
	if err != nil {
		t.Fatalf("translateRequest: %v", err)
	}
	if params.MaxTokens != 20480 {
		t.Errorf("MaxTokens = %d, want 20480 (16384+4096)", params.MaxTokens)
	}
}

// TestTranslateRequest_BudgetThinking_SufficientMaxTokens verifies MaxTokens is
// NOT raised when it already exceeds budget+4096 (Haiku path).
func TestTranslateRequest_BudgetThinking_SufficientMaxTokens(t *testing.T) {
	req := provider.Request{
		Model:           "claude-haiku-4-5",
		MaxTokens:       32000, // already > medium budget+4096 = 12288
		ReasoningEffort: "medium",
	}
	params, err := translateRequest(req)
	if err != nil {
		t.Fatalf("translateRequest: %v", err)
	}
	if params.MaxTokens != 32000 {
		t.Errorf("MaxTokens = %d, want 32000 (no raise needed)", params.MaxTokens)
	}
}

// TestTranslateRequest_AdaptiveThinkingModels verifies current adaptive models
// use the effort output-config, never the legacy budget shape.
func TestTranslateRequest_AdaptiveThinkingModels(t *testing.T) {
	for _, model := range []string{
		"claude-opus-4-6", "claude-opus-4-7", "claude-opus-4-8", "claude-sonnet-5",
		"claude-fable-5", "claude-mythos-5",
	} {
		t.Run(model, func(t *testing.T) {
			params, err := translateRequest(provider.Request{
				Model:           model,
				MaxTokens:       1000,
				ReasoningEffort: "high",
			})
			if err != nil {
				t.Fatalf("translateRequest: %v", err)
			}
			if params.Thinking.OfAdaptive == nil {
				t.Errorf("Thinking.OfAdaptive = nil, want adaptive for %s", model)
			}
			if params.OutputConfig.Effort != sdk.OutputConfigEffortHigh {
				t.Errorf("OutputConfig.Effort = %q, want high", params.OutputConfig.Effort)
			}
			if params.MaxTokens != 1000 {
				t.Errorf("MaxTokens = %d, want 1000 (adaptive must not raise it)", params.MaxTokens)
			}
			out := string(mustMarshal(t, params))
			if !strings.Contains(out, `"type":"adaptive"`) {
				t.Errorf("expected adaptive thinking on the wire; body=%s", out)
			}
			if !strings.Contains(out, `"effort":"high"`) {
				t.Errorf("expected effort on the wire; body=%s", out)
			}
			if strings.Contains(out, `"budget_tokens"`) {
				t.Errorf("budget_tokens must be absent for adaptive models; body=%s", out)
			}
		})
	}
}

// TestTranslateRequest_ThinkingDisabled_EmptyEffort verifies that empty effort
// leaves thinking and effort unset on any model.
func TestTranslateRequest_ThinkingDisabled_EmptyEffort(t *testing.T) {
	req := provider.Request{
		Model:           "claude-opus-4-8",
		MaxTokens:       1000,
		ReasoningEffort: "",
	}
	params, err := translateRequest(req)
	if err != nil {
		t.Fatalf("translateRequest: %v", err)
	}
	out := string(mustMarshal(t, params))
	if strings.Contains(out, `"budget_tokens"`) || strings.Contains(out, `"adaptive"`) || strings.Contains(out, `"effort"`) {
		t.Errorf("thinking/effort present with empty effort; body=%s", out)
	}
	if params.MaxTokens != 1000 {
		t.Errorf("MaxTokens = %d, want 1000 (unchanged)", params.MaxTokens)
	}
}

// mustMarshal JSON-encodes the SDK params for wire-shape assertions.
func mustMarshal(t *testing.T, params sdk.MessageNewParams) []byte {
	t.Helper()
	b, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return b
}

// TestTranslateContentBlocks_ThinkingPreservesSignature verifies that a
// "thinking" ContentBlock round-trips with signature into the SDK param.
func TestTranslateContentBlocks_ThinkingPreservesSignature(t *testing.T) {
	req := provider.Request{
		Model:     "claude-opus-4-7",
		MaxTokens: 10,
		Messages: []provider.Message{{
			Role: "assistant",
			Content: []provider.ContentBlock{
				{Type: "thinking", Text: "my thoughts", Signature: "sig-xyz"},
			},
		}},
	}
	body := marshaledParams(t, req)
	out := string(body)
	if !strings.Contains(out, `"thinking"`) {
		t.Errorf("thinking type missing in payload: %s", out)
	}
	if !strings.Contains(out, `"sig-xyz"`) {
		t.Errorf("signature missing in payload: %s", out)
	}
	if !strings.Contains(out, `"my thoughts"`) {
		t.Errorf("thinking text missing in payload: %s", out)
	}
}

// TestTranslateContentBlocks_RedactedThinkingPreservesData verifies that a
// "redacted_thinking" ContentBlock round-trips with data into the SDK param.
func TestTranslateContentBlocks_RedactedThinkingPreservesData(t *testing.T) {
	req := provider.Request{
		Model:     "claude-opus-4-7",
		MaxTokens: 10,
		Messages: []provider.Message{{
			Role: "assistant",
			Content: []provider.ContentBlock{
				{Type: "redacted_thinking", Data: "opaque-blob"},
			},
		}},
	}
	body := marshaledParams(t, req)
	out := string(body)
	if !strings.Contains(out, `"redacted_thinking"`) {
		t.Errorf("redacted_thinking type missing in payload: %s", out)
	}
	if !strings.Contains(out, `"opaque-blob"`) {
		t.Errorf("redacted_thinking data missing in payload: %s", out)
	}
}

// TestTranslateContentBlocks_ThinkingBeforeToolUse verifies that thinking blocks
// appear before tool_use blocks in translated output (API ordering requirement).
func TestTranslateContentBlocks_ThinkingBeforeToolUse(t *testing.T) {
	blocks := []provider.ContentBlock{
		{Type: "thinking", Text: "reasoning", Signature: "sig1"},
		{Type: "tool_use", ToolUseID: "t1", ToolName: "Bash", Input: json.RawMessage(`{}`)},
	}
	out, err := translateContentBlocks(blocks)
	if err != nil {
		t.Fatalf("translateContentBlocks: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
	if out[0].OfThinking == nil {
		t.Errorf("out[0] is not a thinking block: %+v", out[0])
	}
	if out[1].OfToolUse == nil {
		t.Errorf("out[1] is not a tool_use block: %+v", out[1])
	}
}

// TestTranslateContentBlocks_RedactedThinkingBeforeToolUse verifies same ordering
// for redacted_thinking.
func TestTranslateContentBlocks_RedactedThinkingBeforeToolUse(t *testing.T) {
	blocks := []provider.ContentBlock{
		{Type: "redacted_thinking", Data: "blob"},
		{Type: "tool_use", ToolUseID: "t2", ToolName: "Read", Input: json.RawMessage(`{}`)},
	}
	out, err := translateContentBlocks(blocks)
	if err != nil {
		t.Fatalf("translateContentBlocks: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
	if out[0].OfRedactedThinking == nil {
		t.Errorf("out[0] is not a redacted_thinking block: %+v", out[0])
	}
	if out[1].OfToolUse == nil {
		t.Errorf("out[1] is not a tool_use block: %+v", out[1])
	}
}
