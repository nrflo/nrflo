package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
)

// TestListWithBlockedInfo_SortsByUpdatedThenCreated tests that tickets are
// sorted by updated_at DESC, then created_at DESC when using quick filters.
func TestListWithBlockedInfo_SortsByUpdatedThenCreated(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	defer database.Close()

	// Seed project
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339Nano)
	_, err = database.Exec(`
		INSERT INTO projects (id, name, created_at, updated_at)
		VALUES (?, ?, ?, ?)`, "testproj", "Test Project", nowStr, nowStr)
	if err != nil {
		t.Fatalf("failed to seed project: %v", err)
	}

	// Create tickets with specific created_at and updated_at timestamps
	tickets := []struct {
		id        string
		title     string
		createdAt time.Time
		updatedAt time.Time
	}{
		// Ticket 1: oldest created, never updated
		{"TICKET-1", "First ticket", now.Add(-3 * time.Hour), now.Add(-3 * time.Hour)},
		// Ticket 2: middle created, recently updated (should be first)
		{"TICKET-2", "Second ticket", now.Add(-2 * time.Hour), now.Add(-10 * time.Minute)},
		// Ticket 3: newest created, never updated (should be second)
		{"TICKET-3", "Third ticket", now.Add(-1 * time.Hour), now.Add(-1 * time.Hour)},
		// Ticket 4: old created, moderately updated (should be third)
		{"TICKET-4", "Fourth ticket", now.Add(-4 * time.Hour), now.Add(-30 * time.Minute)},
	}

	for _, tc := range tickets {
		createdStr := tc.createdAt.Format(time.RFC3339Nano)
		updatedStr := tc.updatedAt.Format(time.RFC3339Nano)
		_, err = database.Exec(`
			INSERT INTO tickets (id, project_id, title, description, status, priority, issue_type,
				created_at, updated_at, created_by)
			VALUES (?, ?, ?, '', 'open', 2, 'task', ?, ?, 'tester')`,
			tc.id, "testproj", tc.title, createdStr, updatedStr)
		if err != nil {
			t.Fatalf("failed to insert ticket %s: %v", tc.id, err)
		}
	}

	// Test via repository
	ticketRepo := repo.NewTicketRepo(database, clock.Real())
	filter := &repo.ListFilter{
		ProjectID: "testproj",
		Status:    "open",
	}

	paginated, err := ticketRepo.ListWithBlockedInfo(filter)
	if err != nil {
		t.Fatalf("ListWithBlockedInfo failed: %v", err)
	}
	results := paginated.Tickets

	if len(results) != 4 {
		t.Fatalf("expected 4 tickets, got %d", len(results))
	}

	// Expected order by updated_at DESC, then created_at DESC:
	// TICKET-2 (updated 10min ago), TICKET-4 (updated 30min ago),
	// TICKET-3 (updated=created 1hr ago), TICKET-1 (updated=created 3hr ago)
	expectedOrder := []string{"TICKET-2", "TICKET-4", "TICKET-3", "TICKET-1"}
	for i, expected := range expectedOrder {
		if results[i].ID != expected {
			t.Errorf("position %d: expected %s, got %s", i, expected, results[i].ID)
		}
	}
}

// TestListWithBlockedInfo_TypeFilter tests sorting with issue type filter
func TestListWithBlockedInfo_TypeFilter(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	defer database.Close()

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339Nano)
	_, err = database.Exec(`
		INSERT INTO projects (id, name, created_at, updated_at)
		VALUES (?, ?, ?, ?)`, "proj", "Project", nowStr, nowStr)
	if err != nil {
		t.Fatalf("failed to seed project: %v", err)
	}

	// Create tickets with different types
	tickets := []struct {
		id        string
		issueType string
		updatedAt time.Time
	}{
		{"BUG-1", "bug", now.Add(-1 * time.Hour)},
		{"BUG-2", "bug", now.Add(-30 * time.Minute)}, // Most recently updated bug
		{"TASK-1", "task", now.Add(-2 * time.Hour)},
		{"BUG-3", "bug", now.Add(-2 * time.Hour)},
	}

	for _, tc := range tickets {
		createdStr := now.Add(-3 * time.Hour).Format(time.RFC3339Nano)
		updatedStr := tc.updatedAt.Format(time.RFC3339Nano)
		_, err = database.Exec(`
			INSERT INTO tickets (id, project_id, title, description, status, priority, issue_type,
				created_at, updated_at, created_by)
			VALUES (?, ?, ?, '', 'open', 2, ?, ?, ?, 'tester')`,
			tc.id, "proj", tc.id, tc.issueType, createdStr, updatedStr)
		if err != nil {
			t.Fatalf("failed to insert ticket %s: %v", tc.id, err)
		}
	}

	ticketRepo := repo.NewTicketRepo(database, clock.Real())
	filter := &repo.ListFilter{
		ProjectID: "proj",
		IssueType: "bug",
	}

	paginated, err := ticketRepo.ListWithBlockedInfo(filter)
	if err != nil {
		t.Fatalf("ListWithBlockedInfo failed: %v", err)
	}
	results := paginated.Tickets

	if len(results) != 3 {
		t.Fatalf("expected 3 bug tickets, got %d", len(results))
	}

	// Expected order by updated_at DESC
	expectedOrder := []string{"BUG-2", "BUG-1", "BUG-3"}
	for i, expected := range expectedOrder {
		if results[i].ID != expected {
			t.Errorf("position %d: expected %s, got %s", i, expected, results[i].ID)
		}
	}
}

