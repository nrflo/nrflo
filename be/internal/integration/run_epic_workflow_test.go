package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

// TestRunEpicWorkflow_HappyPath verifies epic with children creates chain with correct topo order
func TestRunEpicWorkflow_HappyPath(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	// Initialize DB
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	seedProject(t, dbPath, "test-proj")
	baseURL := startAPIServer(t, dbPath)

	base := time.Now()
	now := base.UTC().Format(time.RFC3339Nano)

	// Open DB for seeding
	database, err = db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	defer database.Close()

	// Create epic ticket
	_, err = database.Exec(`
		INSERT INTO tickets (id, project_id, title, status, issue_type, priority, created_at, updated_at, created_by)
		VALUES (?, ?, 'Epic Story', 'open', 'epic', 1, ?, ?, 'test')`,
		"epic-1", "test-proj", now, now)
	if err != nil {
		t.Fatalf("failed to create epic: %v", err)
	}

	// Create 3 child tickets with dependencies
	for i, childID := range []string{"child-a", "child-b", "child-c"} {
		created := base.Add(time.Duration(i+1) * time.Second).UTC().Format(time.RFC3339Nano)
		_, err := database.Exec(`
			INSERT INTO tickets (id, project_id, title, status, issue_type, priority, parent_ticket_id, created_at, updated_at, created_by)
			VALUES (?, ?, ?, 'open', 'feature', 2, 'epic-1', ?, ?, 'test')`,
			childID, "test-proj", strings.ToUpper(childID), created, created)
		if err != nil {
			t.Fatalf("failed to create child %s: %v", childID, err)
		}
	}

	// Add dependencies: C depends on B, B depends on A
	depRepo := repo.NewDependencyRepo(database, clock.Real())
	depRepo.Create(&model.Dependency{ProjectID: "test-proj", IssueID: "child-b", DependsOnID: "child-a", Type: "blocks", CreatedBy: "test"})
	depRepo.Create(&model.Dependency{ProjectID: "test-proj", IssueID: "child-c", DependsOnID: "child-b", Type: "blocks", CreatedBy: "test"})

	// POST request
	body := map[string]interface{}{
		"workflow_name": "test",
		"start":         false,
	}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets/epic-1/workflow/run-epic", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "test-proj")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyText, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 201, got %d: %s", resp.StatusCode, string(bodyText))
	}

	var result struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		EpicTicketID string `json:"epic_ticket_id"`
		Status       string `json:"status"`
		Items        []struct {
			TicketID string `json:"ticket_id"`
			Position int    `json:"position"`
		} `json:"items"`
	}
	bodyText, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(bodyText, &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	// Verify response
	if result.EpicTicketID != "epic-1" {
		t.Errorf("expected epic_ticket_id 'epic-1', got %s", result.EpicTicketID)
	}
	if result.Status != string(model.ChainStatusPending) {
		t.Errorf("expected status 'pending', got %s", result.Status)
	}
	if !strings.Contains(result.Name, "Epic:") {
		t.Errorf("expected name to contain 'Epic:', got %s", result.Name)
	}

	// Verify 3 items in correct topological order: A, B, C
	if len(result.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(result.Items))
	}
	if result.Items[0].TicketID != "child-a" {
		t.Errorf("expected first item 'child-a', got %s", result.Items[0].TicketID)
	}
	if result.Items[1].TicketID != "child-b" {
		t.Errorf("expected second item 'child-b', got %s", result.Items[1].TicketID)
	}
	if result.Items[2].TicketID != "child-c" {
		t.Errorf("expected third item 'child-c', got %s", result.Items[2].TicketID)
	}

	// Verify positions
	for i, item := range result.Items {
		if item.Position != i {
			t.Errorf("item %d: expected position %d, got %d", i, i, item.Position)
		}
	}
}

