package openaichat

import (
	"encoding/json"
	"strings"
	"testing"

	"be/internal/spawner/apirun/provider"
)

// TestTranslateRequest_SystemUserAssistantToolResult verifies each block role
// folds into the right Chat Completions message shape: system -> SystemMessage,
// user text -> UserMessage, assistant text+tool_use -> one assistant message
// with ToolCalls, tool_result -> ToolMessage.
func TestTranslateRequest_SystemUserAssistantToolResult(t *testing.T) {
	req := provider.Request{
		Model:     "local-model",
		MaxTokens: 256,
		System:    "You are a helpful assistant.",
		Messages: []provider.Message{
			{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "list files"}}},
			{Role: "assistant", Content: []provider.ContentBlock{
				{Type: "text", Text: "sure, "},
				{Type: "tool_use", ToolUseID: "call_1", ToolName: "ls", Input: json.RawMessage(`{"path":"."}`)},
			}},
			{Role: "user", Content: []provider.ContentBlock{
				{Type: "tool_result", ToolUseID: "call_1", Output: "a.txt\nb.txt"},
			}},
		},
	}

	params, err := translateRequest(req)
	if err != nil {
		t.Fatalf("translateRequest: %v", err)
	}
	if len(params.Messages) != 4 {
		t.Fatalf("Messages len = %d, want 4 (system, user, assistant, tool)", len(params.Messages))
	}
	if params.Messages[0].OfSystem == nil {
		t.Error("Messages[0] is not a system message")
	}
	if params.Messages[1].OfUser == nil {
		t.Error("Messages[1] is not a user message")
	}
	assistant := params.Messages[2].OfAssistant
	if assistant == nil {
		t.Fatal("Messages[2] is not an assistant message")
	}
	if !assistant.Content.OfString.Valid() || assistant.Content.OfString.Value != "sure, " {
		t.Errorf("assistant Content = %+v, want 'sure, '", assistant.Content)
	}
	if len(assistant.ToolCalls) != 1 || assistant.ToolCalls[0].OfFunction == nil {
		t.Fatalf("assistant ToolCalls = %+v, want one function tool call", assistant.ToolCalls)
	}
	fn := assistant.ToolCalls[0].OfFunction
	if fn.ID != "call_1" || fn.Function.Name != "ls" || fn.Function.Arguments != `{"path":"."}` {
		t.Errorf("tool call = %+v, want call_1/ls/{\"path\":\".\"}", fn)
	}
	if params.Messages[3].OfTool == nil {
		t.Fatal("Messages[3] is not a tool message")
	}
	if params.MaxCompletionTokens.Value != 256 {
		t.Errorf("MaxCompletionTokens = %v, want 256", params.MaxCompletionTokens.Value)
	}
	if !params.StreamOptions.IncludeUsage.Value {
		t.Error("StreamOptions.IncludeUsage = false, want true")
	}
}

// TestTranslateRequest_ToolResultError verifies an is_error tool_result
// prefixes the tool message text with "Error: ".
func TestTranslateRequest_ToolResultError(t *testing.T) {
	req := provider.Request{
		Model:     "local-model",
		MaxTokens: 10,
		Messages: []provider.Message{
			{Role: "user", Content: []provider.ContentBlock{
				{Type: "tool_result", ToolUseID: "call_e", Output: "boom", IsError: true},
			}},
		},
	}
	params, err := translateRequest(req)
	if err != nil {
		t.Fatalf("translateRequest: %v", err)
	}
	if len(params.Messages) != 1 || params.Messages[0].OfTool == nil {
		t.Fatalf("Messages = %+v, want one tool message", params.Messages)
	}
	toolMsg := params.Messages[0].OfTool
	if !toolMsg.Content.OfString.Valid() || !strings.HasPrefix(toolMsg.Content.OfString.Value, "Error: ") {
		t.Errorf("tool message content = %+v, want prefixed with 'Error: '", toolMsg.Content)
	}
}

