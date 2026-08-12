package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	"be/internal/spawner/apirun/provider"
)

// A non-zero CacheControlEphemeralParam (TTL set) is required: a zero-valued
// struct is elided by the SDK's omitzero tag and cache_control never reaches the
// wire. translate.go sets TTL=5m, so the serialized marker is
// `cache_control:{"ttl":"5m","type":"ephemeral"}`. Tests match the
// `"cache_control"` key rather than the full value so they don't pin the TTL.
func TestTranslateRequest_CacheBreakpoint_System(t *testing.T) {
	req := provider.Request{
		Model:     "claude-opus-4-7",
		MaxTokens: 100,
		System:    "you are helpful",
		CacheBreakpoints: []provider.CacheBreakpoint{
			{Target: provider.CacheTargetSystem},
		},
	}
	params, err := translateRequest(req)
	if err != nil {
		t.Fatalf("translateRequest: %v", err)
	}
	if len(params.System) != 1 {
		t.Fatalf("System len = %d, want 1", len(params.System))
	}
	body, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	out := string(body)
	if !strings.Contains(out, `"cache_control":{"ttl":"5m","type":"ephemeral"}`) {
		t.Errorf("system cache_control not found in payload: %s", out)
	}
}

func TestTranslateRequest_CacheBreakpoint_ToolsLastOnly(t *testing.T) {
	req := provider.Request{
		Model:     "claude-opus-4-7",
		MaxTokens: 100,
		Tools: []provider.ToolSpec{
			{Name: "first", Description: "first tool", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)},
			{Name: "second", Description: "second tool", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)},
			{Name: "third", Description: "third tool", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)},
		},
		CacheBreakpoints: []provider.CacheBreakpoint{
			{Target: provider.CacheTargetTools},
		},
	}
	body := marshaledParams(t, req)
	out := string(body)
	first := strings.Index(out, `"name":"first"`)
	second := strings.Index(out, `"name":"second"`)
	third := strings.Index(out, `"name":"third"`)
	cc := strings.Index(out, `"cache_control"`)
	if first < 0 || second < 0 || third < 0 || cc < 0 {
		t.Fatalf("missing markers in payload: first=%d second=%d third=%d cc=%d body=%s", first, second, third, cc, out)
	}
	if cc < third {
		t.Errorf("cache_control appeared before last tool: cc=%d third=%d", cc, third)
	}
	if strings.Count(out, `"cache_control"`) != 1 {
		t.Errorf("expected exactly one cache_control entry, got: %s", out)
	}
}

func TestTranslateRequest_CacheBreakpoint_BothTargets(t *testing.T) {
	req := provider.Request{
		Model:     "claude-opus-4-7",
		MaxTokens: 100,
		System:    "be brief",
		Tools: []provider.ToolSpec{
			{Name: "only", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)},
		},
		CacheBreakpoints: []provider.CacheBreakpoint{
			{Target: provider.CacheTargetSystem},
			{Target: provider.CacheTargetTools},
		},
	}
	body := marshaledParams(t, req)
	out := string(body)
	if got := strings.Count(out, `"cache_control"`); got != 2 {
		t.Errorf("cache_control count = %d, want 2; body=%s", got, out)
	}
}

func TestTranslateRequest_CacheBreakpoint_None(t *testing.T) {
	req := provider.Request{
		Model:     "claude-opus-4-7",
		MaxTokens: 100,
		System:    "no cache",
		Tools: []provider.ToolSpec{
			{Name: "only", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)},
		},
	}
	body := marshaledParams(t, req)
	if strings.Contains(string(body), "cache_control") {
		t.Errorf("expected no cache_control without breakpoints; body=%s", body)
	}
}

