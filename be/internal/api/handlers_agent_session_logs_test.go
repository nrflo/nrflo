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
)

func newASLogsServer(t *testing.T) (*Server, *db.Pool) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "aslogs_test.db")
	if err := apiCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	for _, q := range []string{
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('test-proj', 'Test', datetime('now'), datetime('now'))`,
		`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES ('test-proj', 'test-wf', '', 'project', datetime('now'), datetime('now'))`,
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, findings, created_at, updated_at) VALUES ('test-wfi', 'test-proj', '', 'test-wf', 'active', 'project', '{}', datetime('now'), datetime('now'))`,
		`INSERT INTO scheduled_tasks (id, project_id, name, cron_expression, created_at, updated_at) VALUES ('test-sched', 'test-proj', 'Sched', '0 * * * *', datetime('now'), datetime('now'))`,
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, findings, scheduled_task_id, created_at, updated_at) VALUES ('test-wfi-sched', 'test-proj', '', 'test-wf', 'completed', 'project', '{}', 'test-sched', datetime('now'), datetime('now'))`,
		`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, layer, execution_mode, created_at, updated_at) VALUES ('agent-api', 'test-proj', 'test-wf', 'sonnet', 20, '', 0, 'api', datetime('now'), datetime('now'))`,
	} {
		if _, err := pool.Exec(q); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return &Server{pool: pool, clock: clock.Real()}, pool
}

func insertASLogsSess(t *testing.T, pool *db.Pool, id, wfiID, agentType, status, findings string, endedAt time.Time) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	started := endedAt.Add(-10 * time.Second).UTC().Format(time.RFC3339Nano)
	ended := endedAt.UTC().Format(time.RFC3339Nano)
	_, err := pool.Exec(`
		INSERT INTO agent_sessions
		(id, project_id, ticket_id, workflow_instance_id, phase, agent_type, status, findings, started_at, ended_at, created_at, updated_at)
		VALUES (?, 'test-proj', '', ?, 'ph', ?, ?, ?, ?, ?, ?, ?)`,
		id, wfiID, agentType, status, findings, started, ended, now, now,
	)
	if err != nil {
		t.Fatalf("insertASLogsSess(%s): %v", id, err)
	}
}

func TestHandleListAgentSessionLogs_MissingProject(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent-session-logs", nil)
	rr := httptest.NewRecorder()
	s.handleListAgentSessionLogs(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	assertErrorContains(t, rr, "project is required")
}

func TestHandleListAgentSessionLogs_EmptyList(t *testing.T) {
	s, _ := newASLogsServer(t)
	req := httptest.NewRequest(http.MethodGet, withProject("/api/v1/agent-session-logs", "test-proj"), nil)
	rr := httptest.NewRecorder()
	s.handleListAgentSessionLogs(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	var resp struct {
		Sessions   []interface{} `json:"sessions"`
		Total      int           `json:"total"`
		Page       int           `json:"page"`
		PerPage    int           `json:"per_page"`
		TotalPages int           `json:"total_pages"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Sessions == nil {
		t.Errorf("sessions should not be null")
	}
	if len(resp.Sessions) != 0 {
		t.Errorf("sessions count = %d, want 0", len(resp.Sessions))
	}
	if resp.Total != 0 {
		t.Errorf("total = %d, want 0", resp.Total)
	}
	if resp.Page != 1 {
		t.Errorf("page = %d, want 1", resp.Page)
	}
	if resp.PerPage != 20 {
		t.Errorf("per_page = %d, want 20", resp.PerPage)
	}
	if resp.TotalPages != 0 {
		t.Errorf("total_pages = %d, want 0", resp.TotalPages)
	}
}

