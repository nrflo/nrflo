package service

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

// setupFindingsReservedTestEnv creates an isolated DB with a project, workflow,
// workflow instance, and a running agent session. Returns the pool, a
// FindingsService, and the session ID findings should be attributed to.
func setupFindingsReservedTestEnv(t *testing.T) (*db.Pool, *FindingsService, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "findings_reserved_test.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'P', '/tmp', ?, ?)`,
		"proj-fr", now, now); err != nil {
		t.Fatalf("project insert: %v", err)
	}

	wfSvc := NewWorkflowService(pool, clock.Real())
	if _, err := wfSvc.CreateWorkflowDef("proj-fr", &types.WorkflowDefCreateRequest{ID: "wf-fr"}); err != nil {
		t.Fatalf("workflow create: %v", err)
	}

	wfiRepo := repo.NewWorkflowInstanceRepo(pool, clock.Real())
	wfi := &model.WorkflowInstance{
		ID:         "wfi-fr",
		ProjectID:  "proj-fr",
		TicketID:   "",
		WorkflowID: "wf-fr",
		ScopeType:  "project",
		Status:     model.WorkflowInstanceActive,
	}
	if err := wfiRepo.Create(wfi); err != nil {
		t.Fatalf("workflow instance create: %v", err)
	}

	asRepo := repo.NewAgentSessionRepo(pool, clock.Real())
	sessionID := "sess-fr"
	if err := asRepo.Create(&model.AgentSession{
		ID:                 sessionID,
		ProjectID:          "proj-fr",
		TicketID:           "",
		WorkflowInstanceID: wfi.ID,
		Phase:              "implementor",
		AgentType:          "implementor",
		Status:             model.AgentSessionRunning,
	}); err != nil {
		t.Fatalf("agent session create: %v", err)
	}

	svc := NewFindingsService(pool, clock.Real())
	return pool, svc, sessionID
}

// countFindingsForKey returns the number of rows in the findings table for key.
func countFindingsForKey(t *testing.T, pool *db.Pool, key string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM findings WHERE key = ?`, key).Scan(&count); err != nil {
		t.Fatalf("count findings for key %q: %v", key, err)
	}
	return count
}

// assertReservedRejection asserts err is non-nil, names emit_findings, and
// that nothing was persisted to the findings table for WorkflowPlanFindingKey.
func assertReservedRejection(t *testing.T, pool *db.Pool, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error for reserved key, got nil")
	}
	if !strings.Contains(err.Error(), "emit_findings") {
		t.Errorf("error = %q, want it to mention emit_findings", err.Error())
	}
	if got := countFindingsForKey(t, pool, WorkflowPlanFindingKey); got != 0 {
		t.Errorf("findings rows for %q = %d, want 0 (rejected write must not persist)", WorkflowPlanFindingKey, got)
	}
}

// --- Add ---

func TestFindingsAdd_RejectsReservedKey(t *testing.T) {
	t.Parallel()
	pool, svc, sessionID := setupFindingsReservedTestEnv(t)

	_, err := svc.Add(&types.FindingsAddRequest{
		SessionID: sessionID,
		Key:       WorkflowPlanFindingKey,
		Value:     `{"version":1}`,
	})
	assertReservedRejection(t, pool, err)
}

// TestFindingsAdd_AllowsConsultAnswer is the regression guard: _consult_answer
// starts with "_" like the reserved key but is NOT in the reserved registry
// (IsReservedFindingKey is a lookup, not a prefix ban) and must round-trip.
func TestFindingsAdd_AllowsConsultAnswer(t *testing.T) {
	t.Parallel()
	pool, svc, sessionID := setupFindingsReservedTestEnv(t)

	if _, err := svc.Add(&types.FindingsAddRequest{
		SessionID: sessionID,
		Key:       "_consult_answer",
		Value:     "the answer",
	}); err != nil {
		t.Fatalf("Add(_consult_answer) unexpectedly rejected: %v", err)
	}

	findingRepo := repo.NewFindingRepo(pool, clock.Real())
	raw, err := findingRepo.GetOwn("session", sessionID)
	if err != nil {
		t.Fatalf("GetOwn: %v", err)
	}
	val, ok := raw["_consult_answer"]
	if !ok {
		t.Fatal("_consult_answer not found after Add")
	}
	if string(val) != `"the answer"` {
		t.Errorf("_consult_answer value = %s, want %q", val, `"the answer"`)
	}
}

func TestFindingsAdd_AllowsNormalKey(t *testing.T) {
	t.Parallel()
	pool, svc, sessionID := setupFindingsReservedTestEnv(t)

	if _, err := svc.Add(&types.FindingsAddRequest{
		SessionID: sessionID,
		Key:       "root_cause",
		Value:     "off-by-one",
	}); err != nil {
		t.Fatalf("Add(normal key) unexpectedly rejected: %v", err)
	}

	findingRepo := repo.NewFindingRepo(pool, clock.Real())
	raw, err := findingRepo.GetOwn("session", sessionID)
	if err != nil {
		t.Fatalf("GetOwn: %v", err)
	}
	if _, ok := raw["root_cause"]; !ok {
		t.Fatal("root_cause not found after Add")
	}
}

// --- AddBulk ---

func TestFindingsAddBulk_RejectsReservedKey(t *testing.T) {
	t.Parallel()
	pool, svc, sessionID := setupFindingsReservedTestEnv(t)

	_, err := svc.AddBulk(&types.FindingsAddBulkRequest{
		SessionID: sessionID,
		KeyValues: map[string]string{
			WorkflowPlanFindingKey: `{"version":1}`,
			"other_key":            "value",
		},
	})
	assertReservedRejection(t, pool, err)
}

func TestFindingsAddBulk_AllowsConsultAnswer(t *testing.T) {
	t.Parallel()
	pool, svc, sessionID := setupFindingsReservedTestEnv(t)

	if _, err := svc.AddBulk(&types.FindingsAddBulkRequest{
		SessionID: sessionID,
		KeyValues: map[string]string{
			"_consult_answer": "bulk answer",
		},
	}); err != nil {
		t.Fatalf("AddBulk(_consult_answer) unexpectedly rejected: %v", err)
	}

	findingRepo := repo.NewFindingRepo(pool, clock.Real())
	raw, err := findingRepo.GetOwn("session", sessionID)
	if err != nil {
		t.Fatalf("GetOwn: %v", err)
	}
	if val, ok := raw["_consult_answer"]; !ok || string(val) != `"bulk answer"` {
		t.Errorf("_consult_answer = %s (ok=%v), want %q", val, ok, `"bulk answer"`)
	}
}

func TestFindingsAddBulk_AllowsNormalKey(t *testing.T) {
	t.Parallel()
	_, svc, sessionID := setupFindingsReservedTestEnv(t)

	if _, err := svc.AddBulk(&types.FindingsAddBulkRequest{
		SessionID: sessionID,
		KeyValues: map[string]string{"k1": "v1"},
	}); err != nil {
		t.Fatalf("AddBulk(normal key) unexpectedly rejected: %v", err)
	}
}
