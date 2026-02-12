package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"testing"

	"be/internal/db"
)

// ticketResponse is used to decode ticket JSON responses with nullable fields.
type ticketResponse struct {
	ID          string  `json:"id"`
	ProjectID   string  `json:"project_id"`
	Title       string  `json:"title"`
	Status      string  `json:"status"`
	ClosedAt    *string `json:"closed_at"`
	CloseReason *string `json:"close_reason"`
}

func setupReopenTest(t *testing.T) (baseURL string) {
	t.Helper()
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	seedProject(t, dbPath, "reopen")
	return startAPIServer(t, dbPath)
}

// createTicket creates a ticket and returns the decoded response.
func createTicket(t *testing.T, baseURL, project, id, title string) ticketResponse {
	t.Helper()
	body := `{"id":"` + id + `","title":"` + title + `","created_by":"tester"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", project)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create ticket request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(respBody))
	}

	var ticket ticketResponse
	if err := json.NewDecoder(resp.Body).Decode(&ticket); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	return ticket
}

// closeTicket closes a ticket with an optional reason.
func closeTicket(t *testing.T, baseURL, project, id, reason string) ticketResponse {
	t.Helper()
	var body string
	if reason != "" {
		body = `{"reason":"` + reason + `"}`
	}
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets/"+id+"/close", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", project)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("close ticket request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 on close, got %d: %s", resp.StatusCode, string(respBody))
	}

	var ticket ticketResponse
	if err := json.NewDecoder(resp.Body).Decode(&ticket); err != nil {
		t.Fatalf("failed to decode close response: %v", err)
	}
	return ticket
}

// reopenTicket sends POST /reopen and returns status code + decoded body.
func reopenTicket(t *testing.T, baseURL, project, id string) (int, ticketResponse) {
	t.Helper()
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets/"+id+"/reopen", nil)
	req.Header.Set("X-Project", project)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("reopen ticket request failed: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var ticket ticketResponse
	json.Unmarshal(respBody, &ticket)
	return resp.StatusCode, ticket
}

func TestReopenTicket(t *testing.T) {
	baseURL := setupReopenTest(t)

	// 1. Create a ticket
	created := createTicket(t, baseURL, "reopen", "REOPEN-001", "Reopen test ticket")
	if created.Status != "open" {
		t.Fatalf("expected status 'open' after create, got %q", created.Status)
	}

	// 2. Close it with a reason
	closed := closeTicket(t, baseURL, "reopen", created.ID, "done")
	if closed.Status != "closed" {
		t.Fatalf("expected status 'closed' after close, got %q", closed.Status)
	}
	if closed.ClosedAt == nil {
		t.Fatal("expected closed_at to be set after close")
	}
	if closed.CloseReason == nil || *closed.CloseReason != "done" {
		t.Fatalf("expected close_reason 'done', got %v", closed.CloseReason)
	}

	// 3. Reopen it
	status, reopened := reopenTicket(t, baseURL, "reopen", created.ID)
	if status != http.StatusOK {
		t.Fatalf("expected 200 on reopen, got %d", status)
	}
	if reopened.Status != "open" {
		t.Fatalf("expected status 'open' after reopen, got %q", reopened.Status)
	}
	if reopened.ClosedAt != nil {
		t.Fatalf("expected closed_at to be null after reopen, got %v", *reopened.ClosedAt)
	}
	if reopened.CloseReason != nil {
		t.Fatalf("expected close_reason to be null after reopen, got %v", *reopened.CloseReason)
	}
}

func TestReopenTicketNotFound(t *testing.T) {
	baseURL := setupReopenTest(t)

	status, _ := reopenTicket(t, baseURL, "reopen", "NONEXISTENT-999")
	if status != http.StatusNotFound {
		t.Fatalf("expected 404 for non-existent ticket, got %d", status)
	}
}

func TestReopenOpenTicket(t *testing.T) {
	baseURL := setupReopenTest(t)

	// Create a ticket (starts as open)
	created := createTicket(t, baseURL, "reopen", "REOPEN-002", "Already open ticket")

	// Reopen an already-open ticket — should be idempotent
	status, reopened := reopenTicket(t, baseURL, "reopen", created.ID)
	if status != http.StatusOK {
		t.Fatalf("expected 200 on reopening open ticket, got %d", status)
	}
	if reopened.Status != "open" {
		t.Fatalf("expected status 'open', got %q", reopened.Status)
	}
	if reopened.ClosedAt != nil {
		t.Fatalf("expected closed_at to remain null, got %v", *reopened.ClosedAt)
	}
	if reopened.CloseReason != nil {
		t.Fatalf("expected close_reason to remain null, got %v", *reopened.CloseReason)
	}
}

func TestReopenTicketClearsCloseReason(t *testing.T) {
	baseURL := setupReopenTest(t)

	// Create, close with reason, reopen, verify reason is cleared
	created := createTicket(t, baseURL, "reopen", "REOPEN-003", "Reason clearing test")
	closeTicket(t, baseURL, "reopen", created.ID, "completed implementation")

	status, reopened := reopenTicket(t, baseURL, "reopen", created.ID)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if reopened.CloseReason != nil {
		t.Fatalf("expected close_reason to be null after reopen, got %q", *reopened.CloseReason)
	}
	if reopened.ClosedAt != nil {
		t.Fatalf("expected closed_at to be null after reopen, got %v", *reopened.ClosedAt)
	}
}

func TestReopenTicketMissingProject(t *testing.T) {
	baseURL := setupReopenTest(t)

	// Create a ticket first
	created := createTicket(t, baseURL, "reopen", "REOPEN-004", "Missing project test")

	// Try to reopen without X-Project header
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets/"+created.ID+"/reopen", nil)
	// No X-Project header

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 without project header, got %d", resp.StatusCode)
	}
}
