package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/service"
)

// stubObserverSpawner records calls without spawning a real process.
type stubObserverSpawner struct {
	calls []service.ObserverSpawnRequest
	err   error
}

func (s *stubObserverSpawner) SpawnObserver(req service.ObserverSpawnRequest) error {
	s.calls = append(s.calls, req)
	return s.err
}

// newObserverTestServer builds a Server with a real ObserverService backed by a stub spawner.
// Returns the server, pool, and stub spawner for assertions.
func newObserverTestServer(t *testing.T) (*Server, *db.Pool, *stubObserverSpawner) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "observer_handler_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	clk := clock.Real()
	sp := &stubObserverSpawner{}
	gs := service.NewGlobalSettingsService(pool, clk)
	observerSvc := service.NewObserverService(
		pool, clk, gs,
		service.NewWorkflowService(pool, clk),
		service.NewAgentService(pool, clk),
		service.NewFindingsService(pool, clk),
		service.NewProjectFindingsService(pool, clk),
		service.NewProjectService(pool, clk),
		sp,
	)
	return &Server{pool: pool, clock: clk, observerSvc: observerSvc}, pool, sp
}

// enableObserver sets experimental_observer_enabled=true in the DB.
func enableObserver(t *testing.T, pool *db.Pool) {
	t.Helper()
	gs := service.NewGlobalSettingsService(pool, clock.Real())
	if err := gs.SetExperimentalObserverEnabled(true); err != nil {
		t.Fatalf("SetExperimentalObserverEnabled: %v", err)
	}
}

// launchObserverRequest sends a POST /api/v1/observers request.
func launchObserverRequest(t *testing.T, s *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/observers", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleLaunchObserver(rr, req)
	return rr
}

