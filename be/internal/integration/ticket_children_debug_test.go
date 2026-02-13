package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"testing"

	"be/internal/db"
	"be/internal/model"
)

func TestGetEpicTicketWithChildrenDebug(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	seedProject(t, dbPath, "epic-proj")
	baseURL := startAPIServer(t, dbPath)

	// Create an epic ticket
	epicBody := `{"id":"EPIC-001","title":"Epic Ticket","issue_type":"epic","priority":1,"created_by":"tester"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(epicBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "epic-proj")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create epic: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201 for epic, got %d: %s", resp.StatusCode, string(respBody))
	}

	// Create child ticket
	child1Body := `{"id":"CHILD-001","title":"Child 1","parent_ticket_id":"EPIC-001","priority":3,"created_by":"tester"}`
	req, _ = http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(child1Body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "epic-proj")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create child 1: %v", err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for child 1, got %d: %s", resp.StatusCode, string(respBody))
	}
	t.Logf("Child 1 created: %s", string(respBody))

	// Open DB and check what was actually stored
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen DB: %v", err)
	}
	defer database.Close()

	// Query the database directly
	rows, err := database.Query(`SELECT id, project_id, issue_type, parent_ticket_id FROM tickets ORDER BY id`)
	if err != nil {
		t.Fatalf("failed to query tickets: %v", err)
	}
	defer rows.Close()

	t.Log("=== Tickets in database ===")
	for rows.Next() {
		var id, projectID, issueType string
		var parentTicketID *string
		if err := rows.Scan(&id, &projectID, &issueType, &parentTicketID); err != nil {
			t.Fatalf("failed to scan row: %v", err)
		}
		parentStr := "NULL"
		if parentTicketID != nil {
			parentStr = *parentTicketID
		}
		t.Logf("  id=%s project=%s type=%s parent=%s", id, projectID, issueType, parentStr)
	}

	// Test the ListByParent query directly
	t.Log("=== Direct ListByParent query ===")
	rows2, err := database.Query(`
		SELECT id, project_id, title, description, status, priority, issue_type, parent_ticket_id, created_at, updated_at, closed_at, created_by, close_reason
		FROM tickets
		WHERE LOWER(project_id) = LOWER(?) AND LOWER(parent_ticket_id) = LOWER(?)
		ORDER BY priority ASC, created_at ASC`, "epic-proj", "epic-001")
	if err != nil {
		t.Fatalf("failed to run ListByParent query: %v", err)
	}
	defer rows2.Close()

	count := 0
	for rows2.Next() {
		count++
		var id string
		var otherFields [12]interface{}
		vars := append([]interface{}{&id}, make([]interface{}, 12)...)
		for i := range otherFields {
			vars[i+1] = &otherFields[i]
		}
		if err := rows2.Scan(vars...); err != nil {
			t.Fatalf("failed to scan child row: %v", err)
		}
		t.Logf("  Found child: id=%s", id)
	}
	t.Logf("  Total children found: %d", count)

	// Now test via HTTP API
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/tickets/EPIC-001", nil)
	req.Header.Set("X-Project", "epic-proj")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to get epic: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(respBody))
	}

	respBodyBytes, _ := io.ReadAll(resp.Body)
	t.Logf("=== HTTP Response ===")
	t.Logf("%s", string(respBodyBytes))

	var result struct {
		ID        string          `json:"id"`
		IssueType string          `json:"issue_type"`
		Title     string          `json:"title"`
		Children  []*model.Ticket `json:"children"`
	}
	if err := json.Unmarshal(respBodyBytes, &result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	t.Logf("=== Parsed response ===")
	t.Logf("ID: %s, Type: %s, Children count: %d", result.ID, result.IssueType, len(result.Children))

	if len(result.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(result.Children))
	}
}
