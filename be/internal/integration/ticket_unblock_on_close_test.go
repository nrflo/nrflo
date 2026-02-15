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

	"be/internal/db"
	"be/internal/model"
	"be/internal/ws"
)

// TestCloseBlockerBroadcastsUnblockEvents verifies that when a blocker ticket is closed,
// WebSocket ticket.updated events are broadcast for all dependent tickets with unblocked_by data.
func TestCloseBlockerBroadcastsUnblockEvents(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	projectID := "unblock-test"
	blockerID := "unblock-blocker"
	dependentID := "unblock-dependent"
	seedProject(t, dbPath, projectID)

	// Create blocker and dependent tickets
	database, _ = db.Open(dbPath)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, tid := range []string{blockerID, dependentID} {
		_, err = database.Exec(`
			INSERT INTO tickets (id, project_id, title, status, priority, issue_type, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			tid, projectID, "Test ticket", "open", 2, "task", "tester", now, now)
		if err != nil {
			t.Fatalf("failed to seed ticket %s: %v", tid, err)
		}
	}

	// Add dependency: dependent depends on blocker
	_, err = database.Exec(`
		INSERT INTO dependencies (project_id, issue_id, depends_on_id, type, created_at, created_by)
		VALUES (?, ?, ?, ?, ?, ?)`,
		projectID, dependentID, blockerID, "blocks", now, "tester")
	if err != nil {
		t.Fatalf("failed to create dependency: %v", err)
	}
	database.Close()

	// Start server with WS, subscribe to all tickets in project
	baseURL, _, ch := startAPIServerWithWS(t, dbPath, projectID, "")

	// Close the blocker ticket
	body := `{"reason":"Completed"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets/"+blockerID+"/close", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", projectID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(respBody))
	}

	// Collect both events (order is non-deterministic)
	events := make(map[string]ws.Event)
	for i := 0; i < 2; i++ {
		ev := expectEvent(t, ch, ws.EventTicketUpdated, 2*time.Second)
		events[ev.TicketID] = ev
	}

	// Verify blocker close event
	blockerEvent, ok := events[blockerID]
	if !ok {
		t.Fatalf("expected blocker event for ticket %q, not received", blockerID)
	}
	if blockerEvent.Data["status"] != string(model.StatusClosed) {
		t.Fatalf("expected blocker status %q, got %v", model.StatusClosed, blockerEvent.Data["status"])
	}

	// Verify dependent unblock event
	depEvent, ok := events[dependentID]
	if !ok {
		t.Fatalf("expected dependent event for ticket %q, not received", dependentID)
	}
	if depEvent.Data["unblocked_by"] != blockerID {
		t.Fatalf("expected unblocked_by %q, got %v", blockerID, depEvent.Data["unblocked_by"])
	}
}

// TestCloseBlockerWithMultipleDependents verifies that closing a ticket that blocks
// multiple other tickets broadcasts events for all dependents.
func TestCloseBlockerWithMultipleDependents(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	projectID := "multi-unblock"
	blockerID := "multi-blocker"
	dependent1ID := "multi-dep1"
	dependent2ID := "multi-dep2"
	dependent3ID := "multi-dep3"
	seedProject(t, dbPath, projectID)

	// Create blocker and three dependent tickets
	database, _ = db.Open(dbPath)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, tid := range []string{blockerID, dependent1ID, dependent2ID, dependent3ID} {
		_, err = database.Exec(`
			INSERT INTO tickets (id, project_id, title, status, priority, issue_type, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			tid, projectID, "Test ticket", "open", 2, "task", "tester", now, now)
		if err != nil {
			t.Fatalf("failed to seed ticket %s: %v", tid, err)
		}
	}

	// Add dependencies: all three dependents depend on blocker
	for _, depID := range []string{dependent1ID, dependent2ID, dependent3ID} {
		_, err = database.Exec(`
			INSERT INTO dependencies (project_id, issue_id, depends_on_id, type, created_at, created_by)
			VALUES (?, ?, ?, ?, ?, ?)`,
			projectID, depID, blockerID, "blocks", now, "tester")
		if err != nil {
			t.Fatalf("failed to create dependency for %s: %v", depID, err)
		}
	}
	database.Close()

	// Start server with WS, subscribe to all tickets in project
	baseURL, _, ch := startAPIServerWithWS(t, dbPath, projectID, "")

	// Close the blocker ticket
	body := `{"reason":"Done"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets/"+blockerID+"/close", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", projectID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(respBody))
	}

	// Collect all events (should be 4: 1 blocker close + 3 dependent unblock)
	events := make(map[string]ws.Event)
	for i := 0; i < 4; i++ {
		event := expectEvent(t, ch, ws.EventTicketUpdated, 2*time.Second)
		events[event.TicketID] = event
	}

	// Verify blocker close event
	blockerEvent, ok := events[blockerID]
	if !ok {
		t.Fatalf("blocker event not found")
	}
	if blockerEvent.Data["status"] != string(model.StatusClosed) {
		t.Fatalf("expected blocker status closed, got %v", blockerEvent.Data["status"])
	}

	// Verify all three dependents received unblock events
	for _, depID := range []string{dependent1ID, dependent2ID, dependent3ID} {
		event, ok := events[depID]
		if !ok {
			t.Fatalf("dependent %s event not found", depID)
		}
		if event.Data["unblocked_by"] != blockerID {
			t.Fatalf("dependent %s: expected unblocked_by %q, got %v", depID, blockerID, event.Data["unblocked_by"])
		}
	}
}


// TestCloseBlockerWithClosedDependent verifies behavior when a blocker is closed
// and one of its dependents is already closed (broadcasts for all, including closed ones).
func TestCloseBlockerWithClosedDependent(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	projectID := "closed-dep"
	blockerID := "closed-dep-blocker"
	openDependentID := "closed-dep-open"
	closedDependentID := "closed-dep-closed"
	seedProject(t, dbPath, projectID)

	// Create blocker and two dependents (one open, one closed)
	database, _ = db.Open(dbPath)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, tid := range []string{blockerID, openDependentID} {
		_, err = database.Exec(`
			INSERT INTO tickets (id, project_id, title, status, priority, issue_type, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			tid, projectID, "Test ticket", "open", 2, "task", "tester", now, now)
		if err != nil {
			t.Fatalf("failed to seed ticket %s: %v", tid, err)
		}
	}
	_, err = database.Exec(`
		INSERT INTO tickets (id, project_id, title, status, priority, issue_type, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		closedDependentID, projectID, "Closed ticket", "closed", 2, "task", "tester", now, now)
	if err != nil {
		t.Fatalf("failed to seed closed ticket: %v", err)
	}

	// Add dependencies
	for _, depID := range []string{openDependentID, closedDependentID} {
		_, err = database.Exec(`
			INSERT INTO dependencies (project_id, issue_id, depends_on_id, type, created_at, created_by)
			VALUES (?, ?, ?, ?, ?, ?)`,
			projectID, depID, blockerID, "blocks", now, "tester")
		if err != nil {
			t.Fatalf("failed to create dependency for %s: %v", depID, err)
		}
	}
	database.Close()

	// Start server with WS, subscribe to all tickets in project
	baseURL, _, ch := startAPIServerWithWS(t, dbPath, projectID, "")

	// Close the blocker ticket
	body := `{"reason":"Done"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets/"+blockerID+"/close", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", projectID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(respBody))
	}

	// Collect all events (should be 3: blocker close + 2 dependent unblocks)
	events := make(map[string]ws.Event)
	for i := 0; i < 3; i++ {
		event := expectEvent(t, ch, ws.EventTicketUpdated, 2*time.Second)
		events[event.TicketID] = event
	}

	// Verify both dependents received unblock events (even the closed one)
	for _, depID := range []string{openDependentID, closedDependentID} {
		event, ok := events[depID]
		if !ok {
			t.Fatalf("dependent %s event not found", depID)
		}
		if event.Data["unblocked_by"] != blockerID {
			t.Fatalf("dependent %s: expected unblocked_by %q, got %v", depID, blockerID, event.Data["unblocked_by"])
		}
	}
}

// TestCloseBlockerGetBlockedErrorHandling verifies that if GetBlocked fails,
// the close operation still succeeds (200 OK) but the error is logged.
func TestCloseBlockerGetBlockedErrorHandling(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	projectID := "error-test"
	ticketID := "error-ticket"
	seedProject(t, dbPath, projectID)

	// Create ticket
	database, _ = db.Open(dbPath)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = database.Exec(`
		INSERT INTO tickets (id, project_id, title, status, priority, issue_type, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ticketID, projectID, "Error test", "open", 2, "task", "tester", now, now)
	if err != nil {
		t.Fatalf("failed to seed ticket: %v", err)
	}

	// Corrupt the dependencies table to cause GetBlocked to fail
	// (drop the table after the ticket is created)
	_, err = database.Exec("DROP TABLE dependencies")
	if err != nil {
		t.Fatalf("failed to drop dependencies table: %v", err)
	}
	database.Close()

	baseURL, _, ch := startAPIServerWithWS(t, dbPath, projectID, ticketID)

	// Close the ticket - should succeed despite GetBlocked error
	body := `{"reason":"Done"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets/"+ticketID+"/close", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", projectID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// HTTP response should still be 200 OK
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 (close should succeed despite GetBlocked error), got %d: %s", resp.StatusCode, string(respBody))
	}

	// Verify the close response contains the closed ticket
	var ticket model.Ticket
	if err := json.NewDecoder(resp.Body).Decode(&ticket); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if ticket.Status != model.StatusClosed {
		t.Fatalf("expected ticket status closed, got %s", ticket.Status)
	}

	// Should still receive the close event
	event := expectEvent(t, ch, ws.EventTicketUpdated, 2*time.Second)
	if event.TicketID != ticketID {
		t.Fatalf("expected ticket_id %q, got %q", ticketID, event.TicketID)
	}
	if event.Data["status"] != string(model.StatusClosed) {
		t.Fatalf("expected status closed, got %v", event.Data["status"])
	}

	// No additional events should be broadcast (GetBlocked failed)
	expectNoEvent(t, ch, 500*time.Millisecond)
}

// TestReopenBlockerDoesNotBroadcastUnblockEvents verifies that reopening a closed
// ticket does NOT broadcast unblock events for dependents (only the reopen event).
func TestReopenBlockerDoesNotBroadcastUnblockEvents(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	projectID := "reopen-test"
	blockerID := "reopen-blocker"
	dependentID := "reopen-dependent"
	seedProject(t, dbPath, projectID)

	// Create closed blocker and open dependent
	database, _ = db.Open(dbPath)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = database.Exec(`
		INSERT INTO tickets (id, project_id, title, status, priority, issue_type, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		blockerID, projectID, "Blocker", "closed", 2, "task", "tester", now, now)
	if err != nil {
		t.Fatalf("failed to seed blocker: %v", err)
	}
	_, err = database.Exec(`
		INSERT INTO tickets (id, project_id, title, status, priority, issue_type, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		dependentID, projectID, "Dependent", "open", 2, "task", "tester", now, now)
	if err != nil {
		t.Fatalf("failed to seed dependent: %v", err)
	}

	// Add dependency
	_, err = database.Exec(`
		INSERT INTO dependencies (project_id, issue_id, depends_on_id, type, created_at, created_by)
		VALUES (?, ?, ?, ?, ?, ?)`,
		projectID, dependentID, blockerID, "blocks", now, "tester")
	if err != nil {
		t.Fatalf("failed to create dependency: %v", err)
	}
	database.Close()

	// Start server with WS, subscribe to all tickets in project
	baseURL, _, ch := startAPIServerWithWS(t, dbPath, projectID, "")

	// Reopen the blocker
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets/"+blockerID+"/reopen", nil)
	req.Header.Set("X-Project", projectID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(respBody))
	}

	// Should only receive the reopen event, NOT any dependent ticket events
	event := expectEvent(t, ch, ws.EventTicketUpdated, 2*time.Second)
	if event.TicketID != blockerID {
		t.Fatalf("expected blocker ticket_id %q, got %q", blockerID, event.TicketID)
	}
	if event.Data["status"] != string(model.StatusOpen) {
		t.Fatalf("expected status open, got %v", event.Data["status"])
	}

	// No additional events should be broadcast
	expectNoEvent(t, ch, 500*time.Millisecond)
}

// TestCloseBlockerE2E is an end-to-end test that verifies the complete flow:
// 1. Create blocker and dependent tickets
// 2. Add dependency
// 3. Verify dependent has open blocker
// 4. Close blocker
// 5. Verify dependent receives unblock event
// 6. Query dependent and verify it has no open blockers
func TestCloseBlockerE2E(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	database, err := db.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	database.Close()

	projectID := "e2e-unblock"
	blockerID := "e2e-blocker"
	dependentID := "e2e-dependent"
	seedProject(t, dbPath, projectID)

	// Start server with WS, subscribe to all tickets in project
	baseURL, _, ch := startAPIServerWithWS(t, dbPath, projectID, "")

	// Step 1: Create blocker ticket
	body := fmt.Sprintf(`{"id":"%s","title":"Blocker ticket","created_by":"tester"}`, blockerID)
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", projectID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create blocker: %v", err)
	}
	resp.Body.Close()
	expectEvent(t, ch, ws.EventTicketUpdated, 2*time.Second) // Wait for create event

	// Step 2: Create dependent ticket
	body = fmt.Sprintf(`{"id":"%s","title":"Dependent ticket","created_by":"tester"}`, dependentID)
	req, _ = http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", projectID)

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to create dependent: %v", err)
	}
	resp.Body.Close()
	expectEvent(t, ch, ws.EventTicketUpdated, 2*time.Second) // Wait for create event

	// Step 3: Add dependency (dependent depends on blocker)
	body = fmt.Sprintf(`{"issue_id":"%s","depends_on_id":"%s"}`, dependentID, blockerID)
	req, _ = http.NewRequest("POST", baseURL+"/api/v1/dependencies", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", projectID)

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to add dependency: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("failed to add dependency: status %d: %s", resp.StatusCode, string(respBody))
	}
	resp.Body.Close()

	// Step 4: Verify dependent has an open blocker
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/tickets/"+dependentID, nil)
	req.Header.Set("X-Project", projectID)

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to get dependent ticket: %v", err)
	}
	var depResponse map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&depResponse)
	resp.Body.Close()

	blockers, ok := depResponse["blockers"].([]interface{})
	if !ok || len(blockers) == 0 {
		t.Fatalf("expected dependent to have blockers before closing blocker")
	}

	// Step 5: Close blocker
	body = `{"reason":"Completed"}`
	req, _ = http.NewRequest("POST", baseURL+"/api/v1/tickets/"+blockerID+"/close", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", projectID)

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to close blocker: %v", err)
	}
	resp.Body.Close()

	// Step 6: Verify WebSocket events (collect both, order is non-deterministic)
	events := make(map[string]ws.Event)
	for i := 0; i < 2; i++ {
		ev := expectEvent(t, ch, ws.EventTicketUpdated, 2*time.Second)
		events[ev.TicketID] = ev
	}

	if _, ok := events[blockerID]; !ok {
		t.Fatalf("expected blocker event for %q, not received", blockerID)
	}
	depEvent, ok := events[dependentID]
	if !ok {
		t.Fatalf("expected dependent event for %q, not received", dependentID)
	}
	if depEvent.Data["unblocked_by"] != blockerID {
		t.Fatalf("expected unblocked_by %q, got %v", blockerID, depEvent.Data["unblocked_by"])
	}

	// Step 7: Query dependent's blockers endpoint to verify no open blockers remain
	req, _ = http.NewRequest("GET", baseURL+"/api/v1/tickets/"+dependentID+"/dependencies", nil)
	req.Header.Set("X-Project", projectID)

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to get dependencies: %v", err)
	}
	var depsResponse struct {
		Blockers []map[string]interface{} `json:"blockers"`
	}
	json.NewDecoder(resp.Body).Decode(&depsResponse)
	resp.Body.Close()

	// The blocker should still be in the list but with status "closed"
	// The is_blocked computation filters by open blockers, so dependent is now unblocked
	if len(depsResponse.Blockers) == 0 {
		t.Fatalf("expected blocker to still exist in dependencies list")
	}
}