// listObserversRequest sends a GET /api/v1/observers request with optional project param.
func listObserversRequest(t *testing.T, s *Server, projectID string) *httptest.ResponseRecorder {
	t.Helper()
	url := "/api/v1/observers"
	if projectID != "" {
		url += "?project=" + projectID
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rr := httptest.NewRecorder()
	s.handleListObservers(rr, req)
	return rr
}

// --- POST /api/v1/observers ---

func TestHandleLaunchObserver_FlagOff_Returns404(t *testing.T) {
	t.Parallel()
	s, _, sp := newObserverTestServer(t)

	for _, scope := range []string{"global", "project", "workflow"} {
		t.Run(scope, func(t *testing.T) {
			var body string
			switch scope {
			case "global":
				body = `{"scope":"global"}`
			case "project":
				body = `{"scope":"project","project_id":"p1"}`
			case "workflow":
				body = `{"scope":"workflow","project_id":"p1","workflow_id":"wf1"}`
			}
			rr := launchObserverRequest(t, s, body)
			if rr.Code != http.StatusNotFound {
				t.Errorf("scope=%s: status = %d, want 404 (flag off)", scope, rr.Code)
			}
		})
	}
	if len(sp.calls) != 0 {
		t.Errorf("SpawnObserver called %d times, want 0", len(sp.calls))
	}
}

func TestHandleLaunchObserver_WorkflowScope_MissingIDs_400(t *testing.T) {
	t.Parallel()
	s, pool, _ := newObserverTestServer(t)
	enableObserver(t, pool)

	cases := []struct {
		name string
		body string
	}{
		{"missing_both", `{"scope":"workflow"}`},
		{"missing_workflow_id", `{"scope":"workflow","project_id":"p1"}`},
		{"missing_project_id", `{"scope":"workflow","workflow_id":"wf1"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := launchObserverRequest(t, s, tc.body)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("%s: status = %d, want 400", tc.name, rr.Code)
			}
		})
	}
}

func TestHandleLaunchObserver_ProjectScope_MissingProjectID_400(t *testing.T) {
	t.Parallel()
	s, pool, _ := newObserverTestServer(t)
	enableObserver(t, pool)

	rr := launchObserverRequest(t, s, `{"scope":"project"}`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleLaunchObserver_InvalidScope_400(t *testing.T) {
	t.Parallel()
	s, pool, _ := newObserverTestServer(t)
	enableObserver(t, pool)

	rr := launchObserverRequest(t, s, `{"scope":"invalid"}`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for unknown scope", rr.Code)
	}
}

func TestHandleLaunchObserver_BadBody_400(t *testing.T) {
	t.Parallel()
	s, _, _ := newObserverTestServer(t)

	rr := launchObserverRequest(t, s, `{not json}`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for malformed JSON", rr.Code)
	}
}

func TestHandleLaunchObserver_GlobalScope_Success(t *testing.T) {
	t.Parallel()
	s, pool, sp := newObserverTestServer(t)
	enableObserver(t, pool)

	rr := launchObserverRequest(t, s, `{"scope":"global"}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	sessionID, ok := resp["session_id"]
	if !ok || sessionID == "" {
		t.Errorf("session_id missing or empty in response: %v", resp)
	}

	// Spawner was called once.
	if len(sp.calls) != 1 {
		t.Errorf("SpawnObserver calls = %d, want 1", len(sp.calls))
	}

	// DB row exists with kind=observer and observer_scope=global.
	var kind, scope string
	row := pool.QueryRow(`SELECT kind, observer_scope FROM agent_sessions WHERE id = ?`, sessionID)
	if err := row.Scan(&kind, &scope); err != nil {
		t.Fatalf("scan session row: %v", err)
	}
	if kind != "observer" {
		t.Errorf("kind = %q, want observer", kind)
	}
	if scope != "global" {
		t.Errorf("observer_scope = %q, want global", scope)
	}
}

func TestHandleLaunchObserver_WorkflowScope_Success(t *testing.T) {
	t.Parallel()
	s, pool, sp := newObserverTestServer(t)
	enableObserver(t, pool)

	// Seed project + workflow.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?,?,?,?,?)`,
		"p-obs", "ObsProj", "/tmp", now, now,
	); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := pool.Exec(
		`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES (?,?,?,?,?,?)`,
		"p-obs", "wf-obs", "obs workflow", "project", now, now,
	); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}

	rr := launchObserverRequest(t, s,
		`{"scope":"workflow","project_id":"p-obs","workflow_id":"wf-obs"}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	sessionID := resp["session_id"]

	if len(sp.calls) != 1 {
		t.Errorf("SpawnObserver calls = %d, want 1", len(sp.calls))
	}
	if sp.calls[0].Scope != "workflow" {
		t.Errorf("spawn scope = %q, want workflow", sp.calls[0].Scope)
	}

	var scope string
	pool.QueryRow(`SELECT observer_scope FROM agent_sessions WHERE id = ?`, sessionID).Scan(&scope)
	if scope != "workflow" {
		t.Errorf("observer_scope = %q, want workflow", scope)
	}
}

// --- GET /api/v1/observers ---

func insertObserverSession(t *testing.T, pool *db.Pool, id, projectID, kind, status string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := pool.Exec(`
		INSERT INTO agent_sessions
		(id, project_id, ticket_id, phase, agent_type, model_id, status, kind, observer_scope, started_at, created_at, updated_at)
		VALUES (?, ?, '', 'observer', '_observer', 'sonnet', ?, ?, 'global', ?, ?, ?)`,
		id, projectID, status, kind, now, now, now)
	if err != nil {
		t.Fatalf("insertObserverSession(%s): %v", id, err)
	}
}

func TestHandleListObservers_Empty(t *testing.T) {
	t.Parallel()
	s, _, _ := newObserverTestServer(t)

	rr := listObserversRequest(t, s, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	sessions, _ := resp["sessions"].([]interface{})
	if len(sessions) != 0 {
		t.Errorf("sessions = %v, want empty slice", sessions)
	}
	if count := resp["count"]; count != float64(0) {
		t.Errorf("count = %v, want 0", count)
	}
}

func TestHandleListObservers_FiltersKindAndStatus(t *testing.T) {
	t.Parallel()
	s, pool, _ := newObserverTestServer(t)

	insertObserverSession(t, pool, "obs-running", "proj-x", "observer", "running")
	insertObserverSession(t, pool, "obs-interactive", "proj-x", "observer", "user_interactive")
	insertObserverSession(t, pool, "wf-running", "proj-x", "workflow_agent", "running")     // wrong kind
	insertObserverSession(t, pool, "obs-completed", "proj-x", "observer", "completed")      // wrong status
	insertObserverSession(t, pool, "obs-failed", "proj-x", "observer", "failed")           // wrong status

	rr := listObserversRequest(t, s, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	sessions, _ := resp["sessions"].([]interface{})
	if len(sessions) != 2 {
		t.Errorf("sessions count = %d, want 2 (running + user_interactive observers only)", len(sessions))
	}
	if count := resp["count"]; count != float64(2) {
		t.Errorf("count = %v, want 2", count)
	}
}
