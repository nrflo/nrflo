package spawner

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"

	"github.com/google/uuid"
)

// TestContextSaveViaAgent_FreshDigest_SkipsContextSaverSpawn verifies that
// when a fresh autonomous refinery slot digest exists for the killed
// session's (workflow_instance_id, node_id) slot, contextSaveViaAgent skips
// the ws.EventAgentContextSaving broadcast and the context-saver spawn
// entirely, but still completes the shared stop/continue tail.
func TestContextSaveViaAgent_FreshDigest_SkipsContextSaverSpawn(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	clk := clock.NewTest(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	sessionStart := clk.Now().Add(-time.Minute)
	sessionID := env.createSessionWithStartTime(t, sessionStart)

	nodeID := "test-phase"
	digestRepo := repo.NewRefineryDigestRepo(env.database, clk)
	if _, err := digestRepo.UpsertSlot(env.wfiID, nodeID, env.projectID, "DIGEST-TEXT"); err != nil {
		t.Fatalf("UpsertSlot: %v", err)
	}

	hub := ws.NewHub(clk)
	go hub.Run()
	defer hub.Stop()
	client, ch := ws.NewTestClient(hub, "client-digest-skip")
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
		nodeID:             nodeID,
		workflowInstanceID: env.wfiID,
		startTime:          sessionStart,
	}
	req := SpawnRequest{ProjectID: env.projectID, TicketID: env.ticketID, WorkflowName: "feature"}

	sp.contextSaveViaAgent(context.Background(), proc, req)

	// The shared stop tail still broadcasts agent.completed; only the
	// context_saving event must be absent. Drain whatever arrives within a
	// short window and assert none of it is context_saving.
drainLoop:
	for {
		select {
		case msg := <-ch:
			var event ws.Event
			if err := json.Unmarshal(msg, &event); err != nil {
				t.Fatalf("unmarshal event: %v", err)
			}
			if event.Type == ws.EventAgentContextSaving {
				t.Fatalf("unexpected agent.context_saving broadcast when a fresh digest should skip it: %s", msg)
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

// TestContextSaveViaAgent_NoDigest_FallsBackToAgentSave verifies that
// without a fresh slot digest, contextSaveViaAgent runs the existing
// agent-save path: the context_saving event is broadcast before the
// context-saver spawn attempt.
func TestContextSaveViaAgent_NoDigest_FallsBackToAgentSave(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	clk := clock.NewTest(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	sessionStart := clk.Now().Add(-time.Minute)
	sessionID := env.createSessionWithStartTime(t, sessionStart)

	hub := ws.NewHub(clk)
	go hub.Run()
	defer hub.Stop()
	client, ch := ws.NewTestClient(hub, "client-digest-fallback")
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

	select {
	case msg := <-ch:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		if event.Type != ws.EventAgentContextSaving {
			t.Errorf("event.Type = %q, want %q", event.Type, ws.EventAgentContextSaving)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for agent.context_saving event on the fallback path")
	}

	if proc.finalStatus != "CONTINUE" {
		t.Errorf("proc.finalStatus = %q, want CONTINUE", proc.finalStatus)
	}

	status := env.sessionStatus(t, sessionID)
	if status != string(model.AgentSessionContinued) {
		t.Errorf("session status = %q, want %q", status, model.AgentSessionContinued)
	}
}

// TestContextSaveViaAgent_StaleDigest_FallsBackToAgentSave verifies a digest
// folded before the killed session started (stale) does not suppress the
// agent-save broadcast.
func TestContextSaveViaAgent_StaleDigest_FallsBackToAgentSave(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	clk := clock.NewTest(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	sessionStart := clk.Now()
	sessionID := env.createSessionWithStartTime(t, sessionStart)

	nodeID := "test-phase"
	staleClk := clock.NewTest(sessionStart.Add(-time.Nanosecond))
	digestRepo := repo.NewRefineryDigestRepo(env.database, staleClk)
	if _, err := digestRepo.UpsertSlot(env.wfiID, nodeID, env.projectID, "STALE-DIGEST"); err != nil {
		t.Fatalf("UpsertSlot: %v", err)
	}

	hub := ws.NewHub(clk)
	go hub.Run()
	defer hub.Stop()
	client, ch := ws.NewTestClient(hub, "client-digest-stale")
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
		nodeID:             nodeID,
		workflowInstanceID: env.wfiID,
		startTime:          sessionStart,
	}
	req := SpawnRequest{ProjectID: env.projectID, TicketID: env.ticketID, WorkflowName: "feature"}

	sp.contextSaveViaAgent(context.Background(), proc, req)

	select {
	case msg := <-ch:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		if event.Type != ws.EventAgentContextSaving {
			t.Errorf("event.Type = %q, want %q", event.Type, ws.EventAgentContextSaving)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for agent.context_saving event with a stale digest")
	}
}

// createSessionWithStartTime creates a running session with an explicit
// started_at so freshSlotDigest can compare against it.
func (env *contextSaveTestEnv) createSessionWithStartTime(t *testing.T, startedAt time.Time) string {
	t.Helper()

	sessionID := uuid.New().String()
	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	session := &model.AgentSession{
		ID:                 sessionID,
		ProjectID:          env.projectID,
		TicketID:           env.ticketID,
		WorkflowInstanceID: env.wfiID,
		Phase:              "test-phase",
		NodeID:             "test-phase",
		AgentType:          "test-agent",
		ModelID:            sql.NullString{String: "claude:sonnet-5", Valid: true},
		Status:             model.AgentSessionRunning,
		StartedAt:          sql.NullString{String: startedAt.UTC().Format(time.RFC3339Nano), Valid: true},
	}
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	return sessionID
}

// sessionStatus reads back the persisted agent_sessions.status column.
func (env *contextSaveTestEnv) sessionStatus(t *testing.T, sessionID string) string {
	t.Helper()
	var status string
	if err := env.database.QueryRow("SELECT status FROM agent_sessions WHERE id = ?", sessionID).Scan(&status); err != nil {
		t.Fatalf("failed to read session status: %v", err)
	}
	return status
}
