package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

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

// TestTranslateRequest_ThinkingEnabled_LowEffort verifies budget=4096 is set and
// MaxTokens is raised to budget+4096 when the request MaxTokens is below the floor.
func TestTranslateRequest_ThinkingEnabled_LowEffort(t *testing.T) {
	req := provider.Request{
		Model:           "claude-opus-4-7",
		MaxTokens:       100, // below budget+4096 = 8192
		ReasoningEffort: "low",
	}
	params, err := translateRequest(req)
	if err != nil {
		t.Fatalf("translateRequest: %v", err)
	}

	// Thinking must be present (non-zero budget)
	body, _ := json.Marshal(params)
	out := string(body)
	if !strings.Contains(out, `"thinking"`) {
		t.Errorf("expected 'thinking' in params; body=%s", out)
	}
	if !strings.Contains(out, `"budget_tokens"`) {
		t.Errorf("expected budget_tokens in thinking params; body=%s", out)
	}

	// MaxTokens raised to budget+4096 = 8192
	if params.MaxTokens != 8192 {
		t.Errorf("MaxTokens = %d, want 8192 (budget 4096 + 4096)", params.MaxTokens)
	}
}

// TestTranslateRequest_ThinkingEnabled_HighEffort verifies budget=16384 and
// MaxTokens raised to 20480 (16384+4096) when below that.
func TestTranslateRequest_ThinkingEnabled_HighEffort(t *testing.T) {
	req := provider.Request{
		Model:           "claude-opus-4-7",
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

// TestTranslateRequest_ThinkingEnabled_SufficientMaxTokens verifies MaxTokens
// is NOT raised when it already exceeds budget+4096.
func TestTranslateRequest_ThinkingEnabled_SufficientMaxTokens(t *testing.T) {
	req := provider.Request{
		Model:           "claude-opus-4-7",
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

// TestTranslateRequest_ThinkingDisabled_EmptyEffort verifies that empty effort
// leaves thinking params unset.
func TestTranslateRequest_ThinkingDisabled_EmptyEffort(t *testing.T) {
	req := provider.Request{
		Model:           "claude-opus-4-7",
		MaxTokens:       1000,
		ReasoningEffort: "",
	}
	params, err := translateRequest(req)
	if err != nil {
		t.Fatalf("translateRequest: %v", err)
	}
	body, _ := json.Marshal(params)
	out := string(body)
	// budget_tokens must be absent when thinking is disabled
	if strings.Contains(out, `"budget_tokens"`) {
		t.Errorf("budget_tokens present in payload with empty effort; body=%s", out)
	}
	// MaxTokens unchanged
	if params.MaxTokens != 1000 {
		t.Errorf("MaxTokens = %d, want 1000 (unchanged)", params.MaxTokens)
	}
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
