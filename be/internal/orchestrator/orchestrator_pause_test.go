package orchestrator

import (
	"context"
	"testing"
	"time"

	"be/internal/model"
	"be/internal/service"
	"be/internal/ws"
)

// makeLayerGroups returns a simple 2-layer group for pause unit tests.
func makeLayerGroups() []layerGroup {
	return []layerGroup{
		{layer: 0, phases: []service.SpawnerPhaseDef{{Agent: "analyzer", Layer: 0}}},
		{layer: 1, phases: []service.SpawnerPhaseDef{{Agent: "builder", Layer: 1}}},
	}
}

// TestMaybePauseAfterLayer_NoOp_NoPauseFlag verifies no pause when layerPause flag is false.
func TestMaybePauseAfterLayer_NoOp_NoPauseFlag(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "PAU-NF1", "no pause flag")
	wfiID := env.initWorkflow(t, "PAU-NF1")

	req := RunRequest{ProjectID: env.project, TicketID: "PAU-NF1", WorkflowName: "test"}
	layerGroups := makeLayerGroups()

	got := env.orch.maybePauseAfterLayer(context.Background(), wfiID, req, 0, layerGroups, map[int]bool{0: false}, env.pool, t.TempDir())
	if got {
		t.Errorf("maybePauseAfterLayer() = true, want false (pause flag is false)")
	}

	wi := env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceActive {
		t.Errorf("status = %v, want active", wi.Status)
	}
	if _, ok := getWFIFindings(t, env, wfiID)["_pause"]; ok {
		t.Errorf("_pause finding present for no-op, want absent")
	}
}

// TestMaybePauseAfterLayer_NoOp_LastLayer verifies no pause when completed layer is last (no next).
func TestMaybePauseAfterLayer_NoOp_LastLayer(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "PAU-LL1", "last layer")
	wfiID := env.initWorkflow(t, "PAU-LL1")

	req := RunRequest{ProjectID: env.project, TicketID: "PAU-LL1", WorkflowName: "test"}
	layerGroups := makeLayerGroups() // len=2, so index 1 is last

	// completedLayerIdx=1 → 1+1 >= len(2), so no next layer
	got := env.orch.maybePauseAfterLayer(context.Background(), wfiID, req, 1, layerGroups, map[int]bool{1: true}, env.pool, t.TempDir())
	if got {
		t.Errorf("maybePauseAfterLayer() = true for last layer, want false")
	}
	wi := env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceActive {
		t.Errorf("status = %v, want active", wi.Status)
	}
}

// TestMaybePauseAfterLayer_NoOp_NegativeIdx verifies no pause for negative layer index.
func TestMaybePauseAfterLayer_NoOp_NegativeIdx(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "PAU-NI1", "negative idx")
	wfiID := env.initWorkflow(t, "PAU-NI1")

	req := RunRequest{ProjectID: env.project, TicketID: "PAU-NI1", WorkflowName: "test"}
	got := env.orch.maybePauseAfterLayer(context.Background(), wfiID, req, -1, makeLayerGroups(), map[int]bool{0: true}, env.pool, t.TempDir())
	if got {
		t.Errorf("maybePauseAfterLayer(-1) = true, want false")
	}
}

