package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"be/internal/spawner/apirun/provider"
)

// marshaledParams serialises translateRequest output to JSON for assertion.
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

func TestTranslateRequest_BasicShape(t *testing.T) {
	req := provider.Request{Model: "gpt-4o", MaxTokens: 256}
	params, err := translateRequest(req)
	if err != nil {
		t.Fatalf("translateRequest: %v", err)
	}
	if string(params.Model) != "gpt-4o" {
		t.Errorf("Model = %q, want %q", params.Model, "gpt-4o")
	}
	if params.MaxOutputTokens.Value != 256 {
		t.Errorf("MaxOutputTokens = %d, want 256", params.MaxOutputTokens.Value)
	}
}

func TestTranslateRequest_StoreIsFalse(t *testing.T) {
	body := marshaledParams(t, provider.Request{Model: "gpt-4o", MaxTokens: 10})
	if !strings.Contains(string(body), `"store":false`) {
		t.Errorf("expected store:false in payload; body=%s", body)
	}
}

func TestTranslateRequest_SystemBecomesInstructions(t *testing.T) {
	body := marshaledParams(t, provider.Request{
		Model:     "gpt-4o",
		MaxTokens: 10,
		System:    "be helpful",
	})
	if !strings.Contains(string(body), `"instructions":"be helpful"`) {
		t.Errorf("instructions field missing or wrong; body=%s", body)
	}
}

func TestTranslateRequest_NoSystem_NoInstructions(t *testing.T) {
	body := marshaledParams(t, provider.Request{Model: "gpt-4o", MaxTokens: 10})
	if strings.Contains(string(body), `"instructions"`) {
		t.Errorf("unexpected instructions field when System is empty; body=%s", body)
	}
}

func TestTranslateRequest_ReasoningEffort(t *testing.T) {
	body := marshaledParams(t, provider.Request{
		Model:           "o3",
		MaxTokens:       100,
		ReasoningEffort: "medium",
	})
	if !strings.Contains(string(body), `"effort":"medium"`) {
		t.Errorf("reasoning effort missing; body=%s", body)
	}
}

func TestTranslateRequest_ToolSpec(t *testing.T) {
	req := provider.Request{
		Model:     "gpt-4o",
		MaxTokens: 10,
		Tools: []provider.ToolSpec{
			{
				Name:        "Read",
				Description: "reads a file",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
			},
		},
	}
	body := marshaledParams(t, req)
	out := string(body)
	if !strings.Contains(out, `"name":"Read"`) {
		t.Errorf("tool name missing; body=%s", out)
	}
	if !strings.Contains(out, `"path"`) {
		t.Errorf("tool parameters missing; body=%s", out)
	}
}

func TestTranslateRequest_ToolSchemaInvalid(t *testing.T) {
	req := provider.Request{
		Model:     "gpt-4o",
		MaxTokens: 10,
		Tools:     []provider.ToolSpec{{Name: "broken", InputSchema: json.RawMessage(`{not json`)}},
	}
	if _, err := translateRequest(req); err == nil {
		t.Fatalf("expected error for invalid tool schema")
	}
}

func TestTranslateRequest_Messages_TextBlock(t *testing.T) {
	req := provider.Request{
		Model:     "gpt-4o",
		MaxTokens: 10,
		Messages: []provider.Message{{
			Role:    "user",
			Content: []provider.ContentBlock{{Type: "text", Text: "hello world"}},
		}},
	}
	body := marshaledParams(t, req)
	if !strings.Contains(string(body), `"hello world"`) {
		t.Errorf("text content missing; body=%s", body)
	}
}

func TestTranslateRequest_Messages_ToolUse(t *testing.T) {
	req := provider.Request{
		Model:     "gpt-4o",
		MaxTokens: 10,
		Messages: []provider.Message{{
			Role: "assistant",
			Content: []provider.ContentBlock{{
				Type:      "tool_use",
				ToolUseID: "call_abc",
				ToolName:  "Read",
				Input:     json.RawMessage(`{"path":"/x"}`),
			}},
		}},
	}
	body := marshaledParams(t, req)
	out := string(body)
	if !strings.Contains(out, `"call_abc"`) {
		t.Errorf("tool call id missing; body=%s", out)
	}
	if !strings.Contains(out, `"Read"`) {
		t.Errorf("tool name missing; body=%s", out)
	}
	// arguments is a JSON-string-encoded field; check for the path value inside it.
	if !strings.Contains(out, `path`) || !strings.Contains(out, `/x`) {
		t.Errorf("tool input missing; body=%s", out)
	}
}

func TestTranslateRequest_Messages_ToolResult(t *testing.T) {
	req := provider.Request{
		Model:     "gpt-4o",
		MaxTokens: 10,
		Messages: []provider.Message{{
			Role: "user",
			Content: []provider.ContentBlock{{
				Type:      "tool_result",
				ToolUseID: "call_abc",
				Output:    "file data",
			}},
		}},
	}
	body := marshaledParams(t, req)
	out := string(body)
	if !strings.Contains(out, `"call_abc"`) {
		t.Errorf("tool_result call_id missing; body=%s", out)
	}
	if !strings.Contains(out, `"file data"`) {
		t.Errorf("tool_result output missing; body=%s", out)
	}
}

func TestTranslateRequest_Messages_ToolResultIsError(t *testing.T) {
	req := provider.Request{
		Model:     "gpt-4o",
		MaxTokens: 10,
		Messages: []provider.Message{{
			Role: "user",
			Content: []provider.ContentBlock{{
				Type:      "tool_result",
				ToolUseID: "call_err",
				Output:    "not found",
				IsError:   true,
			}},
		}},
	}
	body := marshaledParams(t, req)
	if !strings.Contains(string(body), "Error: not found") {
		t.Errorf("expected 'Error: not found' in payload; body=%s", body)
	}
}

func TestTranslateRequest_Messages_UnknownContentType(t *testing.T) {
	req := provider.Request{
		Model:     "gpt-4o",
		MaxTokens: 10,
		Messages: []provider.Message{{
			Role:    "user",
			Content: []provider.ContentBlock{{Type: "image"}},
		}},
	}
	_, err := translateRequest(req)
	if err == nil {
		t.Fatalf("expected error for unsupported content block type")
	}
	if !strings.Contains(err.Error(), "unsupported content block type") {
		t.Errorf("err = %v, want mention of unsupported content block type", err)
	}
}