// TestRunEpicWorkflow_NoChildren verifies 400 when epic has no children
func TestRunEpicWorkflow_NoChildren(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	seedProject(t, dbPath, "test-proj")
	baseURL := startAPIServer(t, dbPath)

	// Create epic with no children
	database, err = db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	defer database.Close()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = database.Exec(`
		INSERT INTO tickets (id, project_id, title, status, issue_type, priority, created_at, updated_at, created_by)
		VALUES (?, ?, 'Empty Epic', 'open', 'epic', 1, ?, ?, 'test')`,
		"epic-empty", "test-proj", now, now)
	if err != nil {
		t.Fatalf("failed to create epic: %v", err)
	}

	body := map[string]interface{}{
		"workflow_name": "test",
		"start":         false,
	}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets/epic-empty/workflow/run-epic", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "test-proj")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		bodyText, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 400, got %d: %s", resp.StatusCode, string(bodyText))
	}

	var errResp map[string]string
	bodyText, _ := io.ReadAll(resp.Body)
	json.Unmarshal(bodyText, &errResp)
	if !strings.Contains(errResp["error"], "no child tickets") {
		t.Errorf("expected 'no child tickets' error, got: %s", errResp["error"])
	}
}

// TestRunEpicWorkflow_NonEpicTicket verifies 400 when ticket is not an epic
func TestRunEpicWorkflow_NonEpicTicket(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	seedProject(t, dbPath, "test-proj")
	baseURL := startAPIServer(t, dbPath)

	// Create regular feature ticket
	database, err = db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	defer database.Close()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = database.Exec(`
		INSERT INTO tickets (id, project_id, title, status, issue_type, priority, created_at, updated_at, created_by)
		VALUES (?, ?, 'Regular Feature', 'open', 'feature', 2, ?, ?, 'test')`,
		"feat-1", "test-proj", now, now)
	if err != nil {
		t.Fatalf("failed to create ticket: %v", err)
	}

	body := map[string]interface{}{
		"workflow_name": "test",
		"start":         false,
	}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets/feat-1/workflow/run-epic", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "test-proj")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		bodyText, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 400, got %d: %s", resp.StatusCode, string(bodyText))
	}

	var errResp map[string]string
	bodyText, _ := io.ReadAll(resp.Body)
	json.Unmarshal(bodyText, &errResp)
	if !strings.Contains(errResp["error"], "not an epic") {
		t.Errorf("expected 'not an epic' error, got: %s", errResp["error"])
	}
}

// TestRunEpicWorkflow_ExcludesClosedChildren verifies only non-closed children are included
func TestRunEpicWorkflow_ExcludesClosedChildren(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	seedProject(t, dbPath, "test-proj")
	baseURL := startAPIServer(t, dbPath)

	database, err = db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	defer database.Close()

	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Create epic
	_, err = database.Exec(`
		INSERT INTO tickets (id, project_id, title, status, issue_type, priority, created_at, updated_at, created_by)
		VALUES (?, ?, 'Mixed Epic', 'open', 'epic', 1, ?, ?, 'test')`,
		"epic-mixed", "test-proj", now, now)
	if err != nil {
		t.Fatalf("failed to create epic: %v", err)
	}

	// Create 3 children: 2 open, 1 closed
	for i, status := range []string{"open", "closed", "open"} {
		ticketID := fmt.Sprintf("child-%d", i+1)
		_, err := database.Exec(`
			INSERT INTO tickets (id, project_id, title, status, issue_type, priority, parent_ticket_id, created_at, updated_at, created_by)
			VALUES (?, ?, ?, ?, 'feature', 2, 'epic-mixed', ?, ?, 'test')`,
			ticketID, "test-proj", fmt.Sprintf("Child %d", i+1), status, now, now)
		if err != nil {
			t.Fatalf("failed to create child %s: %v", ticketID, err)
		}
	}

	body := map[string]interface{}{
		"workflow_name": "test",
		"start":         false,
	}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets/epic-mixed/workflow/run-epic", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "test-proj")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyText, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 201, got %d: %s", resp.StatusCode, string(bodyText))
	}

	var result struct {
		Items []struct {
			TicketID string `json:"ticket_id"`
		} `json:"items"`
	}
	bodyText, _ := io.ReadAll(resp.Body)
	json.Unmarshal(bodyText, &result)

	// Should only have 2 items (child-1 and child-3, excluding closed child-2)
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items (open children only), got %d", len(result.Items))
	}

	// Verify child-2 is not included
	for _, item := range result.Items {
		if item.TicketID == "child-2" {
			t.Error("closed child-2 should not be included in chain")
		}
	}
}