// TestMaybePauseAfterLayer_Command_Exit0 verifies pause with command hook exit 0:
// returns true, status→waiting, _pause finding with required keys, EventWorkflowPaused broadcast.
func TestMaybePauseAfterLayer_Command_Exit0(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "PAU-C0", "cmd exit 0")
	wfiID := env.initWorkflow(t, "PAU-C0")
	ch := env.subscribeWSClient(t, "ws-pau-c0", "PAU-C0")

	req := RunRequest{
		ProjectID:         env.project,
		TicketID:          "PAU-C0",
		WorkflowName:      "test",
		PauseEventCommand: "exit 0",
	}
	layerGroups := makeLayerGroups()

	got := env.orch.maybePauseAfterLayer(context.Background(), wfiID, req, 0, layerGroups, map[int]bool{0: true}, env.pool, t.TempDir())
	if !got {
		t.Fatalf("maybePauseAfterLayer() = false, want true")
	}

	wi := env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceWaiting {
		t.Errorf("status = %v, want waiting", wi.Status)
	}

	findings := getWFIFindings(t, env, wfiID)
	raw, ok := findings["_pause"]
	if !ok {
		t.Fatalf("_pause finding absent after pause")
	}
	m, ok := raw.(map[string]interface{})
	if !ok {
		t.Fatalf("_pause finding is not a map: %T", raw)
	}
	for _, key := range []string{"paused_after_layer", "resume_layer", "event", "timestamp"} {
		if _, exists := m[key]; !exists {
			t.Errorf("_pause finding missing key %q", key)
		}
	}
	if int(m["paused_after_layer"].(float64)) != 0 {
		t.Errorf("paused_after_layer = %v, want 0", m["paused_after_layer"])
	}
	if int(m["resume_layer"].(float64)) != 1 {
		t.Errorf("resume_layer = %v, want 1", m["resume_layer"])
	}
	ev, _ := m["event"].(map[string]interface{})
	if ev["status"] != "ok" {
		t.Errorf("event.status = %v, want ok", ev["status"])
	}
	if ev["kind"] != "command" {
		t.Errorf("event.kind = %v, want command", ev["kind"])
	}

	wsEv := expectEvent(t, ch, ws.EventWorkflowPaused, 2*time.Second)
	if wsEv.Data["instance_id"] != wfiID {
		t.Errorf("event instance_id = %v, want %v", wsEv.Data["instance_id"], wfiID)
	}
	if int(wsEv.Data["paused_after_layer"].(float64)) != 0 {
		t.Errorf("event paused_after_layer = %v, want 0", wsEv.Data["paused_after_layer"])
	}
	if int(wsEv.Data["resume_layer"].(float64)) != 1 {
		t.Errorf("event resume_layer = %v, want 1", wsEv.Data["resume_layer"])
	}
}

// TestMaybePauseAfterLayer_Command_Exit1 verifies pause proceeds even when hook exits 1.
func TestMaybePauseAfterLayer_Command_Exit1(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "PAU-C1", "cmd exit 1")
	wfiID := env.initWorkflow(t, "PAU-C1")
	ch := env.subscribeWSClient(t, "ws-pau-c1", "PAU-C1")

	req := RunRequest{
		ProjectID:         env.project,
		TicketID:          "PAU-C1",
		WorkflowName:      "test",
		PauseEventCommand: "exit 1",
	}

	got := env.orch.maybePauseAfterLayer(context.Background(), wfiID, req, 0, makeLayerGroups(), map[int]bool{0: true}, env.pool, t.TempDir())
	if !got {
		t.Fatalf("maybePauseAfterLayer() = false for exit 1 hook, want true (pause regardless)")
	}

	wi := env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceWaiting {
		t.Errorf("status = %v, want waiting", wi.Status)
	}

	m, _ := getWFIFindings(t, env, wfiID)["_pause"].(map[string]interface{})
	ev, _ := m["event"].(map[string]interface{})
	if ev["status"] == "ok" {
		t.Errorf("event.status = ok for exit 1 hook, want non-ok status")
	}

	expectEvent(t, ch, ws.EventWorkflowPaused, 2*time.Second)
}

// TestMaybePauseAfterLayer_NoHook verifies pause occurs with no hook configured.
func TestMaybePauseAfterLayer_NoHook(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "PAU-NH1", "no hook")
	wfiID := env.initWorkflow(t, "PAU-NH1")
	ch := env.subscribeWSClient(t, "ws-pau-nh1", "PAU-NH1")

	req := RunRequest{ProjectID: env.project, TicketID: "PAU-NH1", WorkflowName: "test"}

	got := env.orch.maybePauseAfterLayer(context.Background(), wfiID, req, 0, makeLayerGroups(), map[int]bool{0: true}, env.pool, t.TempDir())
	if !got {
		t.Fatalf("maybePauseAfterLayer() = false with no hook, want true")
	}

	wi := env.getWorkflowInstance(t, wfiID)
	if wi.Status != model.WorkflowInstanceWaiting {
		t.Errorf("status = %v, want waiting", wi.Status)
	}

	m, _ := getWFIFindings(t, env, wfiID)["_pause"].(map[string]interface{})
	ev, _ := m["event"].(map[string]interface{})
	if ev["kind"] != "" {
		t.Errorf("event.kind = %v, want empty (no hook)", ev["kind"])
	}

	expectEvent(t, ch, ws.EventWorkflowPaused, 2*time.Second)
}
