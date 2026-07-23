package spawner

// Tests contextSaveViaAgent's stepwise skip gate: the cursor is the save, so
// the context_saving broadcast and the context-saver spawn are both skipped
// for a stepwise def — mirroring context_save_digest_test.go's
// fresh-digest-skip pattern. Full-mode behavior (broadcast + spawn attempt)
// is already covered by that file; this is additive, stepwise-only.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/ws"
)

// TestContextSaveViaAgent_StepwiseDef_SkipsContextSaverSpawn verifies that
// for a stepwise agent def, contextSaveViaAgent never broadcasts
// agent.context_saving and completes the shared stop/continue tail without
// attempting a context-saver spawn (which would otherwise require a real
// system agent def and CLI process).
func TestContextSaveViaAgent_StepwiseDef_SkipsContextSaverSpawn(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	clk := clock.NewTest(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	sessionStart := clk.Now().Add(-time.Minute)
	sessionID := env.createSessionWithStartTime(t, sessionStart)

	createStepwiseAgentDefInContextEnv(t, env, "test-agent", "sonnet-5", threeSteps())

	hub := ws.NewHub(clk)
	go hub.Run()
	defer hub.Stop()
	client, ch := ws.NewTestClient(hub, "client-stepwise-skip")
	hub.Register(client)
	hub.Subscribe(client, env.projectID, env.ticketID)

	sp := New(Config{
		DataPath: env.dbPath,
		Pool:     env.spawner.config.Pool,
		Clock:    clk,
		WSHub:    hub,
	})

	proc := &processInfo{
		sessionID:          sessionID,
		agentID:            "agent-1",
		agentType:          "test-agent",
		modelID:            "claude:sonnet-5",
		nodeID:             "test-phase",
		workflowInstanceID: env.wfiID,
		startTime:          sessionStart,
	}
	req := SpawnRequest{ProjectID: env.projectID, TicketID: env.ticketID, WorkflowName: "feature"}

	sp.contextSaveViaAgent(context.Background(), proc, req)

drainLoop:
	for {
		select {
		case msg := <-ch:
			var event ws.Event
			if err := json.Unmarshal(msg, &event); err != nil {
				t.Fatalf("unmarshal event: %v", err)
			}
			if event.Type == ws.EventAgentContextSaving {
				t.Fatalf("unexpected agent.context_saving broadcast for a stepwise def: %s", msg)
			}
		case <-time.After(150 * time.Millisecond):
			break drainLoop
		}
	}

	if proc.finalStatus != "CONTINUE" {
		t.Errorf("proc.finalStatus = %q, want CONTINUE", proc.finalStatus)
	}

	status := env.sessionStatus(t, sessionID)
	if status != string(model.AgentSessionContinued) {
		t.Errorf("session status = %q, want %q", status, model.AgentSessionContinued)
	}
}
