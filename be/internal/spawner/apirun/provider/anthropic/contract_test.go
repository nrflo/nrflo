//go:build anthropic_live

// This file is for human-run smoke verification against the live Anthropic
// API. CI does NOT pass -tags=anthropic_live, so this is never executed by
// `make test`. To run locally:
//
//	ANTHROPIC_API_KEY=sk-... go test -tags=anthropic_live ./be/internal/spawner/apirun/provider/anthropic/...
package anthropic

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"be/internal/spawner/apirun/provider"
)

type discardSink struct{}

func (discardSink) OnTextDelta(string)                   {}
func (discardSink) OnToolUseStart(string, string)        {}
func (discardSink) OnToolUseInputDelta(string, string)   {}
func (discardSink) OnToolUseStop(string, json.RawMessage) {}
func (discardSink) OnUsage(provider.Usage)               {}

func TestLiveAnthropic_SmokeRun(t *testing.T) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set; skipping live contract test")
	}
	p := New(os.Getenv("ANTHROPIC_API_KEY"))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	resp, err := p.Run(ctx, provider.Request{
		Model:     "claude-haiku-4-5-20251001",
		MaxTokens: 64,
		Messages: []provider.Message{{
			Role:    "user",
			Content: []provider.ContentBlock{{Type: "text", Text: "Reply with the word ok"}},
		}},
	}, discardSink{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.StopReason != "end_turn" {
		t.Errorf("StopReason = %q, want end_turn", resp.StopReason)
	}
	if resp.Usage.OutputTokens <= 0 {
		t.Errorf("Usage.OutputTokens = %d, want > 0", resp.Usage.OutputTokens)
	}
	hasText := false
	for _, b := range resp.Content {
		if b.Type == "text" && b.Text != "" {
			hasText = true
			break
		}
	}
	if !hasText {
		t.Errorf("Content has no non-empty text block: %+v", resp.Content)
	}
}
