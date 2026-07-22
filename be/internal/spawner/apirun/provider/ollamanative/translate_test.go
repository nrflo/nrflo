package ollamanative

import (
	"encoding/json"
	"strings"
	"testing"

	"be/internal/spawner/apirun/provider"
)

// TestTranslateRequest_ThinkToggleByEffort verifies the think switch: ""
// and "none" map to think:false, any other recognized level maps to
// think:true.
func TestTranslateRequest_ThinkToggleByEffort(t *testing.T) {
	tests := []struct {
		effort string
		want   bool
	}{
		{"", false},
		{"none", false},
		{"low", true},
		{"medium", true},
		{"high", true},
	}
	for _, tc := range tests {
		t.Run("effort="+tc.effort, func(t *testing.T) {
			req := provider.Request{
				Model:           "llama3",
				ReasoningEffort: tc.effort,
				Messages:        []provider.Message{{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "hi"}}}},
			}
			body, err := translateRequest(req)
			if err != nil {
				t.Fatalf("translateRequest: %v", err)
			}
			if body.Think != tc.want {
				t.Errorf("effort=%q: Think = %v, want %v", tc.effort, body.Think, tc.want)
			}
		})
	}
}

// TestTranslateRequest_NumPredictFromMaxTokens verifies MaxTokens maps to
// options.num_predict, and is omitted entirely when MaxTokens is zero.
func TestTranslateRequest_NumPredictFromMaxTokens(t *testing.T) {
	t.Run("positive MaxTokens sets num_predict", func(t *testing.T) {
		req := provider.Request{
			Model:     "llama3",
			MaxTokens: 256,
			Messages:  []provider.Message{{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "hi"}}}},
		}
		body, err := translateRequest(req)
		if err != nil {
			t.Fatalf("translateRequest: %v", err)
		}
		if body.Options == nil || body.Options.NumPredict != 256 {
			t.Errorf("Options = %+v, want NumPredict=256", body.Options)
		}
	})

	t.Run("zero MaxTokens omits options", func(t *testing.T) {
		req := provider.Request{
			Model:    "llama3",
			Messages: []provider.Message{{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "hi"}}}},
		}
		body, err := translateRequest(req)
		if err != nil {
			t.Fatalf("translateRequest: %v", err)
		}
		if body.Options != nil {
			t.Errorf("Options = %+v, want nil", body.Options)
		}
	})
}

// TestTranslateRequest_SystemUserAssistantToolResult verifies each block
// role folds into the right native chat message shape.
func TestTranslateRequest_SystemUserAssistantToolResult(t *testing.T) {
	req := provider.Request{
		Model:  "llama3",
		System: "You are a helpful assistant.",
		Messages: []provider.Message{
			{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "list files"}}},
			{Role: "assistant", Content: []provider.ContentBlock{
				{Type: "text", Text: "sure, "},
				{Type: "tool_use", ToolUseID: "call_0", ToolName: "ls", Input: json.RawMessage(`{"path":"."}`)},
			}},
			{Role: "user", Content: []provider.ContentBlock{
				{Type: "tool_result", ToolUseID: "call_0", Output: "a.txt\nb.txt"},
			}},
		},
	}
	body, err := translateRequest(req)
	if err != nil {
		t.Fatalf("translateRequest: %v", err)
	}
	if len(body.Messages) != 4 {
		t.Fatalf("Messages len = %d, want 4 (system, user, assistant, tool)", len(body.Messages))
	}
	if body.Messages[0].Role != "system" || body.Messages[0].Content != "You are a helpful assistant." {
		t.Errorf("Messages[0] = %+v, want system message", body.Messages[0])
	}
	if body.Messages[1].Role != "user" || body.Messages[1].Content != "list files" {
		t.Errorf("Messages[1] = %+v, want user message", body.Messages[1])
	}
	assistant := body.Messages[2]
	if assistant.Role != "assistant" || assistant.Content != "sure, " {
		t.Errorf("Messages[2] = %+v, want assistant text 'sure, '", assistant)
	}
	if len(assistant.ToolCalls) != 1 || assistant.ToolCalls[0].Function.Name != "ls" {
		t.Fatalf("assistant ToolCalls = %+v, want one ls call", assistant.ToolCalls)
	}
	if string(assistant.ToolCalls[0].Function.Arguments) != `{"path":"."}` {
		t.Errorf("tool call args = %q, want %q", assistant.ToolCalls[0].Function.Arguments, `{"path":"."}`)
	}
	if body.Messages[3].Role != "tool" || body.Messages[3].Content != "a.txt\nb.txt" {
		t.Errorf("Messages[3] = %+v, want tool message", body.Messages[3])
	}
}

