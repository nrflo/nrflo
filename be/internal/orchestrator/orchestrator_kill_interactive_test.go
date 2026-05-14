package orchestrator

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/spawner"
	"be/internal/ws"
	"be/internal/clock"
)

// insertInteractiveSession inserts an agent_sessions row with status=user_interactive.
func insertInteractiveSession(t *testing.T, env *testEnv, wfiID, ticketID, sessionID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := env.pool.Exec(`
		INSERT INTO agent_sessions
			(id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
			 model_id, status, result, result_reason, pid, findings,
			 context_left, ancestor_session_id, spawn_command, prompt,
			 restart_count, started_at, ended_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?, ?, ?)`,
		sessionID, env.project, ticketID, wfiID, "test-phase", "test-agent",
		sql.NullString{String: "claude:sonnet", Valid: true},
		"user_interactive",
		sql.NullString{}, sql.NullString{}, sql.NullInt64{}, sql.NullString{},
		sql.NullInt64{}, sql.NullString{}, sql.NullString{}, sql.NullString{},
		0, sql.NullString{String: now, Valid: true}, sql.NullString{},
		now, now,
	)
	if err != nil {
		t.Fatalf("insertInteractiveSession: %v", err)
	}
}

// TestKillInteractive_UnknownSession verifies an error is returned for a nonexistent session.
func TestKillInteractive_UnknownSession(t *testing.T) {
	env := newTestEnv(t)

	err := env.orch.KillInteractive("does-not-exist-xyz")
	if err == nil {
		t.Fatal("expected error for unknown session")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain 'not found'", err.Error())
	}
}

// TestKillInteractive_RejectsNonInteractive verifies that sessions not in
// user_interactive status are rejected with an informative error.
func TestKillInteractive_RejectsNonInteractive(t *testing.T) {
	cases := []struct {
		status model.AgentSessionStatus
	}{
		{model.AgentSessionRunning},
		{model.AgentSessionInteractiveCompleted},
		{model.AgentSessionFailed},
		{model.AgentSessionCompleted},
		{model.AgentSessionSkipped},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(string(tc.status), func(t *testing.T) {
			env := newTestEnv(t)
			env.createTicket(t, "TKT-KI-REJ", "reject test")
			wfiID := env.initWorkflow(t, "TKT-KI-REJ")

			now := time.Now().UTC().Format(time.RFC3339Nano)
			sid := "sess-rej-" + string(tc.status)
			_, insertErr := env.pool.Exec(`
				INSERT INTO agent_sessions
					(id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
					 status, restart_count, started_at, created_at, updated_at)
				VALUES (?, ?, ?, ?, 'phase', 'agent', ?, 0, ?, ?, ?)`,
				sid, env.project, "TKT-KI-REJ", wfiID, string(tc.status), now, now, now)
			if insertErr != nil {
				t.Fatalf("insert session: %v", insertErr)
			}

			err := env.orch.KillInteractive(sid)
			if err == nil {
				t.Errorf("status=%s: expected error, got nil", tc.status)
				return
			}

			// Confirm the DB row was NOT mutated
			asRepo := repo.NewAgentSessionRepo(env.pool, clock.Real())
			got, getErr := asRepo.Get(sid)
			if getErr != nil {
				t.Fatalf("Get: %v", getErr)
			}
			if got.Status != tc.status {
				t.Errorf("status changed from %q to %q unexpectedly", tc.status, got.Status)
			}
		})
	}
}

