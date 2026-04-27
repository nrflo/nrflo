// Package mock provides a scripted Provider implementation for unit tests.
// Tests construct one Script per planned turn; each Run() call replays the
// next script's events through the EventSink and returns its FinalResponse
// (or error).
package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"be/internal/spawner/apirun/provider"
)

// SinkEventKind tags which EventSink callback the SinkEvent triggers.
type SinkEventKind int

const (
	EventText SinkEventKind = iota
	EventToolUseStart
	EventToolUseInputDelta
	EventToolUseStop
	EventUsage
)

// SinkEvent describes one EventSink callback invocation.
type SinkEvent struct {
	Kind        SinkEventKind
	Text        string
	ToolUseID   string
	ToolName    string
	PartialJSON string
	FullInput   json.RawMessage
	Usage       provider.Usage
}

// Script defines a single scripted turn.
type Script struct {
	Events []SinkEvent
	Final  provider.FinalResponse
	Err    error
}

// New constructs a mock Provider that replays the given scripts in order.
// One script is consumed per Run() call. Calling Run after the scripts are
// exhausted returns an error.
func New(scripts ...Script) provider.Provider {
	return &mockProvider{scripts: scripts}
}

type mockProvider struct {
	mu      sync.Mutex
	scripts []Script
	cursor  int
}

func (m *mockProvider) Name() string { return "mock" }

func (m *mockProvider) MaxContext(model string) int { return 200000 }

func (m *mockProvider) Run(ctx context.Context, req provider.Request, sink provider.EventSink) (*provider.FinalResponse, error) {
	m.mu.Lock()
	if m.cursor >= len(m.scripts) {
		m.mu.Unlock()
		return nil, fmt.Errorf("mock provider: no script for turn %d", m.cursor+1)
	}
	script := m.scripts[m.cursor]
	m.cursor++
	m.mu.Unlock()

	for _, ev := range script.Events {
		switch ev.Kind {
		case EventText:
			sink.OnTextDelta(ev.Text)
		case EventToolUseStart:
			sink.OnToolUseStart(ev.ToolUseID, ev.ToolName)
		case EventToolUseInputDelta:
			sink.OnToolUseInputDelta(ev.ToolUseID, ev.PartialJSON)
		case EventToolUseStop:
			sink.OnToolUseStop(ev.ToolUseID, ev.FullInput)
		case EventUsage:
			sink.OnUsage(ev.Usage)
		}
	}

	if script.Err != nil {
		return nil, script.Err
	}
	final := script.Final
	return &final, nil
}