// TestRunEpicWorkflow_TicketNotFound verifies 404 when ticket doesn't exist
func TestRunEpicWorkflow_TicketNotFound(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	seedProject(t, dbPath, "test-proj")
	baseURL := startAPIServer(t, dbPath)

	body := map[string]interface{}{
		"workflow_name": "test",
		"start":         false,
	}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets/nonexistent/workflow/run-epic", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "test-proj")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		bodyText, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 404, got %d: %s", resp.StatusCode, string(bodyText))
	}

	var errResp map[string]string
	bodyText, _ := io.ReadAll(resp.Body)
	json.Unmarshal(bodyText, &errResp)
	if !strings.Contains(errResp["error"], "not found") {
		t.Errorf("expected 'not found' error, got: %s", errResp["error"])
	}
}

// TestRunEpicWorkflow_MissingWorkflowName verifies 400 when workflow_name is missing
func TestRunEpicWorkflow_MissingWorkflowName(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	seedProject(t, dbPath, "test-proj")
	baseURL := startAPIServer(t, dbPath)

	// Create epic
	database, err = db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	defer database.Close()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = database.Exec(`
		INSERT INTO tickets (id, project_id, title, status, issue_type, priority, created_at, updated_at, created_by)
		VALUES (?, ?, 'Epic', 'open', 'epic', 1, ?, ?, 'test')`,
		"epic-1", "test-proj", now, now)
	if err != nil {
		t.Fatalf("failed to create epic: %v", err)
	}

	body := map[string]interface{}{
		"start": false,
	}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets/epic-1/workflow/run-epic", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "test-proj")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		bodyText, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 400, got %d: %s", resp.StatusCode, string(bodyText))
	}

	var errResp map[string]string
	bodyText, _ := io.ReadAll(resp.Body)
	json.Unmarshal(bodyText, &errResp)
	if !strings.Contains(errResp["error"], "workflow_name") {
		t.Errorf("expected 'workflow_name' error, got: %s", errResp["error"])
	}
}

// TestListByParent_ExcludesClosedChildren verifies ListByParent filters out closed tickets
func TestListByParent_ExcludesClosedChildren(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Seed project
	_, err = database.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		"test-proj", "Test Project", now, now)
	if err != nil {
		t.Fatalf("failed to seed project: %v", err)
	}

	// Create parent
	_, err = database.Exec(`
		INSERT INTO tickets (id, project_id, title, status, issue_type, priority, created_at, updated_at, created_by)
		VALUES (?, ?, 'Parent', 'open', 'epic', 1, ?, ?, 'test')`,
		"parent-1", "test-proj", now, now)
	if err != nil {
		t.Fatalf("failed to create parent: %v", err)
	}

	// Create children with different statuses
	for i, status := range []string{"open", "in_progress", "closed"} {
		ticketID := fmt.Sprintf("child-%d", i+1)
		_, err := database.Exec(`
			INSERT INTO tickets (id, project_id, title, status, issue_type, priority, parent_ticket_id, created_at, updated_at, created_by)
			VALUES (?, ?, ?, ?, 'feature', 2, 'parent-1', ?, ?, 'test')`,
			ticketID, "test-proj", fmt.Sprintf("Child %d", i+1), status, now, now)
		if err != nil {
			t.Fatalf("failed to create child %s: %v", ticketID, err)
		}
	}

	// Query via TicketRepo.ListByParent
	ticketRepo := repo.NewTicketRepo(database, clock.Real())
	children, err := ticketRepo.ListByParent("test-proj", "parent-1")
	if err != nil {
		t.Fatalf("ListByParent failed: %v", err)
	}

	// Should only return 2 children (open and in-progress)
	if len(children) != 2 {
		t.Fatalf("expected 2 non-closed children, got %d", len(children))
	}

	// Verify closed child is excluded
	for _, child := range children {
		if child.ID == "child-3" {
			t.Error("closed child-3 should be excluded from ListByParent")
		}
		if child.Status == model.StatusClosed {
			t.Errorf("child %s has closed status but was returned", child.ID)
		}
	}

	database.Close()
}