// TestTranslateRequest_EmptyAssistantMessage_Dropped verifies an assistant
// message with only thinking blocks (no text, no tool_use) produces no
// message item rather than an empty one.
func TestTranslateRequest_EmptyAssistantMessage_Dropped(t *testing.T) {
	req := provider.Request{
		Model:     "local-model",
		MaxTokens: 10,
		Messages: []provider.Message{
			{Role: "assistant", Content: []provider.ContentBlock{{Type: "thinking", Text: "hmm"}}},
			{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "hi"}}},
		},
	}
	params, err := translateRequest(req)
	if err != nil {
		t.Fatalf("translateRequest: %v", err)
	}
	if len(params.Messages) != 1 {
		t.Fatalf("Messages len = %d, want 1 (thinking-only assistant message dropped)", len(params.Messages))
	}
}

// TestTranslateRequest_UnsupportedAssistantBlockType_Errors verifies an
// unrecognized assistant content block type surfaces as an error rather than
// being silently dropped.
func TestTranslateRequest_UnsupportedAssistantBlockType_Errors(t *testing.T) {
	req := provider.Request{
		Model:     "local-model",
		MaxTokens: 10,
		Messages: []provider.Message{
			{Role: "assistant", Content: []provider.ContentBlock{{Type: "image"}}},
		},
	}
	_, err := translateRequest(req)
	if err == nil {
		t.Fatal("translateRequest(unsupported assistant block) succeeded, want error")
	}
}

// TestTranslateRequest_UnsupportedUserBlockType_Errors mirrors the assistant
// case for a user-role block.
func TestTranslateRequest_UnsupportedUserBlockType_Errors(t *testing.T) {
	req := provider.Request{
		Model:     "local-model",
		MaxTokens: 10,
		Messages: []provider.Message{
			{Role: "user", Content: []provider.ContentBlock{{Type: "image"}}},
		},
	}
	_, err := translateRequest(req)
	if err == nil {
		t.Fatal("translateRequest(unsupported user block) succeeded, want error")
	}
}

// TestTranslateRequest_Tools verifies tool definitions translate into
// ChatCompletionFunctionTool entries with auto tool_choice.
func TestTranslateRequest_Tools(t *testing.T) {
	req := provider.Request{
		Model:     "local-model",
		MaxTokens: 10,
		Tools: []provider.ToolSpec{
			{Name: "ls", Description: "list files", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
		Messages: []provider.Message{{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "hi"}}}},
	}
	params, err := translateRequest(req)
	if err != nil {
		t.Fatalf("translateRequest: %v", err)
	}
	if len(params.Tools) != 1 {
		t.Fatalf("Tools len = %d, want 1", len(params.Tools))
	}
	if !params.ToolChoice.OfAuto.Valid() || params.ToolChoice.OfAuto.Value != "auto" {
		t.Errorf("ToolChoice = %+v, want auto", params.ToolChoice)
	}
}

// TestTranslateRequest_ToolInvalidSchema_Errors verifies an invalid tool
// input_schema JSON surfaces as an error naming the tool.
func TestTranslateRequest_ToolInvalidSchema_Errors(t *testing.T) {
	req := provider.Request{
		Model:     "local-model",
		MaxTokens: 10,
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

// TestTranslateRequest_ReasoningEffortPassthrough verifies a non-empty
// ReasoningEffort is passed through to the Chat Completions params.
func TestTranslateRequest_ReasoningEffortPassthrough(t *testing.T) {
	req := provider.Request{
		Model:           "local-model",
		MaxTokens:       10,
		ReasoningEffort: "medium",
		Messages:        []provider.Message{{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "hi"}}}},
	}
	params, err := translateRequest(req)
	if err != nil {
		t.Fatalf("translateRequest: %v", err)
	}
	if string(params.ReasoningEffort) != "medium" {
		t.Errorf("ReasoningEffort = %q, want medium", params.ReasoningEffort)
	}
}
