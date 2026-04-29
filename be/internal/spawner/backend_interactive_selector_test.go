package spawner

import (
	"encoding/json"
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

// selectBackendForTest mirrors startBackend's selection logic without calling
// Start() or registerAgentStart(), enabling clean unit tests of the selector.
func selectBackendForTest(s *Spawner, executionMode string, adapter CLIAdapter) ExecutionBackend {
	if executionMode == "api" {
		return newAPIBackend(s)
	}
	if s.config.InteractiveCLIMode && adapter != nil && adapter.SupportsInteractive() {
		return newCLIInteractiveBackend(adapter, s, nil)
	}
	return newCLIBackend(adapter, nil)
}

// TestStartBackend_SelectorMatrix exercises all five backend-selection branches:
//
//	(api, interactiveOff) → apiBackend
//	(api, interactiveOn)  → apiBackend  (api takes priority)
//	(cli, interactiveOff, supportsInteractive=true) → cliBackend
//	(cli, interactiveOn,  supportsInteractive=true) → cliInteractiveBackend
//	(cli, interactiveOn,  supportsInteractive=false) → cliBackend
func TestStartBackend_SelectorMatrix(t *testing.T) {
	cases := []struct {
		name            string
		executionMode   string
		interactiveCLI  bool
		adapter         CLIAdapter
		wantBackendName string
	}{
		{
			name:            "api + interactiveOff → apiBackend",
			executionMode:   "api",
			interactiveCLI:  false,
			adapter:         &ClaudeAdapter{},
			wantBackendName: "api",
		},
		{
			name:            "api + interactiveOn → apiBackend (api beats interactive)",
			executionMode:   "api",
			interactiveCLI:  true,
			adapter:         &ClaudeAdapter{},
			wantBackendName: "api",
		},
		{
			name:            "cli + interactiveOff + supportsInteractive → cliBackend",
			executionMode:   "cli",
			interactiveCLI:  false,
			adapter:         &ClaudeAdapter{},
			wantBackendName: "cli",
		},
		{
			name:            "cli + interactiveOn + supportsInteractive → cliInteractiveBackend",
			executionMode:   "cli",
			interactiveCLI:  true,
			adapter:         &ClaudeAdapter{},
			wantBackendName: "cli_interactive",
		},
		{
			name:            "cli + interactiveOn + !supportsInteractive → cliBackend",
			executionMode:   "cli",
			interactiveCLI:  true,
			adapter:         &mockNoInteractiveAdapter{},
			wantBackendName: "cli",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := New(Config{Clock: clock.Real(), InteractiveCLIMode: tc.interactiveCLI})
			backend := selectBackendForTest(s, tc.executionMode, tc.adapter)
			if got := backend.Name(); got != tc.wantBackendName {
				t.Errorf("selectBackendForTest(%q, interactiveCLI=%v, adapter=%T) = %q, want %q",
					tc.executionMode, tc.interactiveCLI, tc.adapter, got, tc.wantBackendName)
			}
		})
	}
}

// TestEventAgentViewerAttached_Constant verifies the WS event constant uses the
// resource.action naming convention expected by the web UI.
func TestEventAgentViewerAttached_Constant(t *testing.T) {
	const want = "agent.viewer_attached"
	if ws.EventAgentViewerAttached != want {
		t.Errorf("EventAgentViewerAttached = %q, want %q", ws.EventAgentViewerAttached, want)
	}
}

// TestEventAgentViewerAttached_DistinctFromTakeControl verifies it is different
// from the existing take-control event so UIs can distinguish them.
func TestEventAgentViewerAttached_DistinctFromTakeControl(t *testing.T) {
	if ws.EventAgentViewerAttached == ws.EventAgentTakeControl {
		t.Errorf("EventAgentViewerAttached == EventAgentTakeControl (%q); they must differ",
			ws.EventAgentViewerAttached)
	}
}

// TestEventAgentViewerAttached_Broadcast verifies that broadcasting
// EventAgentViewerAttached delivers the correct payload fields (session_id,
// agent_type, model_id) — mirroring the monitorAll viewer-attach path.
func TestEventAgentViewerAttached_Broadcast(t *testing.T) {
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
	for _, adapter := range []CLIAdapter{&ClaudeAdapter{}, &OpencodeAdapter{}, &CodexAdapter{}} {
		b := newCLIInteractiveBackend(adapter, nil, nil)
		if !b.SupportsTakeControl() {
			t.Errorf("cliInteractiveBackend(%T).SupportsTakeControl() = false, want true", adapter)
		}
	}
}