// TestListWithBlockedInfo_NullUpdatedAt tests sorting when updated_at is NULL
func TestListWithBlockedInfo_NullUpdatedAt(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	defer database.Close()

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339Nano)
	_, err = database.Exec(`
		INSERT INTO projects (id, name, created_at, updated_at)
		VALUES (?, ?, ?, ?)`, "proj", "Project", nowStr, nowStr)
	if err != nil {
		t.Fatalf("failed to seed project: %v", err)
	}

	// Create tickets where some have NULL updated_at (simulating old data)
	tickets := []struct {
		id        string
		createdAt time.Time
		updatedAt *time.Time // nil means NULL
	}{
		{"OLD-1", now.Add(-4 * time.Hour), nil}, // NULL updated_at, oldest created
		{"OLD-2", now.Add(-2 * time.Hour), nil}, // NULL updated_at, newer created
		{"NEW-1", now.Add(-3 * time.Hour), ptrTime(now.Add(-1 * time.Hour))}, // Has updated_at
	}

	for _, tc := range tickets {
		createdStr := tc.createdAt.Format(time.RFC3339Nano)
		var updatedStr interface{}
		if tc.updatedAt != nil {
			updatedStr = tc.updatedAt.Format(time.RFC3339Nano)
		} else {
			updatedStr = createdStr // SQLite doesn't support NULL, so use created_at as fallback
		}
		_, err = database.Exec(`
			INSERT INTO tickets (id, project_id, title, description, status, priority, issue_type,
				created_at, updated_at, created_by)
			VALUES (?, ?, ?, '', 'open', 2, 'task', ?, ?, 'tester')`,
			tc.id, "proj", tc.id, createdStr, updatedStr)
		if err != nil {
			t.Fatalf("failed to insert ticket %s: %v", tc.id, err)
		}
	}

	ticketRepo := repo.NewTicketRepo(database, clock.Real())
	filter := &repo.ListFilter{
		ProjectID: "proj",
		Status:    "open",
	}

	paginated, err := ticketRepo.ListWithBlockedInfo(filter)
	if err != nil {
		t.Fatalf("ListWithBlockedInfo failed: %v", err)
	}
	results := paginated.Tickets

	if len(results) != 3 {
		t.Fatalf("expected 3 tickets, got %d", len(results))
	}

	// Expected order: NEW-1 (most recently updated), OLD-2 (NULL but newer created),
	// OLD-1 (NULL and oldest created)
	expectedOrder := []string{"NEW-1", "OLD-2", "OLD-1"}
	for i, expected := range expectedOrder {
		if results[i].ID != expected {
			t.Errorf("position %d: expected %s, got %s", i, expected, results[i].ID)
		}
	}
}

// TestListWithBlockedInfo_StatusFilterSorting tests sorting with status filter
func TestListWithBlockedInfo_StatusFilterSorting(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	defer database.Close()

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339Nano)
	_, err = database.Exec(`
		INSERT INTO projects (id, name, created_at, updated_at)
		VALUES (?, ?, ?, ?)`, "proj", "Project", nowStr, nowStr)
	if err != nil {
		t.Fatalf("failed to seed project: %v", err)
	}

	// Create tickets with different statuses
	tickets := []struct {
		id        string
		status    string
		updatedAt time.Time
	}{
		{"OPEN-1", "open", now.Add(-2 * time.Hour)},
		{"OPEN-2", "open", now.Add(-10 * time.Minute)}, // Most recently updated
		{"PROG-1", "in_progress", now.Add(-5 * time.Minute)},
		{"OPEN-3", "open", now.Add(-1 * time.Hour)},
	}

	for _, tc := range tickets {
		createdStr := now.Add(-3 * time.Hour).Format(time.RFC3339Nano)
		updatedStr := tc.updatedAt.Format(time.RFC3339Nano)
		_, err = database.Exec(`
			INSERT INTO tickets (id, project_id, title, description, status, priority, issue_type,
				created_at, updated_at, created_by)
			VALUES (?, ?, ?, '', ?, 2, 'task', ?, ?, 'tester')`,
			tc.id, "proj", tc.id, tc.status, createdStr, updatedStr)
		if err != nil {
			t.Fatalf("failed to insert ticket %s: %v", tc.id, err)
		}
	}

	ticketRepo := repo.NewTicketRepo(database, clock.Real())
	filter := &repo.ListFilter{
		ProjectID: "proj",
		Status:    "open",
	}

	paginated, err := ticketRepo.ListWithBlockedInfo(filter)
	if err != nil {
		t.Fatalf("ListWithBlockedInfo failed: %v", err)
	}
	results := paginated.Tickets

	if len(results) != 3 {
		t.Fatalf("expected 3 open tickets, got %d", len(results))
	}

	// Expected order by updated_at DESC
	expectedOrder := []string{"OPEN-2", "OPEN-3", "OPEN-1"}
	for i, expected := range expectedOrder {
		if results[i].ID != expected {
			t.Errorf("position %d: expected %s, got %s", i, expected, results[i].ID)
		}
	}
}

