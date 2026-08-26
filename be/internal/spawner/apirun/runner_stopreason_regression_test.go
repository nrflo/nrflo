package apirun

import (
	"context"
	"testing"
	"time"

	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

// TestRunner_EmptyStopReason_DerivedFromContent is the regression test for the
// OpenRouter "unexpected stop_reason=\"\"" failure: when a provider returns a
// FinalResponse with an empty StopReason (stream ended without its terminal
// event), the runner derives it from content instead of failing the session.
func TestRunner_EmptyStopReason_DerivedFromContent(t *testing.T) {
	t.Run("empty_reason_with_text_content_passes", func(t *testing.T) {
		sink := &recordingSink{}
		prov := mock.New(mock.Script{
			Final: provider.FinalResponse{
				StopReason: "", // stream ended without terminal event
				Content:    []provider.ContentBlock{{Type: "text", Text: "done"}},
			},
		})
		r := NewRunner(Config{
			Provider:      prov,
			Sink:          sink,
			InitialPrompt: "hi",
			MaxIterations: 3,
			MaxContext:    1000,
			Deadline:      time.Now().Add(5 * time.Second),
		})
		proc := newTestProc()
		r.Run(context.Background(), proc)
		if proc.FinalStatus() != "PASS" {
			t.Errorf("FinalStatus = %q, want PASS (derived end_turn)", proc.FinalStatus())
		}
	})

	t.Run("empty_reason_with_tool_use_dispatches_tools", func(t *testing.T) {
		sink := &recordingSink{}
		handler := &recordingHandler{name: "findings_add", output: "ok"}
		prov := mock.New(
			mock.Script{Final: provider.FinalResponse{
				StopReason: "",
				Content:    []provider.ContentBlock{toolUseBlock("tu_e", "findings_add", `{}`)},
			}},
			mock.Script{Final: provider.FinalResponse{StopReason: "end_turn"}},
		)
		r := NewRunner(Config{
			Provider:      prov,
			Sink:          sink,
			Handlers:      Registry{"findings_add": handler},
			InitialPrompt: "hi",
			MaxIterations: 3,
			MaxContext:    1000,
			Deadline:      time.Now().Add(5 * time.Second),
		})
		proc := newTestProc()
		r.Run(context.Background(), proc)
		if proc.FinalStatus() != "PASS" {
			t.Errorf("FinalStatus = %q, want PASS", proc.FinalStatus())
		}
		if len(handler.Calls()) != 1 {
			t.Errorf("handler calls = %d, want 1 (tool dispatched via derived tool_use)", len(handler.Calls()))
		}
	})
}