// TestKillInteractive_HappyPath verifies that KillInteractive:
// - updates DB to status=failed, result=fail, result_reason=user_killed, ended_at set
// - calls OnClosePtySession with the correct sessionID
// - calls sp.KillInteractive on each registered spawner
// - broadcasts EventAgentKilled to subscribed WS clients
func TestKillInteractive_HappyPath(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TKT-KI-HP", "kill happy path")
	wfiID := env.initWorkflow(t, "TKT-KI-HP")
	sid := "sess-ki-happy"
	insertInteractiveSession(t, env, wfiID, "TKT-KI-HP", sid)

	// Wire OnClosePtySession stub
	var closedPTYSID string
	env.orch.OnClosePtySession = func(sessionID string) {
		closedPTYSID = sessionID
	}

	// Register a real spawner and wire it so KillInteractive is callable
	sp := spawner.New(spawner.Config{Clock: clock.Real()})
	sp.RegisterInteractiveWait(sid) // registers wait channel
	env.orch.mu.Lock()
	env.orch.runs[wfiID] = &runState{
		cancel:   func() {},
		spawners: map[string]*spawner.Spawner{sid: sp},
	}
	env.orch.mu.Unlock()
	t.Cleanup(func() {
		env.orch.mu.Lock()
		delete(env.orch.runs, wfiID)
		env.orch.mu.Unlock()
	})

	// Subscribe a WS client for the project/ticket
	wsCh := env.subscribeWSClient(t, "ws-kill-hp", "TKT-KI-HP")

	err := env.orch.KillInteractive(sid)
	if err != nil {
		t.Fatalf("KillInteractive: %v", err)
	}

	// Verify DB update
	asRepo := repo.NewAgentSessionRepo(env.pool, clock.Real())
	got, err := asRepo.Get(sid)
	if err != nil {
		t.Fatalf("Get session: %v", err)
	}
	if got.Status != model.AgentSessionFailed {
		t.Errorf("status = %q, want %q", got.Status, model.AgentSessionFailed)
	}
	if !got.Result.Valid || got.Result.String != "fail" {
		t.Errorf("result = %v, want {Valid:true, String:\"fail\"}", got.Result)
	}
	if !got.ResultReason.Valid || got.ResultReason.String != "user_killed" {
		t.Errorf("result_reason = %v, want \"user_killed\"", got.ResultReason)
	}
	if !got.EndedAt.Valid || got.EndedAt.String == "" {
		t.Error("ended_at should be set")
	}

	// Verify OnClosePtySession was invoked
	if closedPTYSID != sid {
		t.Errorf("OnClosePtySession called with %q, want %q", closedPTYSID, sid)
	}

	// Verify WS event
	event := expectEvent(t, wsCh, ws.EventAgentKilled, 2*time.Second)
	if event.Type != ws.EventAgentKilled {
		t.Errorf("event type = %q, want %q", event.Type, ws.EventAgentKilled)
	}
	data := event.Data
	if data["session_id"] != sid {
		t.Errorf("event.data.session_id = %v, want %q", data["session_id"], sid)
	}
	if data["reason"] != "user_killed" {
		t.Errorf("event.data.reason = %v, want \"user_killed\"", data["reason"])
	}
}

// TestKillInteractive_BroadcastsAgentKilledEvent verifies the WS event is broadcast
// even when no spawner is registered in o.runs.
func TestKillInteractive_BroadcastsAgentKilledEvent(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TKT-KI-WS", "kill ws event")
	wfiID := env.initWorkflow(t, "TKT-KI-WS")
	sid := "sess-ki-ws"
	insertInteractiveSession(t, env, wfiID, "TKT-KI-WS", sid)

	wsCh := env.subscribeWSClient(t, "ws-kill-ev", "TKT-KI-WS")

	if err := env.orch.KillInteractive(sid); err != nil {
		t.Fatalf("KillInteractive: %v", err)
	}

	event := expectEvent(t, wsCh, ws.EventAgentKilled, 2*time.Second)
	rawData, _ := json.Marshal(event.Data)
	if !strings.Contains(string(rawData), sid) {
		t.Errorf("event data %s should contain session_id %q", rawData, sid)
	}
}

// TestKillInteractive_OtherSessionsUntouched verifies that killing one interactive
// session does not mutate sibling sessions in the same workflow instance.
func TestKillInteractive_OtherSessionsUntouched(t *testing.T) {
	env := newTestEnv(t)
	env.createTicket(t, "TKT-KI-BLR", "blast radius test")
	wfiID := env.initWorkflow(t, "TKT-KI-BLR")

	now := time.Now().UTC().Format(time.RFC3339Nano)
	// Two running sibling sessions
	for _, sid := range []string{"sess-run-a", "sess-run-b"} {
		if _, err := env.pool.Exec(`
			INSERT INTO agent_sessions
				(id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
				 status, restart_count, started_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'phase', 'agent', 'running', 0, ?, ?, ?)`,
			sid, env.project, "TKT-KI-BLR", wfiID, now, now, now); err != nil {
			t.Fatalf("insert running session %s: %v", sid, err)
		}
	}
	// One interactive session to kill
	interactiveSID := "sess-interactive-target"
	insertInteractiveSession(t, env, wfiID, "TKT-KI-BLR", interactiveSID)

	if err := env.orch.KillInteractive(interactiveSID); err != nil {
		t.Fatalf("KillInteractive: %v", err)
	}

	asRepo := repo.NewAgentSessionRepo(env.pool, clock.Real())

	// Siblings must be untouched
	for _, sid := range []string{"sess-run-a", "sess-run-b"} {
		s, err := asRepo.Get(sid)
		if err != nil {
			t.Fatalf("Get(%s): %v", sid, err)
		}
		if s.Status != model.AgentSessionRunning {
			t.Errorf("sibling %s status = %q, want running", sid, s.Status)
		}
		if s.EndedAt.Valid {
			t.Errorf("sibling %s ended_at should not be set", sid)
		}
	}

	// Target must be failed
	target, err := asRepo.Get(interactiveSID)
	if err != nil {
		t.Fatalf("Get interactive: %v", err)
	}
	if target.Status != model.AgentSessionFailed {
		t.Errorf("target status = %q, want failed", target.Status)
	}
}