// TestListWithBlockedInfo_ViaHTTP_E2E tests sorting via HTTP API (end-to-end)
func TestListWithBlockedInfo_ViaHTTP_E2E(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProject(t, dbPath, "httptest")
	baseURL := startAPIServer(t, dbPath)

	// Create tickets via HTTP API (nano-precision timestamps ensure different ordering)
	tickets := []struct {
		id    string
		title string
	}{
		{"HTTP-1", "First"},
		{"HTTP-2", "Second"},
		{"HTTP-3", "Third"},
	}

	for _, tc := range tickets {
		body := fmt.Sprintf(`{"id":"%s","title":"%s","created_by":"tester"}`, tc.id, tc.title)
		req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Project", "httptest")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("failed to create ticket %s: %v", tc.id, err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("failed to create ticket %s: status %d", tc.id, resp.StatusCode)
		}
	}

	// Update HTTP-1 to make it most recently updated (after HTTP-3 was created)
	updateBody := `{"title":"First - Updated"}`
	req, _ := http.NewRequest("PATCH", baseURL+"/api/v1/tickets/http-1", bytes.NewBufferString(updateBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "httptest")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to update ticket: %v", err)
	}
	resp.Body.Close()

	// List tickets with status filter
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/tickets?status=open", nil)
	req.Header.Set("X-Project", "httptest")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to list tickets: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(respBody))
	}

	var apiResp struct {
		Tickets    []*repo.PendingTicket `json:"tickets"`
		TotalCount int                   `json:"total_count"`
		Page       int                   `json:"page"`
		PerPage    int                   `json:"per_page"`
		TotalPages int                   `json:"total_pages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	results := apiResp.Tickets

	if len(results) != 3 {
		t.Fatalf("expected 3 tickets, got %d", len(results))
	}

	// Expected order: HTTP-1 (recently updated), HTTP-3 (newest created), HTTP-2 (middle created)
	expectedOrder := []string{"http-1", "http-3", "http-2"}
	for i, expected := range expectedOrder {
		if results[i].ID != expected {
			t.Errorf("position %d: expected %s, got %s (title: %s, updated: %s)",
				i, expected, results[i].ID, results[i].Title, results[i].UpdatedAt.Format(time.RFC3339Nano))
		}
	}
}

// TestListWithBlockedInfo_SameTimestamps tests sorting when multiple tickets have same updated_at
func TestListWithBlockedInfo_SameTimestamps(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	defer database.Close()

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339Nano)
	_, err = database.Exec(`
		INSERT INTO projects (id, name, created_at, updated_at)
		VALUES (?, ?, ?, ?)`, "proj", "Project", nowStr, nowStr)
	if err != nil {
		t.Fatalf("failed to seed project: %v", err)
	}

	// Create tickets with same updated_at but different created_at
	sameUpdateTime := now.Add(-1 * time.Hour)
	tickets := []struct {
		id        string
		createdAt time.Time
	}{
		{"SAME-3", now.Add(-30 * time.Minute)}, // Newest created (should be first among same updated_at)
		{"SAME-1", now.Add(-90 * time.Minute)}, // Oldest created
		{"SAME-2", now.Add(-60 * time.Minute)}, // Middle created
	}

	for _, tc := range tickets {
		createdStr := tc.createdAt.Format(time.RFC3339Nano)
		updatedStr := sameUpdateTime.Format(time.RFC3339Nano)
		_, err = database.Exec(`
			INSERT INTO tickets (id, project_id, title, description, status, priority, issue_type,
				created_at, updated_at, created_by)
			VALUES (?, ?, ?, '', 'open', 2, 'task', ?, ?, 'tester')`,
			tc.id, "proj", tc.id, createdStr, updatedStr)
		if err != nil {
			t.Fatalf("failed to insert ticket %s: %v", tc.id, err)
		}
	}

	ticketRepo := repo.NewTicketRepo(database, clock.Real())
	filter := &repo.ListFilter{
		ProjectID: "proj",
		Status:    "open",
	}

	paginated, err := ticketRepo.ListWithBlockedInfo(filter)
	if err != nil {
		t.Fatalf("ListWithBlockedInfo failed: %v", err)
	}
	results := paginated.Tickets

	if len(results) != 3 {
		t.Fatalf("expected 3 tickets, got %d", len(results))
	}

	// When updated_at is same, should sort by created_at DESC
	expectedOrder := []string{"SAME-3", "SAME-2", "SAME-1"}
	for i, expected := range expectedOrder {
		if results[i].ID != expected {
			t.Errorf("position %d: expected %s, got %s", i, expected, results[i].ID)
		}
	}
}

// ptrTime returns a pointer to a time.Time value
func ptrTime(t time.Time) *time.Time {
	return &t
}
