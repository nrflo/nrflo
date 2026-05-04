package service

import (
	"testing"
	"time"

	"be/internal/model"
)

func TestExtractWorkflowFinalResultByInstanceID_EmptyWhenNoSessions(t *testing.T) {
	t.Parallel()
	pool, _, wfiID := setupDeriveTestEnv(t)

	got := ExtractWorkflowFinalResultByInstanceID(pool, wfiID)
	if got != "" {
		t.Errorf("expected empty string with no sessions, got %q", got)
	}
}

func TestExtractWorkflowFinalResultByInstanceID_EmptyWhenNoMatchingKey(t *testing.T) {
	t.Parallel()
	pool, _, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s1", wfiID, "analyzer", "completed", "pass", "")
	if _, err := pool.Exec(`UPDATE agent_sessions SET findings = ? WHERE id = ?`,
		`{"other_key":"other_value"}`, "s1"); err != nil {
		t.Fatalf("update findings: %v", err)
	}

	got := ExtractWorkflowFinalResultByInstanceID(pool, wfiID)
	if got != "" {
		t.Errorf("expected empty string when key absent, got %q", got)
	}
}

func TestExtractWorkflowFinalResultByInstanceID_ReturnsValue(t *testing.T) {
	t.Parallel()
	pool, _, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s1", wfiID, "analyzer", "completed", "pass", "")
	endedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(`UPDATE agent_sessions SET findings = ?, ended_at = ? WHERE id = ?`,
		`{"workflow_final_result":"hello world"}`, endedAt, "s1"); err != nil {
		t.Fatalf("update session: %v", err)
	}

	got := ExtractWorkflowFinalResultByInstanceID(pool, wfiID)
	if got != "hello world" {
		t.Errorf("expected %q, got %q", "hello world", got)
	}
}

func TestExtractWorkflowFinalResultByInstanceID_PrioritizesLatestEndedAt(t *testing.T) {
	t.Parallel()
	pool, _, wfiID := setupDeriveTestEnv(t)

	t1 := "2025-01-01T00:00:00Z"
	t2 := "2025-01-01T00:00:01Z"

	insertSession(t, pool, "s1", wfiID, "analyzer", "completed", "pass", t1)
	insertSession(t, pool, "s2", wfiID, "builder", "completed", "pass", t2)

	if _, err := pool.Exec(`UPDATE agent_sessions SET findings = ?, ended_at = ? WHERE id = ?`,
		`{"workflow_final_result":"old"}`, t1, "s1"); err != nil {
		t.Fatalf("update s1: %v", err)
	}
	if _, err := pool.Exec(`UPDATE agent_sessions SET findings = ?, ended_at = ? WHERE id = ?`,
		`{"workflow_final_result":"new"}`, t2, "s2"); err != nil {
		t.Fatalf("update s2: %v", err)
	}

	got := ExtractWorkflowFinalResultByInstanceID(pool, wfiID)
	if got != "new" {
		t.Errorf("expected %q (latest ended_at wins), got %q", "new", got)
	}
}

func TestExtractWorkflowFinalResultByInstanceID_IgnoresOtherInstances(t *testing.T) {
	t.Parallel()
	pool, _, wfiID := setupDeriveTestEnv(t)

	// Create a second workflow instance to attach the session to.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	otherWfiID := "wfi-other-instance"
	if _, err := pool.Exec(
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, findings, retry_count, created_at, updated_at)
		 VALUES (?, 'test-proj', '', 'test-wf', 'ticket', 'active', '{}', 0, ?, ?)`,
		otherWfiID, now, now); err != nil {
		t.Fatalf("insert other wfi: %v", err)
	}

	insertSession(t, pool, "s-other", otherWfiID, "analyzer", "completed", "pass", "")
	if _, err := pool.Exec(`UPDATE agent_sessions SET findings = ?, ended_at = ? WHERE id = ?`,
		`{"workflow_final_result":"wrong"}`, now, "s-other"); err != nil {
		t.Fatalf("update s-other: %v", err)
	}

	got := ExtractWorkflowFinalResultByInstanceID(pool, wfiID)
	if got != "" {
		t.Errorf("expected empty string for original wfiID, got %q", got)
	}
}

func TestExtractWorkflowFinalResult_MethodDelegates(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s1", wfiID, "analyzer", "completed", "pass", "")
	endedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(`UPDATE agent_sessions SET findings = ?, ended_at = ? WHERE id = ?`,
		`{"workflow_final_result":"delegate works"}`, endedAt, "s1"); err != nil {
		t.Fatalf("update session: %v", err)
	}

	got := svc.ExtractWorkflowFinalResult(&model.WorkflowInstance{ID: wfiID})
	if got != "delegate works" {
		t.Errorf("expected %q, got %q", "delegate works", got)
	}
}

func TestExtractWorkflowFinalResultByInstanceID_EmptyStringValueReturnsEmpty(t *testing.T) {
	t.Parallel()
	pool, _, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s1", wfiID, "analyzer", "completed", "pass", "")
	endedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(`UPDATE agent_sessions SET findings = ?, ended_at = ? WHERE id = ?`,
		`{"workflow_final_result":""}`, endedAt, "s1"); err != nil {
		t.Fatalf("update session: %v", err)
	}

	got := ExtractWorkflowFinalResultByInstanceID(pool, wfiID)
	if got != "" {
		t.Errorf("expected empty string for empty-string value, got %q", got)
	}
}

func TestExtractWorkflowFinalResultByInstanceID_NonStringValueReturnsEmpty(t *testing.T) {
	t.Parallel()
	pool, _, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s1", wfiID, "analyzer", "completed", "pass", "")
	endedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(`UPDATE agent_sessions SET findings = ?, ended_at = ? WHERE id = ?`,
		`{"workflow_final_result":42}`, endedAt, "s1"); err != nil {
		t.Fatalf("update session: %v", err)
	}

	got := ExtractWorkflowFinalResultByInstanceID(pool, wfiID)
	if got != "" {
		t.Errorf("expected empty string for non-string value, got %q", got)
	}
}
