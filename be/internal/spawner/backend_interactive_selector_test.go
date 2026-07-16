package spawner

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/ws"
)

// selectBackendForTest mirrors startBackend's three-way switch on executionMode,
// enabling clean unit tests of the selector without calling Start() or writing the session row.
// Returns nil for unknown modes (startBackend returns an error for those).
func selectBackendForTest(s *Spawner, executionMode string, adapter CLIAdapter) ExecutionBackend {
	switch executionMode {
	case "api":
		return newAPIBackend(s)
	case "script":
		return newScriptBackend(s)
	case "cli_interactive":
		return newCLIInteractiveBackend(adapter, s, nil)
	default:
		return nil // startBackend returns an error; callers must not expect a backend
	}
}

// TestStartBackend_SelectorMatrix exercises the three valid backend-selection branches:
//
//	api            → apiBackend
//	script         → scriptBackend
//	cli_interactive → cliInteractiveBackend
func TestStartBackend_SelectorMatrix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name            string
		executionMode   string
		adapter         CLIAdapter
		wantBackendName string
	}{
		{
			name:            "api → apiBackend",
			executionMode:   "api",
			adapter:         &ClaudeAdapter{},
			wantBackendName: "api",
		},
		{
			name:            "script → scriptBackend",
			executionMode:   "script",
			adapter:         nil,
			wantBackendName: "script",
		},
		{
			name:            "cli_interactive → cliInteractiveBackend",
			executionMode:   "cli_interactive",
			adapter:         &ClaudeAdapter{},
			wantBackendName: "cli_interactive",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := New(Config{Clock: clock.Real()})
			backend := selectBackendForTest(s, tc.executionMode, tc.adapter)
			if backend == nil {
				t.Fatalf("selectBackendForTest(%q, %T) = nil, want %q backend",
					tc.executionMode, tc.adapter, tc.wantBackendName)
			}
			if got := backend.Name(); got != tc.wantBackendName {
				t.Errorf("selectBackendForTest(%q, adapter=%T) = %q, want %q",
					tc.executionMode, tc.adapter, got, tc.wantBackendName)
			}
		})
	}
}

// TestStartBackend_ReturnsErrorForUnknownMode verifies that startBackend returns an
// error for any execution_mode that is not "api", "script", or "cli_interactive".
// The "cli" mode literal was the deleted batch-mode; it must now produce an error.
func TestStartBackend_ReturnsErrorForUnknownMode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		executionMode string
	}{
		{"cli (deleted batch mode)", "cli"},
		{"empty string", ""},
		{"garbage", "nonsense"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := New(Config{Clock: clock.Real()})
			proc := &processInfo{agentType: "test-agent"}
			prep := &prepResult{
				executionMode: tc.executionMode,
				adapter:       &ClaudeAdapter{},
			}
			err := s.startBackend(proc, prep)
			if err == nil {
				t.Fatalf("startBackend(mode=%q): expected error, got nil", tc.executionMode)
			}
			if !strings.Contains(err.Error(), "unknown execution_mode") {
				t.Errorf("startBackend error = %q, want to contain %q", err.Error(), "unknown execution_mode")
			}
		})
	}
}

// TestEventAgentViewerAttached_DistinctFromTakeControl verifies it is different
// from the existing take-control event so UIs can distinguish them.
func TestEventAgentViewerAttached_DistinctFromTakeControl(t *testing.T) {
	t.Parallel()
	if ws.EventAgentViewerAttached == ws.EventAgentTakeControl {
		t.Errorf("EventAgentViewerAttached == EventAgentTakeControl (%q); they must differ",
			ws.EventAgentViewerAttached)
	}
}

// TestEventAgentViewerAttached_Broadcast verifies that broadcasting
// EventAgentViewerAttached delivers the correct payload fields (session_id,
// agent_type, model_id) — mirroring the monitorAll viewer-attach path.
func TestEventAgentViewerAttached_Broadcast(t *testing.T) {
	t.Parallel()
	hub := ws.NewHub(clock.Real())
	go hub.Run()
	defer hub.Stop()

	client, ch := ws.NewTestClient(hub, "ws-viewer-test")
	hub.Register(client)
	hub.Subscribe(client, "proj-ia", "T-200")

	sp := New(Config{Clock: clock.Real(), WSHub: hub})
	sp.broadcast(ws.EventAgentViewerAttached, "proj-ia", "T-200", "feature", map[string]interface{}{
		"session_id": "interactive-sess-1",
		"agent_type": "implementor",
		"model_id":   "claude:opus-4-7",
	})

	deadline := time.After(2 * time.Second)
	for {
		select {
		case msg := <-ch:
			var event ws.Event
			if err := json.Unmarshal(msg, &event); err != nil {
				continue
			}
			if event.Type != ws.EventAgentViewerAttached {
				continue
			}
			sessID, _ := event.Data["session_id"].(string)
			if sessID != "interactive-sess-1" {
				t.Errorf("session_id = %q, want interactive-sess-1", sessID)
			}
			agentType, _ := event.Data["agent_type"].(string)
			if agentType != "implementor" {
				t.Errorf("agent_type = %q, want implementor", agentType)
			}
			modelID, _ := event.Data["model_id"].(string)
			if modelID != "claude:opus-4-7" {
				t.Errorf("model_id = %q, want claude:opus-4-7", modelID)
			}
			return
		case <-deadline:
			t.Fatal("timeout waiting for EventAgentViewerAttached WS event")
		}
	}
}

// TestCLIInteractiveBackend_SupportsTakeControl_AlwaysTrue verifies that
// cliInteractiveBackend.SupportsTakeControl() returns true for all adapters.
func TestCLIInteractiveBackend_SupportsTakeControl_AlwaysTrue(t *testing.T) {
	t.Parallel()
	for _, adapter := range []CLIAdapter{&ClaudeAdapter{}, &CodexAdapter{}} {
		b := newCLIInteractiveBackend(adapter, nil, nil)
		if !b.SupportsTakeControl() {
			t.Errorf("cliInteractiveBackend(%T).SupportsTakeControl() = false, want true", adapter)
		}
	}
}
