package spawner

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"

	"github.com/google/uuid"
)

// TestIdleWindowElapsed_TableDriven covers idleWindowElapsed's window
// selection (first-message vs after-message) and the disabled (<=0) case.
func TestIdleWindowElapsed_TableDriven(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clk := clock.NewTest(now)

	tests := []struct {
		name                    string
		hasReceivedMessage      bool
		lastMessageTime         time.Time
		idleAfterMessageTimeout time.Duration
		idleStartTimeout        time.Duration
		want                    bool
	}{
		{
			name: "no messages yet, past start window", hasReceivedMessage: false,
			lastMessageTime: now.Add(-3 * time.Minute), idleStartTimeout: 2 * time.Minute, idleAfterMessageTimeout: time.Hour,
			want: true,
		},
		{
			name: "no messages yet, within start window", hasReceivedMessage: false,
			lastMessageTime: now.Add(-1 * time.Minute), idleStartTimeout: 2 * time.Minute, idleAfterMessageTimeout: time.Hour,
			want: false,
		},
		{
			name: "has messages, past after-message window", hasReceivedMessage: true,
			lastMessageTime: now.Add(-5 * time.Minute), idleAfterMessageTimeout: 4 * time.Minute, idleStartTimeout: time.Hour,
			want: true,
		},
		{
			name: "has messages, within after-message window", hasReceivedMessage: true,
			lastMessageTime: now.Add(-1 * time.Minute), idleAfterMessageTimeout: 4 * time.Minute, idleStartTimeout: time.Hour,
			want: false,
		},
		{
			name: "after-message window disabled (<=0)", hasReceivedMessage: true,
			lastMessageTime: now.Add(-time.Hour), idleAfterMessageTimeout: 0, idleStartTimeout: time.Hour,
			want: false,
		},
		{
			name: "start window disabled (<=0)", hasReceivedMessage: false,
			lastMessageTime: now.Add(-time.Hour), idleStartTimeout: 0, idleAfterMessageTimeout: time.Hour,
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			proc := &processInfo{
				lastMessageTime:         tc.lastMessageTime,
				hasReceivedMessage:      tc.hasReceivedMessage,
				idleAfterMessageTimeout: tc.idleAfterMessageTimeout,
				idleStartTimeout:        tc.idleStartTimeout,
			}
			if got := idleWindowElapsed(clk, proc); got != tc.want {
				t.Errorf("idleWindowElapsed() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestLastPlanItemInFlight covers the SQL-driven safety rail: only the
// highest-layer materialized node's running session counts as "in flight".
func TestLastPlanItemInFlight(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	if got := lastPlanItemInFlight(nil, env.wfiID); got {
		t.Error("lastPlanItemInFlight(nil pool) = true, want false")
	}
	if got := lastPlanItemInFlight(env.spawner.pool(), ""); got {
		t.Error("lastPlanItemInFlight(empty instance id) = true, want false")
	}
	if got := lastPlanItemInFlight(env.spawner.pool(), env.wfiID); got {
		t.Error("lastPlanItemInFlight() with no materialized nodes = true, want false")
	}

	insertPlanNode(t, env, "node-layer0", 0, "implementor")
	insertPlanNode(t, env, "node-layer1", 1, "qa-verifier")

	// A running session on the LOWER layer's node must not count.
	insertRunningSession(t, env, "node-layer0")
	if got := lastPlanItemInFlight(env.spawner.pool(), env.wfiID); got {
		t.Error("lastPlanItemInFlight() with only a lower-layer session running = true, want false")
	}

	// A running session on the highest-layer's node counts.
	insertRunningSession(t, env, "node-layer1")
	if got := lastPlanItemInFlight(env.spawner.pool(), env.wfiID); !got {
		t.Error("lastPlanItemInFlight() with the highest-layer session running = false, want true")
	}
}

// TestLastPlanItemInFlight_EndedSessionDoesNotCount verifies a completed
// (ended_at set) session on the highest-layer node is not "in flight".
func TestLastPlanItemInFlight_EndedSessionDoesNotCount(t *testing.T) {
	t.Parallel()
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	insertPlanNode(t, env, "node-final", 0, "implementor")
	insertEndedSession(t, env, "node-final")

	if got := lastPlanItemInFlight(env.spawner.pool(), env.wfiID); got {
		t.Error("lastPlanItemInFlight() with an ended session on the highest-layer node = true, want false")
	}
}

// TestApplyProactiveRotationCarry_ResetsOnRotationPending verifies a
// proactive rotation resets restartCount (instead of incrementing it) so it
// does not spend the emergency continuation budget, while still bumping
// proactiveRestartCount.
func TestApplyProactiveRotationCarry_ResetsOnRotationPending(t *testing.T) {
	t.Parallel()
	oldProc := &processInfo{
		sessionID:                "old-sess",
		restartCount:             5,
		proactiveRestartCount:    2,
		proactiveRotationPending: true,
		proactiveTokensBefore:    50000,
	}
	newProc := &processInfo{sessionID: "new-sess-" + uuid.New().String()}
	t.Cleanup(func() { globalLedgerStore.drop(newProc.sessionID) })

	applyProactiveRotationCarry(context.Background(), oldProc, newProc, "ancestor-1")

	if newProc.ancestorSessionID != "ancestor-1" {
		t.Errorf("ancestorSessionID = %q, want ancestor-1", newProc.ancestorSessionID)
	}
	if newProc.restartCount != 0 {
		t.Errorf("restartCount = %d, want 0 (proactive rotation resets, does not increment)", newProc.restartCount)
	}
	if newProc.proactiveRestartCount != 3 {
		t.Errorf("proactiveRestartCount = %d, want 3 (2+1)", newProc.proactiveRestartCount)
	}
}

// TestApplyProactiveRotationCarry_NormalContinuationIncrements verifies a
// non-proactive continuation (proactiveRotationPending=false) preserves the
// existing behavior: restartCount increments, proactiveRestartCount is
// untouched.
func TestApplyProactiveRotationCarry_NormalContinuationIncrements(t *testing.T) {
	t.Parallel()
	oldProc := &processInfo{
		sessionID:                "old-sess-2",
		restartCount:             3,
		proactiveRestartCount:    1,
		proactiveRotationPending: false,
	}
	newProc := &processInfo{sessionID: "new-sess-2"}

	applyProactiveRotationCarry(context.Background(), oldProc, newProc, "ancestor-2")

	if newProc.restartCount != 4 {
		t.Errorf("restartCount = %d, want 4 (normal increment)", newProc.restartCount)
	}
	if newProc.proactiveRestartCount != 0 {
		t.Errorf("proactiveRestartCount = %d, want 0 (untouched by a non-proactive continuation)", newProc.proactiveRestartCount)
	}
	if newProc.ancestorSessionID != "ancestor-2" {
		t.Errorf("ancestorSessionID = %q, want ancestor-2", newProc.ancestorSessionID)
	}
}

// TestCheckProactiveRestart_LastPlanItemInFlight_Skips wires the
// lastPlanItemInFlight SQL rail into the full checkProactiveRestart call: a
// running session on the highest-layer materialized node must block a
// proactive rotation that would otherwise fire.
func TestCheckProactiveRestart_LastPlanItemInFlight_Skips(t *testing.T) {
	t.Parallel()
	sp, env, clk := newProactiveRestartTestEnv(t)
	defer env.cleanup()

	sessionID := env.createSessionWithFindings(t, map[string]interface{}{})
	t.Cleanup(func() {
		globalLedgerStore.drop(sessionID)
		DropProactiveRestartState(sessionID)
	})

	proc, backend := newProactiveTestProc(env, clk, sessionID, "cli_interactive", false)
	globalLedgerStore.get(sessionID).append(LedgerKindDialog, 300000, "", "", false)

	insertPlanNode(t, env, "node-final", 0, "implementor")
	insertRunningSession(t, env, "node-final") // the run's last item is still in flight

	req := SpawnRequest{ProjectID: env.projectID, TicketID: env.ticketID, WorkflowName: "feature", WorkflowInstanceID: env.wfiID}
	originalDoneCh := proc.doneCh

	sp.checkProactiveRestart(context.Background(), proc, req)

	if proc.doneCh != originalDoneCh {
		t.Error("checkProactiveRestart fired while the last plan item was in flight")
	}
	if backend.wasKilled() {
		t.Error("backend.Kill was called while the last plan item was in flight")
	}
}

// insertPlanNode inserts a workflow_instance_nodes row for env.wfiID.
func insertPlanNode(t *testing.T, env *contextSaveTestEnv, nodeID string, layer int, agentType string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.database.Exec(
		`INSERT INTO workflow_instance_nodes (instance_id, node_id, layer, agent_type, instructions, plan_revision, created_at)
		 VALUES (?, ?, ?, ?, '', 1, ?)`,
		env.wfiID, nodeID, layer, agentType, now,
	); err != nil {
		t.Fatalf("insert workflow_instance_nodes: %v", err)
	}
}

// insertRunningSession inserts an agent_sessions row for nodeID with no
// ended_at (still running).
func insertRunningSession(t *testing.T, env *contextSaveTestEnv, nodeID string) {
	t.Helper()
	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	session := &model.AgentSession{
		ID:                 uuid.New().String(),
		ProjectID:          env.projectID,
		TicketID:           env.ticketID,
		WorkflowInstanceID: env.wfiID,
		Phase:              nodeID,
		NodeID:             nodeID,
		AgentType:          "test-agent",
		ModelID:            sql.NullString{String: "claude:sonnet-5", Valid: true},
		Status:             model.AgentSessionRunning,
		StartedAt:          sql.NullString{String: time.Now().UTC().Format(time.RFC3339Nano), Valid: true},
	}
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("insert running session: %v", err)
	}
}

// insertEndedSession inserts an agent_sessions row for nodeID with ended_at
// set (no longer in flight).
func insertEndedSession(t *testing.T, env *contextSaveTestEnv, nodeID string) {
	t.Helper()
	sessionRepo := repo.NewAgentSessionRepo(env.database, clock.Real())
	now := time.Now().UTC().Format(time.RFC3339Nano)
	session := &model.AgentSession{
		ID:                 uuid.New().String(),
		ProjectID:          env.projectID,
		TicketID:           env.ticketID,
		WorkflowInstanceID: env.wfiID,
		Phase:              nodeID,
		NodeID:             nodeID,
		AgentType:          "test-agent",
		ModelID:            sql.NullString{String: "claude:sonnet-5", Valid: true},
		Status:             model.AgentSessionCompleted,
		Result:             sql.NullString{String: "pass", Valid: true},
		StartedAt:          sql.NullString{String: now, Valid: true},
		EndedAt:            sql.NullString{String: now, Valid: true},
	}
	if err := sessionRepo.Create(session); err != nil {
		t.Fatalf("insert ended session: %v", err)
	}
}
