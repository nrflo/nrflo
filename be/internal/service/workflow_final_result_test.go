package service

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
)

func TestExtractWorkflowFinalResultByInstanceID_EmptyWhenNoSessions(t *testing.T) {
	t.Parallel()
	pool, _, wfiID := setupDeriveTestEnv(t)

	got := ExtractWorkflowFinalResultByInstanceID(pool, wfiID, clock.Real())
	if got != "" {
		t.Errorf("expected empty string with no sessions, got %q", got)
	}
}

func TestExtractWorkflowFinalResultByInstanceID_EmptyWhenNoMatchingKey(t *testing.T) {
	t.Parallel()
	pool, _, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s1", wfiID, "analyzer", "completed", "pass", "")
	fr := repo.NewFindingRepo(pool, clock.Real())
	raw, _ := json.Marshal("other_value")
	if err := fr.Upsert("session", "s1", "other_key", raw,
		repo.Denorm{WorkflowInstanceID: wfiID, AgentType: "analyzer"},
		repo.Actor{Source: "system"}); err != nil {
		t.Fatalf("upsert finding: %v", err)
	}

	got := ExtractWorkflowFinalResultByInstanceID(pool, wfiID, clock.Real())
	if got != "" {
		t.Errorf("expected empty string when key absent, got %q", got)
	}
}

func TestExtractWorkflowFinalResultByInstanceID_ReturnsValue(t *testing.T) {
	t.Parallel()
	pool, _, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s1", wfiID, "analyzer", "completed", "pass", "")
	endedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(`UPDATE agent_sessions SET ended_at = ? WHERE id = ?`, endedAt, "s1"); err != nil {
		t.Fatalf("update session: %v", err)
	}
	fr := repo.NewFindingRepo(pool, clock.Real())
	raw, _ := json.Marshal("hello world")
	if err := fr.Upsert("session", "s1", "workflow_final_result", raw,
		repo.Denorm{WorkflowInstanceID: wfiID, AgentType: "analyzer"},
		repo.Actor{Source: "system"}); err != nil {
		t.Fatalf("upsert finding: %v", err)
	}

	got := ExtractWorkflowFinalResultByInstanceID(pool, wfiID, clock.Real())
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
	if _, err := pool.Exec(`UPDATE agent_sessions SET ended_at = ? WHERE id = ?`, t1, "s1"); err != nil {
		t.Fatalf("set s1 ended_at: %v", err)
	}
	if _, err := pool.Exec(`UPDATE agent_sessions SET ended_at = ? WHERE id = ?`, t2, "s2"); err != nil {
		t.Fatalf("set s2 ended_at: %v", err)
	}

	fr := repo.NewFindingRepo(pool, clock.Real())
	raw1, _ := json.Marshal("old")
	if err := fr.Upsert("session", "s1", "workflow_final_result", raw1,
		repo.Denorm{WorkflowInstanceID: wfiID, AgentType: "analyzer"},
		repo.Actor{Source: "system"}); err != nil {
		t.Fatalf("upsert s1: %v", err)
	}
	raw2, _ := json.Marshal("new")
	if err := fr.Upsert("session", "s2", "workflow_final_result", raw2,
		repo.Denorm{WorkflowInstanceID: wfiID, AgentType: "builder"},
		repo.Actor{Source: "system"}); err != nil {
		t.Fatalf("upsert s2: %v", err)
	}

	got := ExtractWorkflowFinalResultByInstanceID(pool, wfiID, clock.Real())
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
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, scope_type, status, retry_count, created_at, updated_at)
		 VALUES (?, 'test-proj', '', 'test-wf', 'ticket', 'active', 0, ?, ?)`,
		otherWfiID, now, now); err != nil {
		t.Fatalf("insert other wfi: %v", err)
	}

	insertSession(t, pool, "s-other", otherWfiID, "analyzer", "completed", "pass", "")
	if _, err := pool.Exec(`UPDATE agent_sessions SET ended_at = ? WHERE id = ?`, now, "s-other"); err != nil {
		t.Fatalf("set ended_at: %v", err)
	}
	fr := repo.NewFindingRepo(pool, clock.Real())
	raw, _ := json.Marshal("wrong")
	if err := fr.Upsert("session", "s-other", "workflow_final_result", raw,
		repo.Denorm{WorkflowInstanceID: otherWfiID, AgentType: "analyzer"},
		repo.Actor{Source: "system"}); err != nil {
		t.Fatalf("upsert finding: %v", err)
	}

	got := ExtractWorkflowFinalResultByInstanceID(pool, wfiID, clock.Real())
	if got != "" {
		t.Errorf("expected empty string for original wfiID, got %q", got)
	}
}

func TestExtractWorkflowFinalResult_MethodDelegates(t *testing.T) {
	t.Parallel()
	pool, svc, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s1", wfiID, "analyzer", "completed", "pass", "")
	endedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(`UPDATE agent_sessions SET ended_at = ? WHERE id = ?`, endedAt, "s1"); err != nil {
		t.Fatalf("update session: %v", err)
	}
	fr := repo.NewFindingRepo(pool, clock.Real())
	raw, _ := json.Marshal("delegate works")
	if err := fr.Upsert("session", "s1", "workflow_final_result", raw,
		repo.Denorm{WorkflowInstanceID: wfiID, AgentType: "analyzer"},
		repo.Actor{Source: "system"}); err != nil {
		t.Fatalf("upsert finding: %v", err)
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
	if _, err := pool.Exec(`UPDATE agent_sessions SET ended_at = ? WHERE id = ?`, endedAt, "s1"); err != nil {
		t.Fatalf("update session: %v", err)
	}
	fr := repo.NewFindingRepo(pool, clock.Real())
	raw, _ := json.Marshal("")
	if err := fr.Upsert("session", "s1", "workflow_final_result", raw,
		repo.Denorm{WorkflowInstanceID: wfiID, AgentType: "analyzer"},
		repo.Actor{Source: "system"}); err != nil {
		t.Fatalf("upsert finding: %v", err)
	}

	got := ExtractWorkflowFinalResultByInstanceID(pool, wfiID, clock.Real())
	if got != "" {
		t.Errorf("expected empty string for empty-string value, got %q", got)
	}
}

func TestExtractWorkflowFinalResultByInstanceID_NonStringValueReturnsEmpty(t *testing.T) {
	t.Parallel()
	pool, _, wfiID := setupDeriveTestEnv(t)

	insertSession(t, pool, "s1", wfiID, "analyzer", "completed", "pass", "")
	endedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(`UPDATE agent_sessions SET ended_at = ? WHERE id = ?`, endedAt, "s1"); err != nil {
		t.Fatalf("update session: %v", err)
	}
	fr := repo.NewFindingRepo(pool, clock.Real())
	if err := fr.Upsert("session", "s1", "workflow_final_result", []byte(`42`),
		repo.Denorm{WorkflowInstanceID: wfiID, AgentType: "analyzer"},
		repo.Actor{Source: "system"}); err != nil {
		t.Fatalf("upsert finding: %v", err)
	}

	got := ExtractWorkflowFinalResultByInstanceID(pool, wfiID, clock.Real())
	if got != "" {
		t.Errorf("expected empty string for non-string value, got %q", got)
	}
}
