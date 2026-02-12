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

	srv := api.NewServer(cfg, dbPath)

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

	now := time.Now().UTC().Format(time.RFC3339)
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
	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

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

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

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

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

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

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

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

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

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

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

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

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

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

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

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
