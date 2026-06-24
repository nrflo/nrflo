package tools_builtin

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"be/internal/spawner/apirun"
)

// fnRunner adapts a func to apirun.DeepResearchRunner.
type fnRunner func(ctx context.Context, projectID, question string) (json.RawMessage, error)

func (f fnRunner) RunDeepResearch(ctx context.Context, projectID, question string) (json.RawMessage, error) {
	return f(ctx, projectID, question)
}

func invokeDR(env apirun.ToolEnv, question string) (string, bool) {
	out, isErr, _ := webDeepResearchHandler{}.Invoke(context.Background(), env, json.RawMessage(`{"question":"`+question+`"}`))
	return out, isErr
}

func TestWebDeepResearch_RecursionGuard(t *testing.T) {
	var calls int32
	r := fnRunner(func(context.Context, string, string) (json.RawMessage, error) {
		atomic.AddInt32(&calls, 1)
		return json.RawMessage(`{"summary":"x"}`), nil
	})
	out, isErr := invokeDR(apirun.ToolEnv{WorkflowName: "deep-research", DeepResearch: r}, "q")
	if !isErr {
		t.Errorf("expected isError for recursion, got %q", out)
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Error("runner must not be invoked under recursion guard")
	}
}

func TestWebDeepResearch_NilRunner(t *testing.T) {
	if _, isErr := invokeDR(apirun.ToolEnv{WorkflowName: "feature"}, "q"); !isErr {
		t.Error("expected isError when DeepResearch is nil")
	}
}

func TestWebDeepResearch_EmptyQuestion(t *testing.T) {
	r := fnRunner(func(context.Context, string, string) (json.RawMessage, error) { return nil, nil })
	if _, isErr := invokeDR(apirun.ToolEnv{WorkflowName: "feature", DeepResearch: r}, ""); !isErr {
		t.Error("expected isError for empty question")
	}
}

func TestWebDeepResearch_ErrorPropagation(t *testing.T) {
	r := fnRunner(func(context.Context, string, string) (json.RawMessage, error) {
		return nil, context.DeadlineExceeded
	})
	out, isErr := invokeDR(apirun.ToolEnv{WorkflowName: "feature", DeepResearch: r}, "q")
	if !isErr || !strings.Contains(out, "deep research failed") {
		t.Errorf("expected propagated failure, got isErr=%v out=%q", isErr, out)
	}
}

func TestWebDeepResearch_Success(t *testing.T) {
	report := `{"summary":"It works","findings":[{"claim":"C1","confidence":"high"}],"caveats":"be careful"}`
	r := fnRunner(func(_ context.Context, _, q string) (json.RawMessage, error) {
		if q != "q" {
			t.Errorf("question = %q, want q", q)
		}
		return json.RawMessage(report), nil
	})
	out, isErr := invokeDR(apirun.ToolEnv{WorkflowName: "feature", DeepResearch: r}, "q") // ArtifactSvc nil
	if isErr {
		t.Fatalf("unexpected error: %q", out)
	}
	for _, want := range []string{"## Summary", "It works", "C1", "be careful"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q: %q", want, out)
		}
	}
}

// TestWebDeepResearch_Heartbeat verifies the handler bumps the caller's stall
// timer while the run blocks. No sleeps: the runner blocks until the first
// heartbeat fires (interval shrunk to ~ms).
func TestWebDeepResearch_Heartbeat(t *testing.T) {
	old := deepResearchHeartbeatInterval
	deepResearchHeartbeatInterval = time.Millisecond
	defer func() { deepResearchHeartbeatInterval = old }()

	var beats int32
	beat := make(chan struct{}, 1)
	env := apirun.ToolEnv{
		WorkflowName: "feature",
		Heartbeat: func() {
			atomic.AddInt32(&beats, 1)
			select {
			case beat <- struct{}{}:
			default:
			}
		},
		DeepResearch: fnRunner(func(ctx context.Context, _, _ string) (json.RawMessage, error) {
			select {
			case <-beat: // unblock once at least one heartbeat fired
				return json.RawMessage(`{"summary":"ok"}`), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}),
	}
	if _, isErr := invokeDR(env, "q"); isErr {
		t.Fatal("unexpected error")
	}
	if atomic.LoadInt32(&beats) == 0 {
		t.Error("expected at least one heartbeat during the blocking run")
	}
}

func TestSummarizeReport(t *testing.T) {
	// Unparseable -> bounded raw, with artifact note.
	out := summarizeReport(json.RawMessage(`not json`), "art.json")
	if !strings.Contains(out, "art.json") {
		t.Errorf("expected artifact note, got %q", out)
	}
	// Cap enforced.
	huge := `{"summary":"` + strings.Repeat("x", 10000) + `"}`
	capped := summarizeReport(json.RawMessage(huge), "")
	if len(capped) > deepResearchSummaryCap+64 {
		t.Errorf("summary not capped: %d bytes", len(capped))
	}
}
