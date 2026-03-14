package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"be/internal/api"
	"be/internal/config"
	"be/internal/db"
	"be/internal/model"
)

// startAPIServer creates a test HTTP API server backed by the given DB path.
// Returns the base URL and a cleanup function.
func startAPIServer(t *testing.T, dbPath string) string {
	t.Helper()

	// Find a free port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	cfg := config.DefaultConfig()
	cfg.Server.CORSOrigins = []string{"*"}

	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	srv := api.NewServer(cfg, dbPath, pool)

	// Start in background
	go func() {
		_ = srv.Start(port)
	}()

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	// Wait for server to be ready
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/api/v1/projects")
		if err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Cleanup(func() {
		srv.Stop(nil)
	})

	return baseURL
}

// seedProject inserts a project directly into the DB.
func seedProject(t *testing.T, dbPath, projectID string) {
	t.Helper()
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB for seeding: %v", err)
	}
	defer database.Close()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = database.Exec(`
		INSERT INTO projects (id, name, created_at, updated_at)
		VALUES (?, ?, ?, ?)`, projectID, projectID, now, now)
	if err != nil {
		t.Fatalf("failed to seed project: %v", err)
	}
}

func TestCreateTicketWithExplicitID(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	// Initialize DB (migrations run on Open)
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProject(t, dbPath, "myproj")
	baseURL := startAPIServer(t, dbPath)

	body := `{"id":"MYPROJ-001","title":"Explicit ID ticket","created_by":"tester"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "myproj")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(respBody))
	}

	var ticket model.Ticket
	if err := json.NewDecoder(resp.Body).Decode(&ticket); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if ticket.ID != "myproj-001" {
		t.Fatalf("expected ID 'myproj-001' (repo lowercases), got %q", ticket.ID)
	}
	if ticket.Title != "Explicit ID ticket" {
		t.Fatalf("expected title 'Explicit ID ticket', got %q", ticket.Title)
	}
	if ticket.CreatedBy != "tester" {
		t.Fatalf("expected created_by 'tester', got %q", ticket.CreatedBy)
	}
}

func TestCreateTicketAutoGeneratesID(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProject(t, dbPath, "myproj")
	baseURL := startAPIServer(t, dbPath)

	// No "id" field in request body
	body := `{"title":"Auto ID ticket","created_by":"tester"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "myproj")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(respBody))
	}

	var ticket model.Ticket
	if err := json.NewDecoder(resp.Body).Decode(&ticket); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// ID should match pattern {PROJECT}-{6hexchars} (project uppercased, then lowered by generator)
	// The handler does: id.New(strings.ToUpper(projectID)) and generator lowercases prefix
	// So for projectID="myproj", prefix becomes "myproj" (ToUpper→MYPROJ, then New lowercases→myproj)
	pattern := regexp.MustCompile(`^myproj-[0-9a-f]{6}$`)
	if !pattern.MatchString(ticket.ID) {
		t.Fatalf("expected auto-generated ID matching 'myproj-XXXXXX', got %q", ticket.ID)
	}

	if ticket.Title != "Auto ID ticket" {
		t.Fatalf("expected title 'Auto ID ticket', got %q", ticket.Title)
	}
}

func TestCreateTicketAutoIDWithEmptyString(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProject(t, dbPath, "proj")
	baseURL := startAPIServer(t, dbPath)

	// Explicitly pass empty string for id
	body := `{"id":"","title":"Empty ID ticket","created_by":"tester"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "proj")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(respBody))
	}

	var ticket model.Ticket
	if err := json.NewDecoder(resp.Body).Decode(&ticket); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	pattern := regexp.MustCompile(`^proj-[0-9a-f]{6}$`)
	if !pattern.MatchString(ticket.ID) {
		t.Fatalf("expected auto-generated ID matching 'proj-XXXXXX', got %q", ticket.ID)
	}
}

func TestCreateTicketAutoIDUnique(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProject(t, dbPath, "uniq")
	baseURL := startAPIServer(t, dbPath)

	// Create two tickets without IDs, verify they get different IDs
	ids := make(map[string]bool)
	for i := 0; i < 5; i++ {
		body := fmt.Sprintf(`{"title":"Ticket %d","created_by":"tester"}`, i)
		req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Project", "uniq")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}

		if resp.StatusCode != http.StatusCreated {
			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Fatalf("request %d: expected 201, got %d: %s", i, resp.StatusCode, string(respBody))
		}

		var ticket model.Ticket
		if err := json.NewDecoder(resp.Body).Decode(&ticket); err != nil {
			resp.Body.Close()
			t.Fatalf("request %d: failed to decode: %v", i, err)
		}
		resp.Body.Close()

		if ids[ticket.ID] {
			t.Fatalf("duplicate auto-generated ID: %q", ticket.ID)
		}
		ids[ticket.ID] = true
	}

	if len(ids) != 5 {
		t.Fatalf("expected 5 unique IDs, got %d", len(ids))
	}
}

func TestCreateTicketMissingTitle(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProject(t, dbPath, "errproj")
	baseURL := startAPIServer(t, dbPath)

	body := `{"created_by":"tester"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "errproj")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var errResp map[string]string
	json.NewDecoder(resp.Body).Decode(&errResp)
	if errResp["error"] != "title is required" {
		t.Fatalf("expected 'title is required' error, got %q", errResp["error"])
	}
}

