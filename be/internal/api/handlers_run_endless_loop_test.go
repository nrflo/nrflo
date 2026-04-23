package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/orchestrator"
	"be/internal/ws"
)

// newEndlessLoopRunServer creates a Server with a real orchestrator for testing the
// project-scoped run handler's endless_loop validation branches.
func newEndlessLoopRunServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "run_endless_loop_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	hub := ws.NewHub(clock.Real())
	go hub.Run()
	t.Cleanup(hub.Stop)

	orch := orchestrator.New(dbPath, hub, clock.Real(), nil)
	return &Server{orchestrator: orch, clock: clock.Real(), pool: pool, wsHub: hub}
}

// seedWorkflowDef inserts a project + workflow row with the given scope_type
// so that s.workflowService().GetWorkflowDef() resolves successfully.
func seedWorkflowDef(t *testing.T, s *Server, projectID, workflowID, scopeType string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.pool.Exec(
		`INSERT OR IGNORE INTO projects (id, name, root_path, created_at, updated_at)
			VALUES (?, 'Test', '/tmp', ?, ?)`,
		projectID, now, now,
	); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := s.pool.Exec(
		`INSERT OR IGNORE INTO workflows (id, project_id, description, scope_type, groups, created_at, updated_at)
			VALUES (?, ?, '', ?, '[]', ?, ?)`,
		workflowID, projectID, scopeType, now, now,
	); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
}

// TestHandleRunProjectWorkflow_EndlessLoopWithInteractive verifies 400 when
// endless_loop is combined with interactive=true.
func TestHandleRunProjectWorkflow_EndlessLoopWithInteractive(t *testing.T) {
	s := newEndlessLoopRunServer(t)
	body := `{"workflow":"wf","endless_loop":true,"interactive":true}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/projects/proj-1/workflow/run", strings.NewReader(body))
	req.SetPathValue("id", "proj-1")
	rr := httptest.NewRecorder()
	s.handleRunProjectWorkflow(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "endless_loop")
}

// TestHandleRunProjectWorkflow_EndlessLoopWithPlanMode verifies 400 when
// endless_loop is combined with plan_mode=true.
func TestHandleRunProjectWorkflow_EndlessLoopWithPlanMode(t *testing.T) {
	s := newEndlessLoopRunServer(t)
	body := `{"workflow":"wf","endless_loop":true,"plan_mode":true}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/projects/proj-1/workflow/run", strings.NewReader(body))
	req.SetPathValue("id", "proj-1")
	rr := httptest.NewRecorder()
	s.handleRunProjectWorkflow(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "endless_loop")
}

// TestHandleRunProjectWorkflow_EndlessLoopWithBothInteractiveAndPlanMode
// verifies the mutual-exclusivity check fires before the endless_loop check.
// (interactive && plan_mode returns the "mutually exclusive" error first.)
func TestHandleRunProjectWorkflow_EndlessLoopWithBothInteractiveAndPlanMode(t *testing.T) {
	s := newEndlessLoopRunServer(t)
	body := `{"workflow":"wf","endless_loop":true,"interactive":true,"plan_mode":true}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/projects/proj-1/workflow/run", strings.NewReader(body))
	req.SetPathValue("id", "proj-1")
	rr := httptest.NewRecorder()
	s.handleRunProjectWorkflow(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "mutually exclusive")
}

// TestHandleRunProjectWorkflow_EndlessLoopWithTicketScopedWorkflow verifies 400
// when endless_loop is requested for a ticket-scoped workflow definition.
func TestHandleRunProjectWorkflow_EndlessLoopWithTicketScopedWorkflow(t *testing.T) {
	s := newEndlessLoopRunServer(t)
	seedWorkflowDef(t, s, "proj-tkt", "feature-ticket", "ticket")

	body := `{"workflow":"feature-ticket","endless_loop":true}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/projects/proj-tkt/workflow/run", strings.NewReader(body))
	req.SetPathValue("id", "proj-tkt")
	rr := httptest.NewRecorder()
	s.handleRunProjectWorkflow(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "project-scoped")
}

// TestHandleRunProjectWorkflow_EndlessLoopWithMissingWorkflowDef verifies 400
// (bubbling GetWorkflowDef's "workflow not found" error) when endless_loop=true
// is requested for an unknown workflow name.
func TestHandleRunProjectWorkflow_EndlessLoopWithMissingWorkflowDef(t *testing.T) {
	s := newEndlessLoopRunServer(t)

	body := `{"workflow":"does-not-exist","endless_loop":true}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/projects/proj-nf/workflow/run", strings.NewReader(body))
	req.SetPathValue("id", "proj-nf")
	rr := httptest.NewRecorder()
	s.handleRunProjectWorkflow(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "not found")
}

// TestHandleRunProjectWorkflow_EndlessLoopFalse_PassesValidation verifies that
// endless_loop=false (or omitted) bypasses the endless_loop validation entirely.
// The request still fails downstream (missing project row), but NOT with any of
// the endless_loop-specific error strings.
func TestHandleRunProjectWorkflow_EndlessLoopFalse_PassesValidation(t *testing.T) {
	s := newEndlessLoopRunServer(t)

	body := `{"workflow":"wf","endless_loop":false,"interactive":true}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/projects/proj-x/workflow/run", strings.NewReader(body))
	req.SetPathValue("id", "proj-x")
	rr := httptest.NewRecorder()
	s.handleRunProjectWorkflow(rr, req)

	// Whatever the status, the error body must not contain endless_loop validation strings.
	errBody := rr.Body.String()
	if strings.Contains(errBody, "endless_loop") ||
		strings.Contains(errBody, "project-scoped") {
		t.Errorf("endless_loop=false incorrectly triggered endless_loop validation; body: %s", errBody)
	}
}
