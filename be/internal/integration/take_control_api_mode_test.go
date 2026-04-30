package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"be/internal/db"
)

// seedAPISession inserts the full chain of rows required for isAPISession() to
// return true: project → workflow → agent_def(execution_mode=api) →
// ticket → workflow_instance → agent_session.
//
// projectID / ticketID / sessionID / wfiID must all be unique per test to
// avoid collisions when multiple tests share the same copied template DB.
func seedAPISession(t *testing.T, dbPath, projectID, ticketID, sessionID, wfiID string) {
	t.Helper()

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("seedAPISession: open DB: %v", err)
	}
	defer database.Close()

	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Project
	if _, err := database.Exec(
		`INSERT OR IGNORE INTO projects (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		projectID, projectID, now, now,
	); err != nil {
		t.Fatalf("seedAPISession: project: %v", err)
	}

	// Workflow definition
	if _, err := database.Exec(
		`INSERT OR IGNORE INTO workflows (id, project_id, description, scope_type, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"api-wf", projectID, "API workflow", "ticket", now, now,
	); err != nil {
		t.Fatalf("seedAPISession: workflow: %v", err)
	}

	// Agent definition with execution_mode='api' (id must match sess.AgentType)
	if _, err := database.Exec(`
		INSERT OR IGNORE INTO agent_definitions
			(id, project_id, workflow_id, model, timeout, prompt, layer, execution_mode, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"api-agent", projectID, "api-wf", "sonnet", 60, "do stuff", 0, "api", now, now,
	); err != nil {
		t.Fatalf("seedAPISession: agent_definition: %v", err)
	}

	// Ticket (required FK for workflow_instances)
	if _, err := database.Exec(
		`INSERT OR IGNORE INTO tickets (id, project_id, title, created_at, updated_at, created_by) VALUES (?, ?, ?, ?, ?, ?)`,
		ticketID, projectID, "API mode test ticket", now, now, "test",
	); err != nil {
		t.Fatalf("seedAPISession: ticket: %v", err)
	}

	// Workflow instance (links ticket → workflow)
	if _, err := database.Exec(`
		INSERT OR IGNORE INTO workflow_instances
			(id, project_id, ticket_id, workflow_id, scope_type, status, findings, retry_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		wfiID, projectID, ticketID, "api-wf", "ticket", "active", "{}", 0, now, now,
	); err != nil {
		t.Fatalf("seedAPISession: workflow_instance: %v", err)
	}

	// Agent session — agent_type must match the agent_definition id ("api-agent")
	if _, err := database.Exec(`
		INSERT OR IGNORE INTO agent_sessions
			(id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
			 status, result, result_reason, pid, findings,
			 context_left, ancestor_session_id, spawn_command, prompt,
			 restart_count, started_at, ended_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 'running', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, 0, ?, NULL, ?, ?)`,
		sessionID, projectID, ticketID, wfiID, "L0", "api-agent", now, now, now,
	); err != nil {
		t.Fatalf("seedAPISession: agent_session: %v", err)
	}
}

// TestTakeControlHandler_APIMode_Returns409 verifies that
// POST /api/v1/tickets/:id/workflow/take-control returns HTTP 409 Conflict
// with body {"error":"api_mode_unsupported"} when the referenced session
// was spawned in API execution mode.
func TestTakeControlHandler_APIMode_Returns409(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("copyTemplateDB: %v", err)
	}

	projectID := "proj-api-tc-1"
	ticketID := "TICK-API-TC-1"
	sessionID := "sess-api-tc-1"
	wfiID := "wfi-api-tc-1"

	seedAPISession(t, dbPath, projectID, ticketID, sessionID, wfiID)
	baseURL := startAPIServer(t, dbPath)

	body, _ := json.Marshal(map[string]string{
		"workflow":   "api-wf",
		"session_id": sessionID,
	})
	req, _ := http.NewRequest("POST",
		baseURL+"/api/v1/tickets/"+ticketID+"/workflow/take-control",
		bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", projectID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409 Conflict; body: %s", resp.StatusCode, string(respBody))
	}

	var errResp map[string]string
	if err := json.Unmarshal(respBody, &errResp); err != nil {
		t.Fatalf("unmarshal response: %v; body: %s", err, string(respBody))
	}
	if errResp["error"] != "api_mode_unsupported" {
		t.Errorf("error = %q, want api_mode_unsupported", errResp["error"])
	}
}

// TestTakeControlProjectHandler_APIMode_Returns409 verifies that the project-scoped
// POST /api/v1/projects/:id/workflow/take-control also returns 409 Conflict
// for API-mode sessions, mirroring the ticket-scoped guard.
func TestTakeControlProjectHandler_APIMode_Returns409(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("copyTemplateDB: %v", err)
	}

	projectID := "proj-api-prtc-1"
	ticketID := "TICK-API-PRTC-1"
	sessionID := "sess-api-prtc-1"
	wfiID := "wfi-api-prtc-1"

	seedAPISession(t, dbPath, projectID, ticketID, sessionID, wfiID)
	baseURL := startAPIServer(t, dbPath)

	body, _ := json.Marshal(map[string]string{
		"workflow":    "api-wf",
		"session_id":  sessionID,
		"instance_id": wfiID,
	})
	req, _ := http.NewRequest("POST",
		baseURL+"/api/v1/projects/"+projectID+"/workflow/take-control",
		bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409 Conflict; body: %s", resp.StatusCode, string(respBody))
	}

	var errResp map[string]string
	if err := json.Unmarshal(respBody, &errResp); err != nil {
		t.Fatalf("unmarshal response: %v; body: %s", err, string(respBody))
	}
	if errResp["error"] != "api_mode_unsupported" {
		t.Errorf("error = %q, want api_mode_unsupported", errResp["error"])
	}
}

// TestTakeControlHandler_CLIMode_ReachesOrchestrator verifies that a session
// with execution_mode='cli' (or no record at all) is NOT blocked by the
// isAPISession guard and falls through to the orchestrator (which returns 404
// when no orchestration is running). This confirms the guard is selective.
func TestTakeControlHandler_CLIMode_ReachesOrchestrator(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("copyTemplateDB: %v", err)
	}

	seedProject(t, dbPath, "proj-cli-tc")
	baseURL := startAPIServer(t, dbPath)

	// session_id does not exist at all — isAPISession returns false (err != nil),
	// so the guard passes and the orchestrator returns 404 (no running orchestration).
	body, _ := json.Marshal(map[string]string{
		"workflow":   "some-wf",
		"session_id": "nonexistent-cli-session",
	})
	req, _ := http.NewRequest("POST",
		baseURL+"/api/v1/tickets/TICK-CLI/workflow/take-control",
		bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "proj-cli-tc")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	// 404 means the guard passed and the orchestrator was reached (no active run).
	// 409 would mean isAPISession incorrectly flagged a non-existent session.
	if resp.StatusCode == http.StatusConflict {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("got 409 (api_mode_unsupported) for a non-API session; body: %s", string(body))
	}
	if resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 404 (orchestrator reached); body: %s", resp.StatusCode, string(body))
	}
}