func TestCreateTicketMissingCreatedBy(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProject(t, dbPath, "errproj")
	baseURL := startAPIServer(t, dbPath)

	body := `{"title":"No creator"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "errproj")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var errResp map[string]string
	json.NewDecoder(resp.Body).Decode(&errResp)
	if errResp["error"] != "created_by is required" {
		t.Fatalf("expected 'created_by is required' error, got %q", errResp["error"])
	}
}

func TestCreateTicketMissingProject(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	baseURL := startAPIServer(t, dbPath)

	body := `{"title":"No project","created_by":"tester"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	// No X-Project header

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCreateTicketDefaultsApplied(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProject(t, dbPath, "defaults")
	baseURL := startAPIServer(t, dbPath)

	// Minimal request - should get default priority and issue_type
	body := `{"id":"DEF-001","title":"Defaults test","created_by":"tester"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "defaults")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(respBody))
	}

	var ticket model.Ticket
	if err := json.NewDecoder(resp.Body).Decode(&ticket); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if ticket.Priority != 2 {
		t.Fatalf("expected default priority 2, got %d", ticket.Priority)
	}
	if ticket.IssueType != "task" {
		t.Fatalf("expected default issue_type 'task', got %q", ticket.IssueType)
	}
	if ticket.Status != "open" {
		t.Fatalf("expected status 'open', got %q", ticket.Status)
	}
}

func TestGetEpicTicketWithChildren(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

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
		t.Fatalf("expected 201 for epic, got %d", resp.StatusCode)
	}

	// Create child ticket with higher priority (should come second)
	child1Body := `{"id":"CHILD-001","title":"Child 1","parent_ticket_id":"EPIC-001","priority":3,"created_by":"tester"}`
	req, _ = http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(child1Body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "epic-proj")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create child 1: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for child 1, got %d", resp.StatusCode)
	}

	// Create child ticket with lower priority (should come first)
	child2Body := `{"id":"CHILD-002","title":"Child 2","parent_ticket_id":"EPIC-001","priority":1,"created_by":"tester"}`
	req, _ = http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(child2Body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "epic-proj")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create child 2: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 for child 2, got %d", resp.StatusCode)
	}

	// GET the epic and verify children are returned
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

	var result struct {
		ID       string          `json:"id"`
		Title    string          `json:"title"`
		Children []*model.Ticket `json:"children"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify children array is populated
	if len(result.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(result.Children))
	}

	// Verify ordering: priority ASC, then created_at ASC
	// Child 2 has priority 1 (lower), Child 1 has priority 3 (higher)
	if result.Children[0].ID != "child-002" {
		t.Fatalf("expected first child to be child-002 (priority 1), got %q", result.Children[0].ID)
	}
	if result.Children[1].ID != "child-001" {
		t.Fatalf("expected second child to be child-001 (priority 3), got %q", result.Children[1].ID)
	}

	// Verify child details
	if result.Children[0].Title != "Child 2" {
		t.Fatalf("expected child title 'Child 2', got %q", result.Children[0].Title)
	}
	if result.Children[0].Priority != 1 {
		t.Fatalf("expected child priority 1, got %d", result.Children[0].Priority)
	}
}

func TestGetEpicTicketWithNoChildren(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProject(t, dbPath, "empty-epic")
	baseURL := startAPIServer(t, dbPath)

	// Create an epic with no children
	epicBody := `{"id":"LONELY-EPIC","title":"Epic Without Children","issue_type":"epic","created_by":"tester"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(epicBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "empty-epic")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create epic: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	// GET the epic
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/tickets/LONELY-EPIC", nil)
	req.Header.Set("X-Project", "empty-epic")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to get epic: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID       string          `json:"id"`
		Children []*model.Ticket `json:"children"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify children is empty array, not null
	if result.Children == nil {
		t.Fatalf("expected children to be empty array [], got nil")
	}
	if len(result.Children) != 0 {
		t.Fatalf("expected 0 children, got %d", len(result.Children))
	}
}

func TestGetNonEpicTicketHasEmptyChildrenArray(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProject(t, dbPath, "task-proj")
	baseURL := startAPIServer(t, dbPath)

	// Create a non-epic ticket (task)
	taskBody := `{"id":"TASK-001","title":"Regular Task","issue_type":"task","created_by":"tester"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(taskBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "task-proj")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	// GET the task
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/tickets/TASK-001", nil)
	req.Header.Set("X-Project", "task-proj")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID        string          `json:"id"`
		IssueType string          `json:"issue_type"`
		Children  []*model.Ticket `json:"children"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify children is empty array for non-epic
	if result.Children == nil {
		t.Fatalf("expected children to be empty array [], got nil")
	}
	if len(result.Children) != 0 {
		t.Fatalf("expected 0 children for non-epic, got %d", len(result.Children))
	}
	if result.IssueType != "task" {
		t.Fatalf("expected issue_type 'task', got %q", result.IssueType)
	}
}

func TestGetBugTicketHasEmptyChildrenArray(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProject(t, dbPath, "bug-proj")
	baseURL := startAPIServer(t, dbPath)

	// Create a bug ticket
	bugBody := `{"id":"BUG-001","title":"Bug Ticket","issue_type":"bug","created_by":"tester"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(bugBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "bug-proj")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create bug: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	// GET the bug
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/tickets/BUG-001", nil)
	req.Header.Set("X-Project", "bug-proj")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to get bug: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID        string          `json:"id"`
		IssueType string          `json:"issue_type"`
		Children  []*model.Ticket `json:"children"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify children is empty array for bug
	if result.Children == nil {
		t.Fatalf("expected children to be empty array [], got nil")
	}
	if len(result.Children) != 0 {
		t.Fatalf("expected 0 children for bug, got %d", len(result.Children))
	}
	if result.IssueType != "bug" {
		t.Fatalf("expected issue_type 'bug', got %q", result.IssueType)
	}
}

func TestGetEpicChildrenOrderedByPriorityThenCreatedAt(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProject(t, dbPath, "order-proj")
	baseURL := startAPIServer(t, dbPath)

	// Create epic
	epicBody := `{"id":"ORDER-EPIC","title":"Test Ordering","issue_type":"epic","created_by":"tester"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(epicBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "order-proj")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create epic: %v", err)
	}
	resp.Body.Close()

	// Create child with priority 2, created first
	child1Body := `{"id":"C1","title":"Priority 2 First","parent_ticket_id":"ORDER-EPIC","priority":2,"created_by":"tester"}`
	req, _ = http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(child1Body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "order-proj")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create C1: %v", err)
	}
	resp.Body.Close()

	time.Sleep(10 * time.Millisecond)

	// Create child with priority 2, created second (same priority, later timestamp)
	child2Body := `{"id":"C2","title":"Priority 2 Second","parent_ticket_id":"ORDER-EPIC","priority":2,"created_by":"tester"}`
	req, _ = http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(child2Body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "order-proj")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create C2: %v", err)
	}
	resp.Body.Close()

	time.Sleep(10 * time.Millisecond)

	// Create child with priority 1 (should come first despite later creation)
	child3Body := `{"id":"C3","title":"Priority 1","parent_ticket_id":"ORDER-EPIC","priority":1,"created_by":"tester"}`
	req, _ = http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(child3Body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "order-proj")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create C3: %v", err)
	}
	resp.Body.Close()

	time.Sleep(10 * time.Millisecond)

	// Create child with priority 3 (should come last)
	child4Body := `{"id":"C4","title":"Priority 3","parent_ticket_id":"ORDER-EPIC","priority":3,"created_by":"tester"}`
	req, _ = http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(child4Body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "order-proj")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create C4: %v", err)
	}
	resp.Body.Close()

	// GET epic and verify order
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/tickets/ORDER-EPIC", nil)
	req.Header.Set("X-Project", "order-proj")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to get epic: %v", err)
	}
	defer resp.Body.Close()

	var result struct {
		Children []*model.Ticket `json:"children"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(result.Children) != 4 {
		t.Fatalf("expected 4 children, got %d", len(result.Children))
	}

	// Expected order: C3 (pri 1), C1 (pri 2, earlier), C2 (pri 2, later), C4 (pri 3)
	expectedOrder := []string{"c3", "c1", "c2", "c4"}
	for i, expected := range expectedOrder {
		if result.Children[i].ID != expected {
			t.Errorf("position %d: expected %s, got %s", i, expected, result.Children[i].ID)
		}
	}
}

func TestGetEpicCaseInsensitiveParentLookup(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	seedProject(t, dbPath, "case-proj")
	baseURL := startAPIServer(t, dbPath)

	// Create epic with mixed case
	epicBody := `{"id":"MixedCase-Epic","title":"Case Test","issue_type":"epic","created_by":"tester"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(epicBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "case-proj")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create epic: %v", err)
	}
	resp.Body.Close()

	// Create child with different case in parent_ticket_id
	childBody := `{"id":"CHILD","title":"Child","parent_ticket_id":"MIXEDCASE-EPIC","created_by":"tester"}`
	req, _ = http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(childBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", "case-proj")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create child: %v", err)
	}
	resp.Body.Close()

	// GET epic using different case
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/tickets/mixedcase-epic", nil)
	req.Header.Set("X-Project", "case-proj")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to get epic: %v", err)
	}
	defer resp.Body.Close()

	var result struct {
		Children []*model.Ticket `json:"children"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should find the child despite case differences
	if len(result.Children) != 1 {
		t.Fatalf("expected 1 child (case-insensitive), got %d", len(result.Children))
	}
	if result.Children[0].ID != "child" {
		t.Fatalf("expected child ID 'child', got %q", result.Children[0].ID)
	}
}
