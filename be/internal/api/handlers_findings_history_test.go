package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
)

// histResp is the decoded JSON payload from handleListFindingsHistory.
type histResp struct {
	Items  []map[string]interface{} `json:"items"`
	Limit  float64                  `json:"limit"`
	Offset float64                  `json:"offset"`
}

func newHistServer(t *testing.T) *Server {
	t.Helper()
	return newProjectFindingsServer(t)
}

func newHistServerWithClock(t *testing.T) (*Server, *clock.TestClock) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "hist_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	return &Server{pool: pool, clock: clk}, clk
}

func histReq(t *testing.T, query string) *http.Request {
	t.Helper()
	return httptest.NewRequest(http.MethodGet, "/api/v1/findings/history?"+query, nil)
}

func decodeHistResp(t *testing.T, rr *httptest.ResponseRecorder) histResp {
	t.Helper()
	var resp histResp
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode hist response: %v", err)
	}
	return resp
}

// seedWFIForHistory inserts project + workflow + workflow_instance rows.
func seedWFIForHistory(t *testing.T, s *Server, wfiID, projectID string) {
	t.Helper()
	seedProjectForFindings(t, s, projectID)
	wfID := "wf-" + wfiID
	now := "2026-01-01T00:00:00Z"
	for _, stmt := range []struct {
		sql  string
		args []interface{}
	}{
		{`INSERT OR IGNORE INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES (?, ?, '', 'project', ?, ?)`, []interface{}{projectID, wfID, now, now}},
		{`INSERT OR IGNORE INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at) VALUES (?, ?, '', ?, 'completed', 'project', ?, ?)`, []interface{}{wfiID, projectID, wfID, now, now}},
	} {
		if _, err := s.pool.Exec(stmt.sql, stmt.args...); err != nil {
			t.Fatalf("seedWFIForHistory %q: %v", wfiID, err)
		}
	}
}

// seedSessionForHistory inserts project + workflow + wfi + agent_session rows.
func seedSessionForHistory(t *testing.T, s *Server, sessionID, projectID string) {
	t.Helper()
	wfiID := sessionID + "-wfi"
	seedWFIForHistory(t, s, wfiID, projectID)
	now := "2026-01-01T00:00:00Z"
	_, err := s.pool.Exec(
		`INSERT OR IGNORE INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, created_at, updated_at) VALUES (?, ?, '', ?, 'ph', 'ag', 'completed', ?, ?)`,
		sessionID, projectID, wfiID, now, now,
	)
	if err != nil {
		t.Fatalf("seedSessionForHistory %q: %v", sessionID, err)
	}
}

// upsertHistFinding wraps FindingRepo.Upsert for test setup.
func upsertHistFinding(t *testing.T, s *Server, scope, scopeID, key string) {
	t.Helper()
	r := repo.NewFindingRepo(s.pool, s.clock)
	if err := r.Upsert(scope, scopeID, key, []byte(`"v"`), repo.Denorm{}, repo.Actor{Source: "system"}); err != nil {
		t.Fatalf("upsertHistFinding %s/%s/%s: %v", scope, scopeID, key, err)
	}
}

// TestHandleListFindingsHistory_ValidationErrors covers all 400 cases.
func TestHandleListFindingsHistory_ValidationErrors(t *testing.T) {
	s := newHistServer(t)

	cases := []struct {
		name  string
		query string
		want  string
	}{
		{"missing scope", "scope_id=x", "scope is required"},
		{"invalid scope ticket", "scope=ticket&scope_id=x", "invalid scope"},
		{"invalid scope workflow", "scope=workflow&scope_id=x", "invalid scope"},
		{"missing scope_id", "scope=project", "scope_id is required"},
		{"negative offset", "scope=project&scope_id=p1&offset=-1", "offset must not be negative"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			s.handleListFindingsHistory(rr, histReq(t, tc.query))
			if rr.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
			}
			assertErrorContains(t, rr, tc.want)
		})
	}
}

// TestHandleListFindingsHistory_403Cases covers unknown scope_id → 403.
func TestHandleListFindingsHistory_403Cases(t *testing.T) {
	s := newHistServer(t)

	cases := []struct {
		name  string
		query string
	}{
		{"unknown project", "scope=project&scope_id=no-such-proj"},
		{"unknown workflow_instance", "scope=workflow_instance&scope_id=no-such-wfi"},
		{"unknown session", "scope=session&scope_id=no-such-sess"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			s.handleListFindingsHistory(rr, histReq(t, tc.query))
			if rr.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403; body: %s", rr.Code, rr.Body.String())
			}
		})
	}
}

// TestHandleListFindingsHistory_EmptyResult verifies items is [] not null on empty history.
func TestHandleListFindingsHistory_EmptyResult(t *testing.T) {
	s := newHistServer(t)
	seedProjectForFindings(t, s, "proj-hist-empty")

	rr := httptest.NewRecorder()
	s.handleListFindingsHistory(rr, histReq(t, "scope=project&scope_id=proj-hist-empty"))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	resp := decodeHistResp(t, rr)
	if resp.Items == nil {
		t.Error("items must be empty slice, not nil")
	}
	if len(resp.Items) != 0 {
		t.Errorf("items len = %d, want 0", len(resp.Items))
	}
	if resp.Limit != 50 {
		t.Errorf("default limit = %v, want 50", resp.Limit)
	}
	if resp.Offset != 0 {
		t.Errorf("offset = %v, want 0", resp.Offset)
	}
}

// TestHandleListFindingsHistory_ContentType checks Content-Type is application/json.
func TestHandleListFindingsHistory_ContentType(t *testing.T) {
	s := newHistServer(t)
	seedProjectForFindings(t, s, "proj-hist-ct")

	rr := httptest.NewRecorder()
	s.handleListFindingsHistory(rr, histReq(t, "scope=project&scope_id=proj-hist-ct"))

	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}
