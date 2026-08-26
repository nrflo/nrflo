package openai

import (
	"context"
	"strings"
	"testing"
)

// TestRun_CachedTokensSplitOutOfInput verifies input_tokens_details.cached_tokens
// lands in Usage.CacheReadTokens and is subtracted from Usage.InputTokens: the
// Responses API counts cached tokens inside input_tokens, while consumers sum
// the Usage input fields (context %) and price them separately (session cost).
func TestRun_CachedTokensSplitOutOfInput(t *testing.T) {
	completed := `{"type":"response.completed","response":{"id":"resp_1","created_at":1,"status":"completed","model":"gpt-4o",` +
		`"usage":{"input_tokens":1000,"output_tokens":20,"total_tokens":1020,` +
		`"input_tokens_details":{"cached_tokens":800},"output_tokens_details":{"reasoning_tokens":0}},` +
		`"incomplete_details":{},"output":[],"error":{},"instructions":null,"metadata":{},` +
		`"parallel_tool_calls":false,"temperature":1,"tool_choice":{},"object":"response"}}`

	var b strings.Builder
	b.WriteString(sseEvent("response.completed", completed))

	resp, err := newTestProvider(b.String()).Run(context.Background(), minimalRequest(), &recordingSink{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.Usage.InputTokens != 200 {
		t.Errorf("InputTokens = %d, want 200 (1000 total - 800 cached)", resp.Usage.InputTokens)
	}
	if resp.Usage.CacheReadTokens != 800 {
		t.Errorf("CacheReadTokens = %d, want 800", resp.Usage.CacheReadTokens)
	}
	if got := resp.Usage.InputTokens + resp.Usage.CacheReadTokens + resp.Usage.CacheCreationTokens; got != 1000 {
		t.Errorf("summed input fields = %d, want 1000 (no double count)", got)
	}
}
