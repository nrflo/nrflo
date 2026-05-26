package orchestrator

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"be/internal/repo"
	"be/internal/service"
	"be/internal/ws"
)

// drainEvents collects all WS events available on ch within window.
func drainEvents(t *testing.T, ch chan []byte, window time.Duration) []ws.Event {
	t.Helper()
	var out []ws.Event
	deadline := time.After(window)
	for {
		select {
		case msg := <-ch:
			var e ws.Event
			if err := json.Unmarshal(msg, &e); err != nil {
				t.Fatalf("unmarshal event: %v", err)
			}
			out = append(out, e)
		case <-deadline:
			return out
		}
	}
}

func skippedSessionAgents(t *testing.T, env *testEnv, wfiID string) []string {
	t.Helper()
	rows, err := env.pool.Query(
		`SELECT agent_type FROM agent_sessions WHERE workflow_instance_id = ? AND status = 'skipped' ORDER BY agent_type`, wfiID)
	if err != nil {
		t.Fatalf("query skipped sessions: %v", err)
	}
	defer rows.Close()
	var agents []string
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			t.Fatalf("scan: %v", err)
		}
		agents = append(agents, a)
	}
	return agents
}

// TestApplyLayerSkips_Partial verifies that one of two same-layer agents is skipped
// while the other remains runnable: only the skipped agent gets a skipped session and
// a per-agent skipped event, and no whole-layer skip event is emitted.
func TestApplyLayerSkips_Partial(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "PSKIP-1", "Partial skip test")
	wfiID := env.initWorkflow(t, "PSKIP-1")

	wfiRepo := repo.NewWorkflowInstanceRepo(env.pool, env.orch.clock)
	if err := wfiRepo.UpdateSkipTags(wfiID, `["fe"]`); err != nil {
		t.Fatalf("UpdateSkipTags: %v", err)
	}

	ch := env.subscribeWSClient(t, "ws-partial", "PSKIP-1")
	req := RunRequest{ProjectID: env.project, TicketID: "PSKIP-1", WorkflowName: "test", ScopeType: "ticket"}
	phases := []service.SpawnerPhaseDef{{Agent: "fe-impl", Layer: 1}, {Agent: "be-impl", Layer: 1}}
	agentTags := map[string]string{"fe-impl": "fe", "be-impl": "be"}

	runnable, wholeSkip := env.orch.applyLayerSkips(context.Background(), wfiID, req, phases, agentTags, env.pool)

	if wholeSkip {
		t.Error("wholeLayerSkipped = true, want false (partial skip)")
	}
	if len(runnable) != 1 || runnable[0].Agent != "be-impl" {
		t.Errorf("runnable = %v, want [be-impl]", agentNamesOf(runnable))
	}
	if got := skippedSessionAgents(t, env, wfiID); len(got) != 1 || got[0] != "fe-impl" {
		t.Errorf("skipped sessions = %v, want [fe-impl]", got)
	}

	events := drainEvents(t, ch, 300*time.Millisecond)
	var skippedAgents []string
	for _, e := range events {
		if e.Type == ws.EventLayerSkipped {
			t.Error("EventLayerSkipped broadcast on partial skip, want none")
		}
		if e.Type == ws.EventAgentCompleted && e.Data["result"] == "skipped" {
			skippedAgents = append(skippedAgents, e.Data["agent_id"].(string))
		}
	}
	if len(skippedAgents) != 1 || skippedAgents[0] != "fe-impl" {
		t.Errorf("per-agent skipped events = %v, want [fe-impl]", skippedAgents)
	}
}

// TestApplyLayerSkips_Whole verifies that when every agent matches a skip tag the
// whole layer is skipped: all agents get skipped sessions and a layer-skipped event fires.
func TestApplyLayerSkips_Whole(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "WSKIP-1", "Whole skip test")
	wfiID := env.initWorkflow(t, "WSKIP-1")

	wfiRepo := repo.NewWorkflowInstanceRepo(env.pool, env.orch.clock)
	if err := wfiRepo.UpdateSkipTags(wfiID, `["fe","be"]`); err != nil {
		t.Fatalf("UpdateSkipTags: %v", err)
	}

	ch := env.subscribeWSClient(t, "ws-whole", "WSKIP-1")
	req := RunRequest{ProjectID: env.project, TicketID: "WSKIP-1", WorkflowName: "test", ScopeType: "ticket"}
	phases := []service.SpawnerPhaseDef{{Agent: "fe-impl", Layer: 1}, {Agent: "be-impl", Layer: 1}}
	agentTags := map[string]string{"fe-impl": "fe", "be-impl": "be"}

	runnable, wholeSkip := env.orch.applyLayerSkips(context.Background(), wfiID, req, phases, agentTags, env.pool)

	if !wholeSkip {
		t.Error("wholeLayerSkipped = false, want true (all agents skipped)")
	}
	if len(runnable) != 0 {
		t.Errorf("runnable = %v, want empty", agentNamesOf(runnable))
	}
	if got := skippedSessionAgents(t, env, wfiID); len(got) != 2 {
		t.Errorf("skipped sessions = %v, want 2", got)
	}

	layerEvent := expectEvent(t, ch, ws.EventLayerSkipped, 2*time.Second)
	if layerEvent.Data["skip_tag"] == "" {
		t.Error("EventLayerSkipped skip_tag empty, want a matched tag")
	}
}