// TestTranslateRequest_ToolResultError verifies an is_error tool_result
// prefixes the tool message content with "Error: ".
func TestTranslateRequest_ToolResultError(t *testing.T) {
	req := provider.Request{
		Model: "llama3",
		Messages: []provider.Message{
			{Role: "user", Content: []provider.ContentBlock{
				{Type: "tool_result", ToolUseID: "call_e", Output: "boom", IsError: true},
			}},
		},
	}
	body, err := translateRequest(req)
	if err != nil {
		t.Fatalf("translateRequest: %v", err)
	}
	if len(body.Messages) != 1 || !strings.HasPrefix(body.Messages[0].Content, "Error: ") {
		t.Errorf("Messages = %+v, want one tool message prefixed with 'Error: '", body.Messages)
	}
}

// TestTranslateRequest_Tools verifies tool definitions translate into
// function-typed chatTool entries with the schema decoded.
func TestTranslateRequest_Tools(t *testing.T) {
	req := provider.Request{
		Model: "llama3",
		Tools: []provider.ToolSpec{
			{Name: "ls", Description: "list files", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
		Messages: []provider.Message{{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "hi"}}}},
	}
	body, err := translateRequest(req)
	if err != nil {
		t.Fatalf("translateRequest: %v", err)
	}
	if len(body.Tools) != 1 {
		t.Fatalf("Tools len = %d, want 1", len(body.Tools))
	}
	tool := body.Tools[0]
	if tool.Type != "function" || tool.Function.Name != "ls" || tool.Function.Description != "list files" {
		t.Errorf("Tools[0] = %+v, want function/ls/list files", tool)
	}
	if tool.Function.Parameters["type"] != "object" {
		t.Errorf("Parameters = %+v, want type=object", tool.Function.Parameters)
	}
}

// TestTranslateRequest_ToolInvalidSchema_Errors verifies an invalid tool
// input_schema JSON surfaces as an error naming the tool.
func TestTranslateRequest_ToolInvalidSchema_Errors(t *testing.T) {
	req := provider.Request{
		Model: "llama3",
		Tools: []provider.ToolSpec{
			{Name: "bad-tool", InputSchema: json.RawMessage(`not json`)},
		},
		Messages: []provider.Message{{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "hi"}}}},
	}
	_, err := translateRequest(req)
	if err == nil {
		t.Fatal("translateRequest(invalid tool schema) succeeded, want error")
	}
	if !strings.Contains(err.Error(), "bad-tool") {
		t.Errorf("err = %v, want mention of tool name bad-tool", err)
	}
}

// TestTranslateRequest_EmptyAssistantMessage_Dropped verifies an assistant
// message with only thinking blocks (no text, no tool_use) produces no
// message item rather than an empty one.
func TestTranslateRequest_EmptyAssistantMessage_Dropped(t *testing.T) {
	req := provider.Request{
		Model: "llama3",
		Messages: []provider.Message{
			{Role: "assistant", Content: []provider.ContentBlock{{Type: "thinking", Text: "hmm"}}},
			{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "hi"}}},
		},
	}
	body, err := translateRequest(req)
	if err != nil {
		t.Fatalf("translateRequest: %v", err)
	}
	if len(body.Messages) != 1 {
		t.Fatalf("Messages len = %d, want 1 (thinking-only assistant message dropped)", len(body.Messages))
	}
}

// TestTranslateRequest_UnsupportedAssistantBlockType_Errors verifies an
// unrecognized assistant content block type surfaces as an error.
func TestTranslateRequest_UnsupportedAssistantBlockType_Errors(t *testing.T) {
	req := provider.Request{
		Model:    "llama3",
		Messages: []provider.Message{{Role: "assistant", Content: []provider.ContentBlock{{Type: "image"}}}},
	}
	if _, err := translateRequest(req); err == nil {
		t.Fatal("translateRequest(unsupported assistant block) succeeded, want error")
	}
}

// TestTranslateRequest_UnsupportedUserBlockType_Errors mirrors the assistant
// case for a user-role block.
func TestTranslateRequest_UnsupportedUserBlockType_Errors(t *testing.T) {
	req := provider.Request{
		Model:    "llama3",
		Messages: []provider.Message{{Role: "user", Content: []provider.ContentBlock{{Type: "image"}}}},
	}
	if _, err := translateRequest(req); err == nil {
		t.Fatal("translateRequest(unsupported user block) succeeded, want error")
	}
}