// TestTranslateRequest_CacheBreakpoint_Message marks the last block of the most
// recent message — the sliding write point the next turn reads from.
func TestTranslateRequest_CacheBreakpoint_Message(t *testing.T) {
	req := provider.Request{
		Model:     "claude-opus-4-7",
		MaxTokens: 100,
		System:    "be brief",
		Messages: []provider.Message{
			{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "hello"}}},
			{Role: "assistant", Content: []provider.ContentBlock{{Type: "text", Text: "hi"}}},
			{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "again"}}},
		},
		CacheBreakpoints: []provider.CacheBreakpoint{
			{Target: provider.CacheTargetMessage},
		},
	}
	out := string(marshaledParams(t, req))
	if got := strings.Count(out, `"cache_control"`); got != 1 {
		t.Fatalf("cache_control count = %d, want 1 (short tail → single marker); body=%s", got, out)
	}
	// The marker must sit on the final message, after its "again" text.
	if cc, again := strings.Index(out, `"cache_control"`), strings.Index(out, "again"); cc < again {
		t.Errorf("marker not on the last message: cc=%d again=%d body=%s", cc, again, out)
	}
}

// TestTranslateRequest_CacheBreakpoint_Message_Media verifies the marker lands
// on the trailing media block, not on the label text before it — otherwise the
// media payload (the largest part of the turn) falls outside the cached prefix.
func TestTranslateRequest_CacheBreakpoint_Message_Media(t *testing.T) {
	req := provider.Request{
		Model:     "claude-opus-4-7",
		MaxTokens: 100,
		Messages: []provider.Message{{
			Role: "user",
			Content: []provider.ContentBlock{{
				Type: "tool_result", ToolUseID: "tool_doc", Output: "loaded",
				OutputMedia: []provider.MediaBlock{
					{Kind: "document", MediaType: "application/pdf", DataB64: "JVBERi0="},
				},
			}},
		}},
		CacheBreakpoints: []provider.CacheBreakpoint{{Target: provider.CacheTargetMessage}},
	}
	params, err := translateRequest(req)
	if err != nil {
		t.Fatalf("translateRequest: %v", err)
	}
	content := params.Messages[0].Content
	last := content[len(content)-1]
	if last.OfDocument == nil {
		t.Fatalf("last block = %+v, want the document block", last)
	}
	if last.OfDocument.CacheControl.TTL == "" {
		t.Errorf("document block carries no cache_control; media falls outside the cached prefix")
	}
}

// TestTranslateRequest_CacheBreakpoint_Message_LongTail verifies that when the
// conversation tail exceeds the lookback spacing, intermediate markers are
// placed at earlier message boundaries (so the previous turn's marker stays
// reachable), bounded by the slot budget.
func TestTranslateRequest_CacheBreakpoint_Message_LongTail(t *testing.T) {
	var msgs []provider.Message
	for i := 0; i < 12; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		msgs = append(msgs, provider.Message{
			Role: role,
			Content: []provider.ContentBlock{
				{Type: "text", Text: "a"},
				{Type: "text", Text: "b"},
				{Type: "text", Text: "c"},
			},
		})
	}
	req := provider.Request{
		Model:            "claude-opus-4-7",
		MaxTokens:        100,
		Messages:         msgs,
		CacheBreakpoints: []provider.CacheBreakpoint{{Target: provider.CacheTargetMessage}},
	}
	out := string(marshaledParams(t, req))
	got := strings.Count(out, `"cache_control"`)
	if got < 2 {
		t.Errorf("expected intermediate markers on a long tail, got %d markers; body=%s", got, out)
	}
	if got > maxCacheBreakpoints {
		t.Errorf("marker count %d exceeds slot budget %d", got, maxCacheBreakpoints)
	}
}

// TestTranslateRequest_CacheBreakpoint_SystemAndMessage confirms the production
// wiring (backend.go uses system+message): the system slot is spent first, then
// message markers fill the remainder — never exceeding the 4-slot cap.
func TestTranslateRequest_CacheBreakpoint_SystemAndMessage(t *testing.T) {
	req := provider.Request{
		Model:     "claude-opus-4-7",
		MaxTokens: 100,
		System:    "be brief",
		Messages: []provider.Message{
			{Role: "user", Content: []provider.ContentBlock{{Type: "text", Text: "q"}}},
		},
		CacheBreakpoints: []provider.CacheBreakpoint{
			{Target: provider.CacheTargetSystem},
			{Target: provider.CacheTargetMessage},
		},
	}
	out := string(marshaledParams(t, req))
	if got := strings.Count(out, `"cache_control"`); got != 2 {
		t.Errorf("cache_control count = %d, want 2 (system + one message marker); body=%s", got, out)
	}
}
