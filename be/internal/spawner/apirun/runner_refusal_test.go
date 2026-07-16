package apirun

import (
	"context"
	"strings"
	"testing"
	"time"

	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

func TestRunner_RefusalIsTerminalFailure(t *testing.T) {
	sink := &recordingSink{}
	proc := newTestProc()
	runner := NewRunner(Config{
		Provider:      mock.New(mock.Script{Final: provider.FinalResponse{StopReason: "refusal"}}),
		Sink:          sink,
		InitialPrompt: "hi",
		MaxIterations: 1,
		MaxContext:    1000,
		Deadline:      time.Now().Add(5 * time.Second),
	})

	runner.Run(context.Background(), proc)
	if proc.FinalStatus() != "FAIL" {
		t.Fatalf("FinalStatus = %q, want FAIL", proc.FinalStatus())
	}
	for _, call := range sink.Calls() {
		if call.category == "system" && strings.Contains(call.content, "provider refusal") {
			return
		}
	}
	t.Fatal("provider refusal message was not emitted")
}
