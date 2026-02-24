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
	"be/internal/ws"
)

// insertEpicAndChildrenDB inserts an epic + children directly into the given DB path.
// Children have parent_ticket_id set to epicID.
func insertEpicAndChildrenDB(t *testing.T, dbPath, projectID, epicID string, childIDs []string) {
	t.Helper()
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	defer database.Close()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = database.Exec(`
		INSERT INTO tickets (id, project_id, title, status, issue_type, priority, created_at, updated_at, created_by)
		VALUES (?, ?, ?, 'open', 'epic', 2, ?, ?, 'tester')`,
		epicID, projectID, "Epic ticket", now, now)
	if err != nil {
		t.Fatalf("failed to create epic %s: %v", epicID, err)
	}

	for i, childID := range childIDs {
		ts := time.Now().UTC().Add(time.Duration(i+1) * time.Millisecond).Format(time.RFC3339Nano)
		_, err = database.Exec(`
			INSERT INTO tickets (id, project_id, title, status, issue_type, priority, parent_ticket_id, created_at, updated_at, created_by)
			VALUES (?, ?, ?, 'open', 'task', 2, ?, ?, ?, 'tester')`,
			childID, projectID, childID+" task", epicID, ts, ts)
		if err != nil {
			t.Fatalf("failed to create child %s: %v", childID, err)
		}
	}
}

// doClose issues a POST /close request and returns the response.
func doClose(t *testing.T, baseURL, projectID, ticketID string) *http.Response {
	t.Helper()
	body := `{"reason":"done"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets/"+ticketID+"/close", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", projectID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("close request failed for %s: %v", ticketID, err)
	}
	return resp
}

// getTicketStatus fetches a ticket via GET and returns its status field.
func getTicketStatus(t *testing.T, baseURL, projectID, ticketID string) string {
	t.Helper()
	req, _ := http.NewRequest("GET", baseURL+"/api/v1/tickets/"+ticketID, nil)
	req.Header.Set("X-Project", projectID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET ticket %s failed: %v", ticketID, err)
	}
	defer resp.Body.Close()
	var m map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("failed to decode ticket %s: %v", ticketID, err)
	}
	s, _ := m["status"].(string)
	return s
}

// TestAPICloseLastChild_AutoClosesEpicAndBroadcasts tests the API handler path:
// - Closing first child does NOT auto-close the epic.
// - Closing last child auto-closes the epic and broadcasts EventTicketUpdated for it.
func TestAPICloseLastChild_AutoClosesEpicAndBroadcasts(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	projectID := "api-epic-ac1"
	epicID := "api-epic-ac1-epic"
	child1ID := "api-epic-ac1-c1"
	child2ID := "api-epic-ac1-c2"
	seedProject(t, dbPath, projectID)
	insertEpicAndChildrenDB(t, dbPath, projectID, epicID, []string{child1ID, child2ID})

	// Subscribe to all project events (empty ticketID)
	baseURL, _, ch := startAPIServerWithWS(t, dbPath, projectID, "")

	// --- Close child1 (NOT the last child) ---
	resp1 := doClose(t, baseURL, projectID, child1ID)
	io.Copy(io.Discard, resp1.Body)
	resp1.Body.Close()
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 closing child1, got %d", resp1.StatusCode)
	}

	// Receive child1 close event
	ev := expectEvent(t, ch, ws.EventTicketUpdated, 2*time.Second)
	if ev.TicketID != child1ID {
		t.Errorf("expected child1 event, got ticketID=%s", ev.TicketID)
	}

	// No epic auto-close event (child2 still open)
	expectNoEvent(t, ch, 300*time.Millisecond)

	// Verify epic still open
	if status := getTicketStatus(t, baseURL, projectID, epicID); status != "open" {
		t.Errorf("expected epic open after closing child1, got %q", status)
	}

	// --- Close child2 (the last child) ---
	resp2 := doClose(t, baseURL, projectID, child2ID)
	io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 closing child2, got %d", resp2.StatusCode)
	}

	// Collect 2 EventTicketUpdated events: child2 close + epic auto-close
	events := make(map[string]ws.Event)
	for i := 0; i < 2; i++ {
		ev := expectEvent(t, ch, ws.EventTicketUpdated, 2*time.Second)
		events[ev.TicketID] = ev
	}

	// child2 must be closed
	if _, ok := events[child2ID]; !ok {
		t.Fatalf("expected event for child2, not received (got: %v)", events)
	}
	if events[child2ID].Data["status"] != "closed" {
		t.Errorf("expected child2 status 'closed', got %v", events[child2ID].Data["status"])
	}

	// epic must be auto-closed
	epicEv, ok := events[epicID]
	if !ok {
		t.Fatalf("expected EventTicketUpdated for epic %q, not received (got: %v)", epicID, events)
	}
	if epicEv.Data["status"] != "closed" {
		t.Errorf("expected epic status 'closed' in WS event, got %v", epicEv.Data["status"])
	}

	// Verify epic closed via GET
	if status := getTicketStatus(t, baseURL, projectID, epicID); status != "closed" {
		t.Errorf("expected epic closed via GET, got %q", status)
	}
}

// TestAPICloseChild_NonEpicParentNotAffected verifies that closing a child whose
// parent is not an epic does not trigger auto-close of the parent.
func TestAPICloseChild_NonEpicParentNotAffected(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	projectID := "api-nonepic-ac1"
	parentID := "api-nonepic-parent"
	childID := "api-nonepic-child"
	seedProject(t, dbPath, projectID)

	// Insert parent (task, not epic) + child with parent_ticket_id
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open DB: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = database.Exec(`
		INSERT INTO tickets (id, project_id, title, status, issue_type, priority, created_at, updated_at, created_by)
		VALUES (?, ?, ?, 'open', 'task', 2, ?, ?, 'tester')`,
		parentID, projectID, "Parent task", now, now)
	if err != nil {
		t.Fatalf("failed to create parent: %v", err)
	}
	_, err = database.Exec(`
		INSERT INTO tickets (id, project_id, title, status, issue_type, priority, parent_ticket_id, created_at, updated_at, created_by)
		VALUES (?, ?, ?, 'open', 'task', 2, ?, ?, ?, 'tester')`,
		childID, projectID, "Child task", parentID,
		time.Now().UTC().Add(time.Millisecond).Format(time.RFC3339Nano),
		time.Now().UTC().Add(time.Millisecond).Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("failed to create child: %v", err)
	}
	database.Close()

	baseURL, _, ch := startAPIServerWithWS(t, dbPath, projectID, "")

	resp := doClose(t, baseURL, projectID, childID)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Only 1 event (child close); no parent auto-close
	expectEvent(t, ch, ws.EventTicketUpdated, 2*time.Second)
	expectNoEvent(t, ch, 300*time.Millisecond)

	// Parent must remain open
	if status := getTicketStatus(t, baseURL, projectID, parentID); status != "open" {
		t.Errorf("expected non-epic parent to remain open, got %q", status)
	}
}
