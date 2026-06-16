package socket

import (
	"database/sql"
	"encoding/json"
	"testing"
)

func callStopHook(t *testing.T, env *handlerTestEnv, sessionID string) Response {
	t.Helper()
	eventRaw, _ := json.Marshal(map[string]interface{}{"hook_event_name": "Stop"})
	paramsData, _ := json.Marshal(map[string]interface{}{
		"event":      json.RawMessage(eventRaw),
		"session_id": sessionID,
	})
	return env.handler.Handle(Request{ID: "stop-req", Method: "agent.record_event", Project: env.project, Params: paramsData})
}

func stopBlocked(t *testing.T, resp Response) (bool, string) {
	t.Helper()
	if resp.Error != nil {
		t.Fatalf("stop hook error: %v", resp.Error)
	}
	var r struct {
		StopDecision *struct {
			Block  bool   `json:"block"`
			Reason string `json:"reason"`
		} `json:"stop_decision"`
	}
	if err := json.Unmarshal(resp.Result, &r); err != nil {
		t.Fatalf("unmarshal stop resp: %v", err)
	}
	if r.StopDecision == nil {
		return false, ""
	}
	return r.StopDecision.Block, r.StopDecision.Reason
}

func insertSessionForStop(t *testing.T, env *handlerTestEnv, ticketID, sessionID, status, result string) {
	t.Helper()
	env.createTicketAndWorkflow(t, ticketID)
	var wfiID string
	if err := env.pool.QueryRow(`SELECT id FROM workflow_instances WHERE LOWER(project_id)=LOWER(?) AND LOWER(ticket_id)=LOWER(?) AND LOWER(workflow_id)=LOWER(?)`,
		env.project, ticketID, "test").Scan(&wfiID); err != nil {
		t.Fatalf("get workflow instance: %v", err)
	}
	if _, err := env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, result, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'impl', 'implementor', 'claude-opus-4-8', ?, ?, datetime('now'), datetime('now'))
	`, sessionID, env.project, ticketID, wfiID, status, sql.NullString{String: result, Valid: result != ""}); err != nil {
		t.Fatalf("insert session: %v", err)
	}
}

// TestStopHook_BlocksThenFails: an autonomous running session that keeps ending
// turns without a completion tool is blocked up to the budget, then failed.
func TestStopHook_BlocksThenFails(t *testing.T) {
	env := newHandlerTestEnv(t)
	insertSessionForStop(t, env, "STOP-1", "sess-stop-1", "running", "")

	for i := 1; i <= 3; i++ {
		block, reason := stopBlocked(t, callStopHook(t, env, "sess-stop-1"))
		if !block {
			t.Fatalf("call %d: expected block, got allow", i)
		}
		if reason == "" {
			t.Errorf("call %d: expected non-empty reason", i)
		}
	}

	// 4th exceeds the budget → no block; session is failed explicitly.
	if block, _ := stopBlocked(t, callStopHook(t, env, "sess-stop-1")); block {
		t.Fatal("4th call: expected no block (budget exceeded)")
	}

	var result, reason string
	var count int
	if err := env.pool.QueryRow(`SELECT result, result_reason, stop_block_count FROM agent_sessions WHERE id=?`, "sess-stop-1").
		Scan(&result, &reason, &count); err != nil {
		t.Fatalf("query session: %v", err)
	}
	if result != "fail" {
		t.Errorf("result = %q, want fail", result)
	}
	if reason != "unresponsive_after_stop_blocks" {
		t.Errorf("result_reason = %q, want unresponsive_after_stop_blocks", reason)
	}
	if count != 4 {
		t.Errorf("stop_block_count = %d, want 4", count)
	}
}

// TestStopHook_AllowsWhenResultSet: a session that already called a completion
// tool is never blocked, and its block count is untouched.
func TestStopHook_AllowsWhenResultSet(t *testing.T) {
	env := newHandlerTestEnv(t)
	insertSessionForStop(t, env, "STOP-2", "sess-stop-2", "running", "pass")

	if block, _ := stopBlocked(t, callStopHook(t, env, "sess-stop-2")); block {
		t.Fatal("expected allow when a completion tool already set a result")
	}
	var count int
	_ = env.pool.QueryRow(`SELECT stop_block_count FROM agent_sessions WHERE id=?`, "sess-stop-2").Scan(&count)
	if count != 0 {
		t.Errorf("stop_block_count = %d, want 0 (no increment on allow)", count)
	}
}

// TestStopHook_AllowsWhenNotAutonomous: interactive/plan/take-control sessions
// (status != running) must never be blocked.
func TestStopHook_AllowsWhenNotAutonomous(t *testing.T) {
	env := newHandlerTestEnv(t)
	insertSessionForStop(t, env, "STOP-3", "sess-stop-3", "user_interactive", "")

	if block, _ := stopBlocked(t, callStopHook(t, env, "sess-stop-3")); block {
		t.Fatal("expected allow for a non-autonomous (user_interactive) session")
	}
	var count int
	_ = env.pool.QueryRow(`SELECT stop_block_count FROM agent_sessions WHERE id=?`, "sess-stop-3").Scan(&count)
	if count != 0 {
		t.Errorf("stop_block_count = %d, want 0", count)
	}
}
