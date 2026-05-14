package spawner

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/ws"
)

// mockNoInteractiveAdapter wraps ClaudeAdapter but overrides SupportsInteractive
// to return false, representing a hypothetical adapter that does not support
// PTY-based interactive execution.
type mockNoInteractiveAdapter struct{ ClaudeAdapter }

func (m *mockNoInteractiveAdapter) SupportsInteractive() bool { return false }

// selectBackendForTest mirrors startBackend's four-way switch on executionMode,
// enabling clean unit tests of the selector without calling Start() or registerAgentStart().
func selectBackendForTest(s *Spawner, executionMode string, adapter CLIAdapter) ExecutionBackend {
	switch executionMode {
	case "api":
		return newAPIBackend(s)
	case "script":
		return newScriptBackend(s)
	case "cli_interactive":
		return newCLIInteractiveBackend(adapter, s, nil)
	default:
		return newCLIBackend(adapter, nil)
	}
}

// TestStartBackend_SelectorMatrix exercises all four backend-selection branches:
//
//	api            → apiBackend
//	script         → scriptBackend
//	cli_interactive → cliInteractiveBackend
//	cli / default  → cliBackend
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
		{
			name:            "cli → cliBackend",
			executionMode:   "cli",
			adapter:         &ClaudeAdapter{},
			wantBackendName: "cli",
		},
		{
			name:            "empty → cliBackend (default)",
			executionMode:   "",
			adapter:         &ClaudeAdapter{},
			wantBackendName: "cli",
		},
		// cli_interactive + opencode is intentionally absent from this selector matrix:
		// startBackend rejects it via the SupportsInteractive() guard (see TestStartBackend_RejectsOpencodeInteractive).
		// selectBackendForTest bypasses that guard, so including it here would be misleading.
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := New(Config{Clock: clock.Real()})
			backend := selectBackendForTest(s, tc.executionMode, tc.adapter)
			if got := backend.Name(); got != tc.wantBackendName {
				t.Errorf("selectBackendForTest(%q, adapter=%T) = %q, want %q",
					tc.executionMode, tc.adapter, got, tc.wantBackendName)
			}
		})
	}
}

// TestEventAgentViewerAttached_Constant verifies the WS event constant uses the
// resource.action naming convention expected by the web UI.
func TestEventAgentViewerAttached_Constant(t *testing.T) {
	t.Parallel()
	const want = "agent.viewer_attached"
	if ws.EventAgentViewerAttached != want {
		t.Errorf("EventAgentViewerAttached = %q, want %q", ws.EventAgentViewerAttached, want)
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
		"model_id":   "claude:opus_4_7",
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
			if modelID != "claude:opus_4_7" {
				t.Errorf("model_id = %q, want claude:opus_4_7", modelID)
			}
			return
		case <-deadline:
			t.Fatal("timeout waiting for EventAgentViewerAttached WS event")
		}
	}
}

// TestCLIInteractiveBackend_SupportsTakeControl_AlwaysTrue verifies that
// cliInteractiveBackend.SupportsTakeControl() returns true for all adapters —
// unlike cliBackend which gates on SupportsResume().
func TestCLIInteractiveBackend_SupportsTakeControl_AlwaysTrue(t *testing.T) {
	t.Parallel()
	for _, adapter := range []CLIAdapter{&ClaudeAdapter{}, &OpencodeAdapter{}, &CodexAdapter{}} {
		b := newCLIInteractiveBackend(adapter, nil, nil)
		if !b.SupportsTakeControl() {
			t.Errorf("cliInteractiveBackend(%T).SupportsTakeControl() = false, want true", adapter)
		}
	}
}

// TestStartBackend_RejectsOpencodeInteractive verifies the SupportsInteractive guard
// at startBackend: OpencodeAdapter.SupportsInteractive() returns false, so requesting
// cli_interactive execution mode must return an error.
func TestStartBackend_RejectsOpencodeInteractive(t *testing.T) {
	t.Parallel()
	s := New(Config{Clock: clock.Real()})
	proc := &processInfo{agentType: "test-agent"}
	prep := &prepResult{
		executionMode: "cli_interactive",
		adapter:       &OpencodeAdapter{},
	}
	err := s.startBackend(proc, prep)
	if err == nil {
		t.Fatal("startBackend(opencode, cli_interactive): expected error, got nil")
	}
	if !strings.Contains(err.Error(), "does not support PTY interactive mode") {
		t.Errorf("startBackend error = %q, want to contain %q", err.Error(), "does not support PTY interactive mode")
	}
}
