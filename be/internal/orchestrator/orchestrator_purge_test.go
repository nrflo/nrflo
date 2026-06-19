package orchestrator

import (
	"database/sql"
	"testing"
	"time"
)

// TestMarkFailedPurgesTraceWhenEnabled covers the full wiring: the workflow def's
// purge_on_completion flag is snapshotted onto the instance at init, and the markFailed
// terminal hook then redacts sessions and deletes messages.
func TestMarkFailedPurgesTraceWhenEnabled(t *testing.T) {
	env := newTestEnv(t)

	if _, err := env.pool.Exec(
		`UPDATE workflows SET purge_on_completion = 1 WHERE LOWER(project_id)=LOWER(?) AND LOWER(id)=LOWER(?)`,
		env.project, "test"); err != nil {
		t.Fatalf("enable purge on def: %v", err)
	}
	wfiID := env.initProjectWorkflow(t, "test")

	if !env.getWorkflowInstance(t, wfiID).PurgeOnCompletion {
		t.Fatal("instance did not snapshot purge_on_completion from the workflow def")
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, prompt, config, created_at, updated_at)
		VALUES ('s1', ?, '', ?, 'p', 'implementor', 'failed', 'SENSITIVE PROMPT', '', ?, ?)`,
		env.project, wfiID, now, now); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if _, err := env.pool.Exec(`INSERT INTO agent_messages (session_id, seq, content, created_at) VALUES ('s1',1,'secret',?)`, now); err != nil {
		t.Fatalf("seed message: %v", err)
	}

	env.orch.markFailed(wfiID, RunRequest{ProjectID: env.project, WorkflowName: "test", ScopeType: "project"}, "boom")

	var prompt sql.NullString
	if err := env.pool.QueryRow(`SELECT prompt FROM agent_sessions WHERE id='s1'`).Scan(&prompt); err != nil {
		t.Fatalf("read session: %v", err)
	}
	if prompt.Valid {
		t.Errorf("prompt not redacted after purge: %q", prompt.String)
	}
	var msgs int
	if err := env.pool.QueryRow(`SELECT COUNT(*) FROM agent_messages WHERE session_id='s1'`).Scan(&msgs); err != nil {
		t.Fatal(err)
	}
	if msgs != 0 {
		t.Errorf("messages not deleted after purge: %d", msgs)
	}
}

// TestMarkFailedNoPurgeWhenDisabled verifies the hook is a no-op when the flag is off.
func TestMarkFailedNoPurgeWhenDisabled(t *testing.T) {
	env := newTestEnv(t)
	wfiID := env.initProjectWorkflow(t, "test") // purge flag off by default

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, prompt, config, created_at, updated_at)
		VALUES ('s1', ?, '', ?, 'p', 'implementor', 'failed', 'SENSITIVE PROMPT', '', ?, ?)`,
		env.project, wfiID, now, now); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	env.orch.markFailed(wfiID, RunRequest{ProjectID: env.project, WorkflowName: "test", ScopeType: "project"}, "boom")

	var prompt sql.NullString
	if err := env.pool.QueryRow(`SELECT prompt FROM agent_sessions WHERE id='s1'`).Scan(&prompt); err != nil {
		t.Fatalf("read session: %v", err)
	}
	if !prompt.Valid || prompt.String != "SENSITIVE PROMPT" {
		t.Errorf("prompt should be untouched when purge disabled, got %v", prompt)
	}
}
