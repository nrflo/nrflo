package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"testing"

	"be/internal/repo"
)

// paginationListResponse mirrors the JSON shape returned by handleListTickets.
type paginationListResponse struct {
	Tickets    []*repo.PendingTicket `json:"tickets"`
	TotalCount int                   `json:"total_count"`
	Page       int                   `json:"page"`
	PerPage    int                   `json:"per_page"`
	TotalPages int                   `json:"total_pages"`
}

// doListTickets sends GET /api/v1/tickets with the given query string.
func doListTickets(t *testing.T, baseURL, project, query string) *paginationListResponse {
	t.Helper()
	url := baseURL + "/api/v1/tickets"
	if query != "" {
		url += "?" + query
	}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("X-Project", project)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET tickets: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}
	var result paginationListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return &result
}

// createTicketHTTP creates a ticket via the API with optional priority.
func createTicketHTTP(t *testing.T, baseURL, project, id, title string, priority int) {
	t.Helper()
	var body string
	if priority > 0 {
		body = fmt.Sprintf(`{"id":%q,"title":%q,"created_by":"tester","priority":%d}`, id, title, priority)
	} else {
		body = fmt.Sprintf(`{"id":%q,"title":%q,"created_by":"tester"}`, id, title)
	}
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", project)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create ticket %s: %v", id, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create ticket %s: status %d", id, resp.StatusCode)
	}
}

// TestListTickets_PaginationMetadata_E2E verifies that the handler returns the
// correct pagination envelope fields: total_count, page, per_page, total_pages.
func TestListTickets_PaginationMetadata_E2E(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	seedProject(t, dbPath, "pgmeta")
	baseURL := startAPIServer(t, dbPath)

	// Create 5 tickets.
	for i := 1; i <= 5; i++ {
		createTicketHTTP(t, baseURL, "pgmeta", fmt.Sprintf("META-%d", i), fmt.Sprintf("Ticket %d", i), 0)
	}

	// Request page 1 of 2 per page → expect 2 tickets, total=5, total_pages=3.
	result := doListTickets(t, baseURL, "pgmeta", "page=1&per_page=2")

	if result.TotalCount != 5 {
		t.Errorf("total_count = %d, want 5", result.TotalCount)
	}
	if result.Page != 1 {
		t.Errorf("page = %d, want 1", result.Page)
	}
	if result.PerPage != 2 {
		t.Errorf("per_page = %d, want 2", result.PerPage)
	}
	if result.TotalPages != 3 { // ceil(5/2) = 3
		t.Errorf("total_pages = %d, want 3", result.TotalPages)
	}
	if len(result.Tickets) != 2 {
		t.Errorf("len(tickets) = %d, want 2", len(result.Tickets))
	}
	if result.Tickets == nil {
		t.Error("tickets must not be nil")
	}

	// Page 3 should return the remaining 1 ticket.
	result3 := doListTickets(t, baseURL, "pgmeta", "page=3&per_page=2")
	if result3.TotalCount != 5 {
		t.Errorf("page3 total_count = %d, want 5", result3.TotalCount)
	}
	if len(result3.Tickets) != 1 {
		t.Errorf("page3 len(tickets) = %d, want 1", len(result3.Tickets))
	}

	// Page beyond total: empty tickets array, not nil, correct total.
	result99 := doListTickets(t, baseURL, "pgmeta", "page=99&per_page=2")
	if result99.TotalCount != 5 {
		t.Errorf("page99 total_count = %d, want 5", result99.TotalCount)
	}
	if len(result99.Tickets) != 0 {
		t.Errorf("page99 len(tickets) = %d, want 0", len(result99.Tickets))
	}
	if result99.Tickets == nil {
		t.Error("page99 tickets must not be nil (handler sets empty slice)")
	}
}

// TestListTickets_SortByPriority_E2E verifies sort_by=priority&sort_order=asc via HTTP.
func TestListTickets_SortByPriority_E2E(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	seedProject(t, dbPath, "pgprio")
	baseURL := startAPIServer(t, dbPath)

	// Create tickets with distinct priorities (in non-sorted order).
	createTicketHTTP(t, baseURL, "pgprio", "PGPRIO-1", "Ticket 1", 3)
	createTicketHTTP(t, baseURL, "pgprio", "PGPRIO-2", "Ticket 2", 1)
	createTicketHTTP(t, baseURL, "pgprio", "PGPRIO-3", "Ticket 3", 2)

	result := doListTickets(t, baseURL, "pgprio", "sort_by=priority&sort_order=asc")

	if len(result.Tickets) != 3 {
		t.Fatalf("len(tickets) = %d, want 3", len(result.Tickets))
	}
	// Verify priorities are strictly ascending.
	for i := 1; i < len(result.Tickets); i++ {
		prev := result.Tickets[i-1].Priority
		curr := result.Tickets[i].Priority
		if curr < prev {
			t.Errorf("priority not ascending at pos %d: %d < %d", i, curr, prev)
		}
	}
	// Exact order: pgprio-2(1), pgprio-3(2), pgprio-1(3).
	wantIDs := []string{"pgprio-2", "pgprio-3", "pgprio-1"}
	for i, want := range wantIDs {
		if result.Tickets[i].ID != want {
			t.Errorf("pos %d: got %q, want %q", i, result.Tickets[i].ID, want)
		}
	}

	// Verify sort_order=desc reverses the order.
	resultDesc := doListTickets(t, baseURL, "pgprio", "sort_by=priority&sort_order=desc")
	if len(resultDesc.Tickets) != 3 {
		t.Fatalf("desc len(tickets) = %d, want 3", len(resultDesc.Tickets))
	}
	for i := 1; i < len(resultDesc.Tickets); i++ {
		prev := resultDesc.Tickets[i-1].Priority
		curr := resultDesc.Tickets[i].Priority
		if curr > prev {
			t.Errorf("desc priority not descending at pos %d: %d > %d", i, curr, prev)
		}
	}
}

// TestListTickets_PerPageCapped_E2E verifies per_page > 100 is capped to 100.
func TestListTickets_PerPageCapped_E2E(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	seedProject(t, dbPath, "pgcap")
	baseURL := startAPIServer(t, dbPath)

	createTicketHTTP(t, baseURL, "pgcap", "CAP-1", "Ticket 1", 0)
	createTicketHTTP(t, baseURL, "pgcap", "CAP-2", "Ticket 2", 0)

	// Request per_page=200 — handler must cap it to 100.
	result := doListTickets(t, baseURL, "pgcap", "per_page=200")
	if result.PerPage != 100 {
		t.Errorf("per_page = %d, want 100 (capped from 200)", result.PerPage)
	}
	// All 2 tickets still returned (well within the cap).
	if result.TotalCount != 2 {
		t.Errorf("total_count = %d, want 2", result.TotalCount)
	}

	// Default params (no page/per_page): page=1, per_page=30.
	resultDefault := doListTickets(t, baseURL, "pgcap", "")
	if resultDefault.Page != 1 {
		t.Errorf("default page = %d, want 1", resultDefault.Page)
	}
	if resultDefault.PerPage != 30 {
		t.Errorf("default per_page = %d, want 30", resultDefault.PerPage)
	}
}
