package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	"be/internal/spawner/apirun/provider"
)

// marshaledParams returns the JSON-serialized form of the SDK params. Tests
// use this to assert cache_control placement without poking at SDK internals.
func marshaledParams(t *testing.T, req provider.Request) []byte {
	t.Helper()
	params, err := translateRequest(req)
	if err != nil {
		t.Fatalf("translateRequest: %v", err)
	}
	b, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return b
}

// TestTranslateRequest_StripsContextSuffix verifies the "[1m]" context marker is
// stripped from the model id before the request (it 404s on the API).
func TestTranslateRequest_StripsContextSuffix(t *testing.T) {
	params, err := translateRequest(provider.Request{Model: "claude-opus-4-8[1m]", MaxTokens: 10})
	if err != nil {
		t.Fatalf("translateRequest: %v", err)
	}
	if params.Model != "claude-opus-4-8" {
		t.Errorf("Model = %q, want %q (suffix stripped)", params.Model, "claude-opus-4-8")
	}
}

func TestTranslateRequest_ToolChoice(t *testing.T) {
	for _, choice := range []string{"", "auto"} {
		t.Run("ok_"+choice, func(t *testing.T) {
			params, err := translateRequest(provider.Request{
				Model:      "claude-opus-4-7",
				MaxTokens:  10,
				ToolChoice: choice,
			})
			if err != nil {
				t.Fatalf("translateRequest: %v", err)
			}
			if params.ToolChoice.OfAuto == nil {
				t.Errorf("ToolChoice.OfAuto = nil, want non-nil for %q", choice)
			}
		})
	}

	for _, choice := range []string{"any", "none", "tool"} {
		t.Run("err_"+choice, func(t *testing.T) {
			_, err := translateRequest(provider.Request{
				Model:      "claude-opus-4-7",
				MaxTokens:  10,
				ToolChoice: choice,
			})
			if err == nil {
				t.Errorf("expected error for tool_choice=%q", choice)
			}
		})
	}
}

func TestTranslateRequest_ContentBlocks_Text(t *testing.T) {
	req := provider.Request{
		Model:     "claude-opus-4-7",
		MaxTokens: 10,
		Messages: []provider.Message{{
			Role:    "user",
			Content: []provider.ContentBlock{{Type: "text", Text: "hi"}},
		}},
	}
	body := marshaledParams(t, req)
	out := string(body)
	if !strings.Contains(out, `"text":"hi"`) {
		t.Errorf("text content not in payload: %s", out)
	}
}

func TestTranslateRequest_ContentBlocks_ToolUseValid(t *testing.T) {
	req := provider.Request{
		Model:     "claude-opus-4-7",
		MaxTokens: 10,
		Messages: []provider.Message{{
			Role: "assistant",
			Content: []provider.ContentBlock{{
				Type:      "tool_use",
				ToolUseID: "tool_1",
				ToolName:  "Read",
				Input:     json.RawMessage(`{"file_path":"/x"}`),
			}},
		}},
	}
	body := marshaledParams(t, req)
	out := string(body)
	if !strings.Contains(out, `"id":"tool_1"`) {
		t.Errorf("tool id missing from payload: %s", out)
	}
	if !strings.Contains(out, `"name":"Read"`) {
		t.Errorf("tool name missing from payload: %s", out)
	}
	if !strings.Contains(out, `"file_path":"/x"`) {
		t.Errorf("tool input missing from payload: %s", out)
	}
}

func TestTranslateRequest_ContentBlocks_ToolUseEmptyInput(t *testing.T) {
	req := provider.Request{
		Model:     "claude-opus-4-7",
		MaxTokens: 10,
		Messages: []provider.Message{{
			Role: "assistant",
			Content: []provider.ContentBlock{{
				Type:      "tool_use",
				ToolUseID: "tool_1",
				ToolName:  "NoArg",
			}},
		}},
	}
	if _, err := translateRequest(req); err != nil {
		t.Fatalf("translateRequest: %v", err)
	}
}

func TestTranslateRequest_ContentBlocks_ToolUseInvalidInput(t *testing.T) {
	req := provider.Request{
		Model:     "claude-opus-4-7",
		MaxTokens: 10,
		Messages: []provider.Message{{
			Role: "assistant",
			Content: []provider.ContentBlock{{
				Type:      "tool_use",
				ToolUseID: "tool_1",
				ToolName:  "Bad",
				Input:     json.RawMessage(`{not valid}`),
			}},
		}},
	}
	_, err := translateRequest(req)
	if err == nil {
		t.Fatalf("expected error for invalid tool_use input JSON")
	}
	if !strings.Contains(err.Error(), "tool_use") {
		t.Errorf("err = %v, want it to mention tool_use", err)
	}
}

func TestTranslateRequest_ContentBlocks_ToolResult(t *testing.T) {
	req := provider.Request{
		Model:     "claude-opus-4-7",
		MaxTokens: 10,
		Messages: []provider.Message{{
			Role: "user",
			Content: []provider.ContentBlock{{
				Type:      "tool_result",
				ToolUseID: "tool_1",
				Output:    "ok",
				IsError:   true,
			}},
		}},
	}
	body := marshaledParams(t, req)
	out := string(body)
	if !strings.Contains(out, `"tool_use_id":"tool_1"`) {
		t.Errorf("tool_result id missing: %s", out)
	}
	if !strings.Contains(out, `"is_error":true`) {
		t.Errorf("tool_result is_error missing: %s", out)
	}
	if !strings.Contains(out, `"text":"ok"`) {
		t.Errorf("tool_result output missing: %s", out)
	}
}

func TestTranslateRequest_ContentBlocks_UnknownType(t *testing.T) {
	req := provider.Request{
		Model:     "claude-opus-4-7",
		MaxTokens: 10,
		Messages: []provider.Message{{
			Role:    "user",
			Content: []provider.ContentBlock{{Type: "wat"}},
		}},
	}
	_, err := translateRequest(req)
	if err == nil {
		t.Fatalf("expected error for unknown content block type")
	}
	if !strings.Contains(err.Error(), "unsupported content block type") {
		t.Errorf("err = %v, want it to mention unsupported content block type", err)
	}
}

func TestTranslateRequest_ToolSchemaInvalid(t *testing.T) {
	req := provider.Request{
		Model:     "claude-opus-4-7",
		MaxTokens: 10,
		Tools: []provider.ToolSpec{
			{Name: "broken", InputSchema: json.RawMessage(`{not json`)},
		},
	}
	if _, err := translateRequest(req); err == nil {
		t.Fatalf("expected error for invalid tool input schema")
	}
}

func TestTranslateRequest_BasicShape(t *testing.T) {
	req := provider.Request{
		Model:     "claude-opus-4-7",
		MaxTokens: 256,
	}
	params, err := translateRequest(req)
	if err != nil {
		t.Fatalf("translateRequest: %v", err)
	}
	if params.Model != "claude-opus-4-7" {
		t.Errorf("Model = %q, want %q", params.Model, "claude-opus-4-7")
	}
	if params.MaxTokens != 256 {
		t.Errorf("MaxTokens = %d, want 256", params.MaxTokens)
	}
}
