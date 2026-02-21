package integration

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"be/internal/db"
	"be/internal/repo"
)

// TestTicketListAPI_WorkflowProgressEndToEnd is a comprehensive end-to-end test
// that verifies the complete flow of the workflow progress feature through the HTTP API
func TestTicketListAPI_WorkflowProgressEndToEnd(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	// Initialize DB
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	// Seed project
	seedProject(t, dbPath, "testproj")

	// Seed workflow definitions
	seedWorkflow(t, dbPath, "testproj", "feature", "Feature workflow")
	seedWorkflow(t, dbPath, "testproj", "bugfix", "Bugfix workflow")

	// Start API server
	baseURL := startAPIServer(t, dbPath)

	// Create three tickets via API
	tickets := []struct {
		id    string
		title string
	}{
		{"TESTPROJ-001", "Ticket with active workflow"},
		{"TESTPROJ-002", "Ticket without workflow"},
		{"TESTPROJ-003", "Ticket with completed workflow"},
	}

	for _, tc := range tickets {
		createTicketViaAPI(t, baseURL, "testproj", tc.id, tc.title)
	}

	// Directly seed workflow instances in DB
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Update "feature" workflow def to have 4 phases matching what the test expects
	_, err = database.Exec(`UPDATE workflows SET phases = ? WHERE project_id = 'testproj' AND id = 'feature'`,
		`[{"agent":"investigation","layer":0},{"agent":"test-design","layer":1},{"agent":"implementation","layer":2},{"agent":"verification","layer":3}]`)
	if err != nil {
		database.Close()
		t.Fatalf("failed to update feature workflow phases: %v", err)
	}

	// Create active workflow for TESTPROJ-001
	_, err = database.Exec(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, findings, retry_count, parent_session, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"wf-1", "testproj", "testproj-001", "feature", "active",
		"{}", 0, sql.NullString{}, now, now)
	if err != nil {
		database.Close()
		t.Fatalf("failed to create workflow instance 1: %v", err)
	}

	// Create agent_sessions for derivation:
	// investigation completed (L0), test-design skipped (L1, no session), implementation running (L2)
	_, err = database.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
			status, result, restart_count, started_at, ended_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"sess-inv", "testproj", "testproj-001", "wf-1", "investigation", "investigation",
		"completed", "pass", 0, now, now, now, now)
	if err != nil {
		database.Close()
		t.Fatalf("failed to create investigation session: %v", err)
	}
	_, err = database.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
			status, restart_count, started_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"sess-impl", "testproj", "testproj-001", "wf-1", "implementation", "implementation",
		"running", 0, now, now, now)
	if err != nil {
		database.Close()
		t.Fatalf("failed to create implementation session: %v", err)
	}

	// Create completed workflow for TESTPROJ-003
	_, err = database.Exec(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, findings, retry_count, parent_session, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"wf-3", "testproj", "testproj-003", "bugfix", "completed",
		"{}", 0, sql.NullString{}, now, now)
	if err != nil {
		database.Close()
		t.Fatalf("failed to create workflow instance 3: %v", err)
	}

	database.Close()

	// Wait for DB writes to settle
	time.Sleep(50 * time.Millisecond)

	// Call GET /api/v1/tickets?status=open
	req, _ := http.NewRequest("GET", baseURL+"/api/v1/tickets?status=open", nil)
	req.Header.Set("X-Project", "testproj")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to list tickets: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result struct {
		Tickets []*repo.PendingTicket `json:"tickets"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify we got 3 tickets
	if len(result.Tickets) != 3 {
		t.Fatalf("expected 3 tickets, got %d", len(result.Tickets))
	}

	// Find tickets by ID and verify workflow_progress
	ticketMap := make(map[string]*repo.PendingTicket)
	for _, ticket := range result.Tickets {
		ticketMap[ticket.ID] = ticket
	}

	// TESTPROJ-001: should have workflow_progress with 2 completed (1 completed + 1 skipped), 4 total
	ticket1 := ticketMap["testproj-001"]
	if ticket1 == nil {
		t.Fatal("TESTPROJ-001 not found in response")
	}
	if ticket1.WorkflowProgress == nil {
		t.Fatal("expected workflow_progress for TESTPROJ-001")
	}
	if ticket1.WorkflowProgress.WorkflowName != "feature" {
		t.Fatalf("expected workflow_name 'feature', got %q", ticket1.WorkflowProgress.WorkflowName)
	}
	if ticket1.WorkflowProgress.CurrentPhase != "implementation" {
		t.Fatalf("expected current_phase 'implementation', got %q", ticket1.WorkflowProgress.CurrentPhase)
	}
	if ticket1.WorkflowProgress.CompletedPhases != 2 {
		t.Fatalf("expected completed_phases 2 (completed + skipped), got %d", ticket1.WorkflowProgress.CompletedPhases)
	}
	if ticket1.WorkflowProgress.TotalPhases != 4 {
		t.Fatalf("expected total_phases 4, got %d", ticket1.WorkflowProgress.TotalPhases)
	}
	if ticket1.WorkflowProgress.Status != "active" {
		t.Fatalf("expected status 'active', got %q", ticket1.WorkflowProgress.Status)
	}

	// TESTPROJ-002: should have nil workflow_progress
	ticket2 := ticketMap["testproj-002"]
	if ticket2 == nil {
		t.Fatal("TESTPROJ-002 not found in response")
	}
	if ticket2.WorkflowProgress != nil {
		t.Fatalf("expected nil workflow_progress for TESTPROJ-002, got %+v", ticket2.WorkflowProgress)
	}

	// TESTPROJ-003: should have nil workflow_progress (workflow is completed, not active)
	ticket3 := ticketMap["testproj-003"]
	if ticket3 == nil {
		t.Fatal("TESTPROJ-003 not found in response")
	}
	if ticket3.WorkflowProgress != nil {
		t.Fatalf("expected nil workflow_progress for TESTPROJ-003 (completed workflow), got %+v", ticket3.WorkflowProgress)
	}
}

func TestTicketListAPI_InProgressFilter_ShowsWorkflowProgress(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProject(t, dbPath, "testproj2")
	seedWorkflow(t, dbPath, "testproj2", "feature", "Feature workflow")
	baseURL := startAPIServer(t, dbPath)

	// Create tickets
	createTicketViaAPI(t, baseURL, "testproj2", "PROJ2-001", "In progress ticket")
	createTicketViaAPI(t, baseURL, "testproj2", "PROJ2-002", "Open ticket")

	// Set PROJ2-001 to in-progress status
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}

	_, err = database.Exec(`UPDATE tickets SET status = 'in_progress' WHERE id = 'proj2-001'`)
	if err != nil {
		database.Close()
		t.Fatalf("failed to update ticket status: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Update "feature" workflow def to have 5 phases
	_, err = database.Exec(`UPDATE workflows SET phases = ? WHERE project_id = 'testproj2' AND id = 'feature'`,
		`[{"agent":"phase1","layer":0},{"agent":"phase2","layer":1},{"agent":"phase3","layer":2},{"agent":"phase4","layer":3},{"agent":"phase5","layer":4}]`)
	if err != nil {
		database.Close()
		t.Fatalf("failed to update feature workflow: %v", err)
	}

	// Create active workflow for PROJ2-001
	_, err = database.Exec(`
		INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, findings, retry_count, parent_session, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"wf-prog", "testproj2", "proj2-001", "feature", "active",
		"{}", 0, sql.NullString{}, now, now)
	if err != nil {
		database.Close()
		t.Fatalf("failed to create workflow instance: %v", err)
	}

	// Create agent_sessions for derivation: phase1 & phase2 completed, phase3 running
	for _, sess := range []struct{ id, phase, status, result string }{
		{"sess-p1", "phase1", "completed", "pass"},
		{"sess-p2", "phase2", "completed", "pass"},
		{"sess-p3", "phase3", "running", ""},
	} {
		result := sql.NullString{String: sess.result, Valid: sess.result != ""}
		_, err = database.Exec(`
			INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type,
				status, result, restart_count, started_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			sess.id, "testproj2", "proj2-001", "wf-prog", sess.phase, sess.phase,
			sess.status, result, 0, now, now, now)
		if err != nil {
			database.Close()
			t.Fatalf("failed to create session %s: %v", sess.id, err)
		}
	}

	database.Close()
	time.Sleep(50 * time.Millisecond)

	// Request tickets with status=in_progress
	req, _ := http.NewRequest("GET", baseURL+"/api/v1/tickets?status=in_progress", nil)
	req.Header.Set("X-Project", "testproj2")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to list tickets: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result struct {
		Tickets []*repo.PendingTicket `json:"tickets"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should only get PROJ2-001
	if len(result.Tickets) != 1 {
		t.Fatalf("expected 1 in-progress ticket, got %d", len(result.Tickets))
	}

	ticket := result.Tickets[0]
	if ticket.ID != "proj2-001" {
		t.Fatalf("expected ticket proj2-001, got %q", ticket.ID)
	}

	// Verify workflow_progress shows completion percentage
	if ticket.WorkflowProgress == nil {
		t.Fatal("expected workflow_progress for in-progress ticket")
	}

	// Completion: 2/5 = 40%
	if ticket.WorkflowProgress.CompletedPhases != 2 {
		t.Fatalf("expected completed_phases 2, got %d", ticket.WorkflowProgress.CompletedPhases)
	}
	if ticket.WorkflowProgress.TotalPhases != 5 {
		t.Fatalf("expected total_phases 5, got %d", ticket.WorkflowProgress.TotalPhases)
	}

	// Calculate percentage
	percentage := float64(ticket.WorkflowProgress.CompletedPhases) / float64(ticket.WorkflowProgress.TotalPhases) * 100
	if percentage != 40.0 {
		t.Fatalf("expected 40%% completion, got %.2f%%", percentage)
	}
}

func TestTicketListAPI_WorkflowProgressErrorRecovery(t *testing.T) {
	// Test that if workflow progress loading fails, tickets are still returned
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProject(t, dbPath, "testproj3")
	baseURL := startAPIServer(t, dbPath)

	// Create ticket
	createTicketViaAPI(t, baseURL, "testproj3", "PROJ3-001", "Test ticket")

	// Request tickets - even if workflow enrichment has issues, tickets should return
	req, _ := http.NewRequest("GET", baseURL+"/api/v1/tickets?status=open", nil)
	req.Header.Set("X-Project", "testproj3")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to list tickets: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 even if workflow enrichment fails, got %d", resp.StatusCode)
	}

	var result struct {
		Tickets []*repo.PendingTicket `json:"tickets"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(result.Tickets) != 1 {
		t.Fatalf("expected 1 ticket, got %d", len(result.Tickets))
	}

	// Ticket should still be returned without workflow_progress
	if result.Tickets[0].WorkflowProgress != nil {
		t.Fatalf("expected nil workflow_progress, got %+v", result.Tickets[0].WorkflowProgress)
	}
}

// Helper function to seed workflow definition directly into DB
func seedWorkflow(t *testing.T, dbPath, projectID, workflowID, description string) {
	t.Helper()
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB for seeding workflow: %v", err)
	}
	defer database.Close()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	phasesJSON, _ := json.Marshal([]map[string]interface{}{
		{"agent": "phase1", "layer": 0},
		{"agent": "phase2", "layer": 1},
		{"agent": "phase3", "layer": 2},
	})
	_, err = database.Exec(`
		INSERT INTO workflows (project_id, id, description, phases, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		projectID, workflowID, description, string(phasesJSON), now, now)
	if err != nil {
		t.Fatalf("failed to seed workflow: %v", err)
	}
}

// Helper function to create tickets via API
func createTicketViaAPI(t *testing.T, baseURL, projectID, ticketID, title string) {
	t.Helper()

	body := `{"id":"` + ticketID + `","title":"` + title + `","created_by":"tester"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", projectID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create ticket %s: %v", ticketID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("failed to create ticket %s, status: %d", ticketID, resp.StatusCode)
	}
}
