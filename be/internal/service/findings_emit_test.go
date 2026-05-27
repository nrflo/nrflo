package service

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/types"
)

const emitArraySchema = `[{"key":"security_issues","schema":{"type":"array","items":{"type":"object","properties":{"file":{"type":"string"},"severity":{"type":"string"}},"required":["file","severity"]}},"example":[{"file":"a.go","severity":"high"}]}]`

// setupEmitEnv builds a project + workflow (with finding_schemas) + instance +
// agent session, returning the pool, FindingsService, and session id.
func setupEmitEnv(t *testing.T) (*db.Pool, *FindingsService, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "emit_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExec := func(q string, args ...interface{}) {
		if _, err := pool.Exec(q, args...); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	mustExec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES ('p', 'P', '/tmp', ?, ?)`, now, now)
	mustExec(`INSERT INTO workflows (id, project_id, description, scope_type, finding_schemas, created_at, updated_at) VALUES ('wf', 'p', '', 'ticket', ?, ?, ?)`, emitArraySchema, now, now)
	mustExec(`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, retry_count, created_at, updated_at) VALUES ('wfi', 'p', '', 'wf', 'ticket', 'active', 0, ?, ?)`, now, now)
	mustExec(`INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, restart_count, created_at, updated_at) VALUES ('sess', 'p', '', 'wfi', 'analyze', 'analyzer', 'running', 0, ?, ?)`, now, now)

	return pool, NewFindingsService(pool, clock.Real()), "sess"
}

func TestEmit_ValidStored(t *testing.T) {
	t.Parallel()
	pool, svc, sid := setupEmitEnv(t)

	_, err := svc.Emit(&types.FindingsEmitRequest{
		Key:        "security_issues",
		Value:      json.RawMessage(`[{"file":"a.go","severity":"high"}]`),
		SessionID:  sid,
		InstanceID: "wfi",
	})
	if err != nil {
		t.Fatalf("Emit valid: unexpected error: %v", err)
	}

	var raw string
	if err := pool.QueryRow(`SELECT value FROM findings WHERE scope='session' AND scope_id=? AND key='security_issues'`, sid).Scan(&raw); err != nil {
		t.Fatalf("finding not stored: %v", err)
	}
	if !strings.Contains(raw, "a.go") {
		t.Fatalf("stored value missing payload: %s", raw)
	}
}

func TestEmit_InvalidRejectedWithExample(t *testing.T) {
	t.Parallel()
	pool, svc, sid := setupEmitEnv(t)

	_, err := svc.Emit(&types.FindingsEmitRequest{
		Key:        "security_issues",
		Value:      json.RawMessage(`[{"file":"a.go"}]`), // missing required "severity"
		SessionID:  sid,
		InstanceID: "wfi",
	})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "Expected structure example") || !strings.Contains(err.Error(), "security_issues") {
		t.Fatalf("error missing example/key context: %v", err)
	}
	// Nothing should be stored.
	var count int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM findings WHERE scope='session' AND scope_id=?`, sid).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no findings stored on validation failure, got %d", count)
	}
}

func TestEmit_UnknownKeyListsConfigured(t *testing.T) {
	t.Parallel()
	_, svc, sid := setupEmitEnv(t)

	_, err := svc.Emit(&types.FindingsEmitRequest{
		Key:        "notes",
		Value:      json.RawMessage(`[]`),
		SessionID:  sid,
		InstanceID: "wfi",
	})
	if err == nil {
		t.Fatal("expected unknown-key error, got nil")
	}
	if !strings.Contains(err.Error(), "no schema defined") || !strings.Contains(err.Error(), "security_issues") {
		t.Fatalf("error should list configured keys: %v", err)
	}
}