func TestHandleListAgentSessionLogs_HappyPath(t *testing.T) {
	s, pool := newASLogsServer(t)
	base := time.Date(2025, 8, 1, 12, 0, 0, 0, time.UTC)

	// sess-1: scheduled, workflow_final_result present, execution_mode=api (matches agent-api def).
	insertASLogsSess(t, pool, "sess-1", "test-wfi-sched", "agent-api", "completed",
		`{"workflow_final_result":"all done"}`, base.Add(2*time.Second))
	// sess-2: not scheduled, no findings, execution_mode unknown (no matching agent_def → NULL → "").
	insertASLogsSess(t, pool, "sess-2", "test-wfi", "agent-cli", "failed", "", base.Add(time.Second))

	req := httptest.NewRequest(http.MethodGet, withProject("/api/v1/agent-session-logs", "test-proj"), nil)
	rr := httptest.NewRecorder()
	s.handleListAgentSessionLogs(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Sessions []struct {
			SessionID           string  `json:"session_id"`
			Status              string  `json:"status"`
			Scheduled           bool    `json:"scheduled"`
			WorkflowFinalResult string  `json:"workflow_final_result"`
			ExecutionMode       string  `json:"execution_mode"`
			DurationSec         float64 `json:"duration_sec"`
		} `json:"sessions"`
		Total      int `json:"total"`
		Page       int `json:"page"`
		PerPage    int `json:"per_page"`
		TotalPages int `json:"total_pages"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 2 {
		t.Errorf("total = %d, want 2", resp.Total)
	}
	if resp.Page != 1 {
		t.Errorf("page = %d, want 1", resp.Page)
	}
	if resp.PerPage != 20 {
		t.Errorf("per_page = %d, want 20", resp.PerPage)
	}
	if resp.TotalPages != 1 {
		t.Errorf("total_pages = %d, want 1 (ceil(2/20))", resp.TotalPages)
	}
	if len(resp.Sessions) != 2 {
		t.Fatalf("sessions count = %d, want 2", len(resp.Sessions))
	}

	// First (most recent): sess-1 is scheduled, has workflow_final_result, api mode.
	s0 := resp.Sessions[0]
	if s0.SessionID != "sess-1" {
		t.Errorf("sessions[0].SessionID = %q, want sess-1 (most recent)", s0.SessionID)
	}
	if s0.Status != "completed" {
		t.Errorf("sessions[0].Status = %q, want completed", s0.Status)
	}
	if !s0.Scheduled {
		t.Errorf("sessions[0].Scheduled = false, want true")
	}
	if s0.WorkflowFinalResult != "all done" {
		t.Errorf("sessions[0].WorkflowFinalResult = %q, want all done", s0.WorkflowFinalResult)
	}
	if s0.ExecutionMode != "api" {
		t.Errorf("sessions[0].ExecutionMode = %q, want api", s0.ExecutionMode)
	}
	if s0.DurationSec <= 0 {
		t.Errorf("sessions[0].DurationSec = %v, want > 0", s0.DurationSec)
	}

	// Second (older): sess-2 not scheduled, no workflow_final_result, no execution_mode.
	s1 := resp.Sessions[1]
	if s1.SessionID != "sess-2" {
		t.Errorf("sessions[1].SessionID = %q, want sess-2", s1.SessionID)
	}
	if s1.Scheduled {
		t.Errorf("sessions[1].Scheduled = true, want false")
	}
	if s1.WorkflowFinalResult != "" {
		t.Errorf("sessions[1].WorkflowFinalResult = %q, want empty", s1.WorkflowFinalResult)
	}
	if s1.ExecutionMode != "" {
		t.Errorf("sessions[1].ExecutionMode = %q, want empty (no agent def)", s1.ExecutionMode)
	}
}

func TestHandleListAgentSessionLogs_PerPageCapped(t *testing.T) {
	s, _ := newASLogsServer(t)
	req := httptest.NewRequest(http.MethodGet,
		withProject("/api/v1/agent-session-logs?per_page=500", "test-proj"), nil)
	rr := httptest.NewRecorder()
	s.handleListAgentSessionLogs(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	var resp struct {
		PerPage int `json:"per_page"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.PerPage != 100 {
		t.Errorf("per_page = %d, want 100 (capped)", resp.PerPage)
	}
}

func TestHandleListAgentSessionLogs_InvalidPageParam(t *testing.T) {
	s, _ := newASLogsServer(t)
	req := httptest.NewRequest(http.MethodGet,
		withProject("/api/v1/agent-session-logs?page=abc", "test-proj"), nil)
	rr := httptest.NewRecorder()
	s.handleListAgentSessionLogs(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	var resp struct {
		Page int `json:"page"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Page != 1 {
		t.Errorf("page = %d, want 1 (fallback on invalid param)", resp.Page)
	}
}
