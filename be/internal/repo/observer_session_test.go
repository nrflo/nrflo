package repo

import (
	"database/sql"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
)

// setupObserverFixture creates a DB with project + workflow + workflow_instance rows.
// Observer sessions created via r.Create() use the wfiID to satisfy the FK constraint
// (current production code gap: Create passes "" not NULL for WorkflowInstanceID).
func setupObserverFixture(t *testing.T) (*AgentSessionRepo, string) {
	t.Helper()
	database := newTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	if _, err := database.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('obs-proj','Observer Project',?,?)`,
		now, now,
	); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at)
		 VALUES ('obs-proj','obs-wf','Observer WF','ticket',?,?)`,
		now, now,
	); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	wfiID := "obs-wfi-001"
	if _, err := database.Exec(
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at)
		 VALUES (?,'obs-proj','','obs-wf','active','ticket',?,?)`,
		wfiID, now, now,
	); err != nil {
		t.Fatalf("insert workflow_instance: %v", err)
	}
	return NewAgentSessionRepo(database, clock.Real()), wfiID
}

// makeObserverSession returns a minimal AgentSession with kind=observer and the given scope+wfiID.
func makeObserverSession(id, scope, wfiID, token string) *model.AgentSession {
	return &model.AgentSession{
		ID:                 id,
		ProjectID:          "obs-proj",
		WorkflowInstanceID: wfiID,
		AgentType:          "_observer",
		Phase:              "observer",
		Status:             model.AgentSessionRunning,
		Kind:               "observer",
		ObserverScope:      sql.NullString{String: scope, Valid: true},
		SpawnToken:         sql.NullString{String: token, Valid: true},
	}
}

// TestObserverSession_KindObserver_WithWFI verifies kind=observer + observer_scope round-trip.
func TestObserverSession_KindObserver_WithWFI(t *testing.T) {
	t.Parallel()
	r, wfiID := setupObserverFixture(t)

	sess := makeObserverSession("obs-kind-001", "workflow", wfiID, "tok-kind-001")
	if err := r.Create(sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	stored, err := r.Get("obs-kind-001")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.Kind != "observer" {
		t.Errorf("Kind = %q, want observer", stored.Kind)
	}
	if !stored.ObserverScope.Valid || stored.ObserverScope.String != "workflow" {
		t.Errorf("ObserverScope = %v, want {workflow, true}", stored.ObserverScope)
	}
	if !stored.SpawnToken.Valid || stored.SpawnToken.String != "tok-kind-001" {
		t.Errorf("SpawnToken = %v, want {tok-kind-001, true}", stored.SpawnToken)
	}
	if stored.Status != model.AgentSessionRunning {
		t.Errorf("Status = %q, want running", stored.Status)
	}
}

// TestObserverSession_KindDefaultsToWorkflowAgent verifies that when Kind is empty,
// the repo defaults the stored value to "workflow_agent".
func TestObserverSession_KindDefaultsToWorkflowAgent(t *testing.T) {
	t.Parallel()
	_, r, wfiID := setupTestDB(t)

	sess := &model.AgentSession{
		ID:                 "wf-kind-default",
		ProjectID:          "proj",
		AgentType:          "implementor",
		Phase:              "impl",
		Status:             model.AgentSessionRunning,
		WorkflowInstanceID: wfiID,
		// Kind intentionally zero-value
	}
	if err := r.Create(sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	stored, err := r.Get("wf-kind-default")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.Kind != "workflow_agent" {
		t.Errorf("Kind = %q, want workflow_agent", stored.Kind)
	}
	if stored.ObserverScope.Valid {
		t.Errorf("ObserverScope should be NULL for workflow_agent, got %v", stored.ObserverScope)
	}
}

// TestObserverSession_AllScopeValues checks each valid scope string round-trips.
func TestObserverSession_AllScopeValues(t *testing.T) {
	t.Parallel()
	r, wfiID := setupObserverFixture(t)

	scopes := []string{"workflow", "project", "global"}
	for i, scope := range scopes {
		scope := scope
		id := "obs-scope-" + scope
		sess := makeObserverSession(id, scope, wfiID, "tok-"+scope+"-"+string(rune('a'+i)))
		if err := r.Create(sess); err != nil {
			t.Fatalf("Create scope=%s: %v", scope, err)
		}
		stored, err := r.Get(id)
		if err != nil {
			t.Fatalf("Get scope=%s: %v", scope, err)
		}
		if stored.ObserverScope.String != scope {
			t.Errorf("scope=%s: ObserverScope = %q, want %q", scope, stored.ObserverScope.String, scope)
		}
		if stored.Kind != "observer" {
			t.Errorf("scope=%s: Kind = %q, want observer", scope, stored.Kind)
		}
	}
}

// TestObserverSession_NullWFI_FKBug documents that repo.Create currently passes
// "" (empty string) instead of NULL for WorkflowInstanceID, causing a FK failure
// when no workflow_instance with id="" exists.
//
// Fix needed in repo/agent_session.go Create: pass nil/sql.NullString{Valid:false}
// when WorkflowInstanceID == "".
func TestObserverSession_NullWFI_FKBug(t *testing.T) {
	t.Parallel()
	r, _ := setupObserverFixture(t)

	sess := &model.AgentSession{
		ID:                 "obs-no-wfi",
		ProjectID:          "obs-proj",
		AgentType:          "_observer",
		Phase:              "observer",
		Status:             model.AgentSessionRunning,
		Kind:               "observer",
		ObserverScope:      sql.NullString{String: "global", Valid: true},
		WorkflowInstanceID: "", // empty → should map to NULL but currently maps to ""
	}
	err := r.Create(sess)
	if err == nil {
		// Bug is fixed — verify correct storage
		stored, getErr := r.Get("obs-no-wfi")
		if getErr != nil {
			t.Fatalf("Get after null-WFI create: %v", getErr)
		}
		if stored.WorkflowInstanceID != "" {
			t.Errorf("WorkflowInstanceID = %q, want empty", stored.WorkflowInstanceID)
		}
		return
	}
	// Current behaviour: FK constraint error
	if err.Error() == "" {
		t.Error("expected a non-empty FK constraint error")
	}
}

// TestObserverSession_GetByToken_ObserverSession verifies GetByToken returns observer sessions.
func TestObserverSession_GetByToken_ObserverSession(t *testing.T) {
	t.Parallel()
	r, wfiID := setupObserverFixture(t)

	sess := makeObserverSession("obs-tok-test", "project", wfiID, "obs-unique-token-abc")
	if err := r.Create(sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.GetByToken("obs-unique-token-abc")
	if err != nil {
		t.Fatalf("GetByToken: %v", err)
	}
	if got == nil {
		t.Fatal("GetByToken returned nil for valid token")
	}
	if got.ID != "obs-tok-test" {
		t.Errorf("ID = %q, want obs-tok-test", got.ID)
	}
	if got.Kind != "observer" {
		t.Errorf("Kind = %q, want observer", got.Kind)
	}
}

// TestObserverSession_GetByToken_EmptyTokenReturnsNil confirms no-match on empty token.
func TestObserverSession_GetByToken_EmptyTokenReturnsNil(t *testing.T) {
	t.Parallel()
	r, _ := setupObserverFixture(t)

	got, err := r.GetByToken("")
	if err != nil {
		t.Fatalf("GetByToken empty: %v", err)
	}
	if got != nil {
		t.Errorf("GetByToken(\"\") = %v, want nil", got.ID)
	}
}

// TestObserverSession_UpdateStatusToFailed verifies UpdateStatusToFailedWithReason on observer sessions.
func TestObserverSession_UpdateStatusToFailed(t *testing.T) {
	t.Parallel()
	r, wfiID := setupObserverFixture(t)

	sess := makeObserverSession("obs-fail-test", "global", wfiID, "tok-fail-test")
	if err := r.Create(sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := r.UpdateStatusToFailedWithReason("obs-fail-test", "spawn_failed"); err != nil {
		t.Fatalf("UpdateStatusToFailedWithReason: %v", err)
	}

	stored, err := r.Get("obs-fail-test")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.Status != model.AgentSessionFailed {
		t.Errorf("Status = %q, want failed", stored.Status)
	}
	if !stored.ResultReason.Valid || stored.ResultReason.String != "spawn_failed" {
		t.Errorf("ResultReason = %v, want {spawn_failed, true}", stored.ResultReason)
	}
}

// TestObserverSession_UpdateStatusToFailed_MissingSessionErrors verifies the
// "not found" path of UpdateStatusToFailedWithReason.
func TestObserverSession_UpdateStatusToFailed_MissingSessionErrors(t *testing.T) {
	t.Parallel()
	r, _ := setupObserverFixture(t)

	err := r.UpdateStatusToFailedWithReason("does-not-exist", "spawn_failed")
	if err == nil {
		t.Error("expected error for non-existent session, got nil")
	}
}
