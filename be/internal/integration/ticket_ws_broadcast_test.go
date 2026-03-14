package integration

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"be/internal/api"
	"be/internal/config"
	"be/internal/db"
	"be/internal/model"
	"be/internal/ws"
)

// startAPIServerWithWS creates a test HTTP API server with WebSocket support.
// Returns the base URL, ws client, ws channel, and cleanup function.
func startAPIServerWithWS(t *testing.T, dbPath, projectID, ticketID string) (string, *ws.Client, chan []byte) {
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

	// Create server (it creates its own hub internally)
	srv := api.NewServer(cfg, dbPath, pool)

	// Get the hub from the server
	hub := srv.GetWSHub()

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

	// Create test WS client subscribed to the ticket
	client, ch := ws.NewTestClient(hub, "test-ws-client")
	hub.Register(client)
	hub.Subscribe(client, projectID, ticketID)

	t.Cleanup(func() {
		srv.Stop(nil)
	})

	return baseURL, client, ch
}

func TestTicketCreateBroadcastsWSEvent(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	// Initialize DB
	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	projectID := "ws-test"
	ticketID := "ws-test-create1"
	seedProject(t, dbPath, projectID)
	baseURL, _, ch := startAPIServerWithWS(t, dbPath, projectID, ticketID)

	// Create ticket
	body := fmt.Sprintf(`{"id":"%s","title":"Create broadcast test","created_by":"tester"}`, ticketID)
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", projectID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(respBody))
	}

	// Verify WS event
	event := expectEvent(t, ch, ws.EventTicketUpdated, 2*time.Second)
	if event.ProjectID != projectID {
		t.Fatalf("expected project_id %q, got %q", projectID, event.ProjectID)
	}
	if event.TicketID != ticketID {
		t.Fatalf("expected ticket_id %q, got %q", ticketID, event.TicketID)
	}
	if event.Data["status"] != string(model.StatusOpen) {
		t.Fatalf("expected status %q, got %v", model.StatusOpen, event.Data["status"])
	}
	if event.Data["action"] != "created" {
		t.Fatalf("expected action 'created', got %v", event.Data["action"])
	}
}

func TestTicketUpdateBroadcastsWSEvent(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	projectID := "ws-test"
	ticketID := "ws-test-update1"
	seedProject(t, dbPath, projectID)

	// Create ticket first
	database, err := db.Open(dbPath)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = database.Exec(`
		INSERT INTO tickets (id, project_id, title, status, priority, issue_type, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ticketID, projectID, "Update test", "open", 2, "task", "tester", now, now)
	if err != nil {
		t.Fatalf("failed to seed ticket: %v", err)
	}
	database.Close()

	baseURL, _, ch := startAPIServerWithWS(t, dbPath, projectID, ticketID)

	// Update ticket status
	body := `{"status":"in_progress"}`
	req, _ := http.NewRequest("PATCH", baseURL+"/api/v1/tickets/"+ticketID, bytes.NewBufferString(body))
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

	// Verify WS event
	event := expectEvent(t, ch, ws.EventTicketUpdated, 2*time.Second)
	if event.ProjectID != projectID {
		t.Fatalf("expected project_id %q, got %q", projectID, event.ProjectID)
	}
	if event.TicketID != ticketID {
		t.Fatalf("expected ticket_id %q, got %q", ticketID, event.TicketID)
	}
	if event.Data["status"] != string(model.StatusInProgress) {
		t.Fatalf("expected status %q, got %v", model.StatusInProgress, event.Data["status"])
	}
}

func TestTicketCloseBroadcastsWSEvent(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	projectID := "ws-test"
	ticketID := "ws-test-close1"
	seedProject(t, dbPath, projectID)

	// Create ticket first
	database, err := db.Open(dbPath)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = database.Exec(`
		INSERT INTO tickets (id, project_id, title, status, priority, issue_type, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ticketID, projectID, "Close test", "open", 2, "task", "tester", now, now)
	if err != nil {
		t.Fatalf("failed to seed ticket: %v", err)
	}
	database.Close()

	baseURL, _, ch := startAPIServerWithWS(t, dbPath, projectID, ticketID)

	// Close ticket
	body := `{"reason":"Done"}`
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets/"+ticketID+"/close", bytes.NewBufferString(body))
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

	// Verify WS event
	event := expectEvent(t, ch, ws.EventTicketUpdated, 2*time.Second)
	if event.ProjectID != projectID {
		t.Fatalf("expected project_id %q, got %q", projectID, event.ProjectID)
	}
	if event.TicketID != ticketID {
		t.Fatalf("expected ticket_id %q, got %q", ticketID, event.TicketID)
	}
	if event.Data["status"] != string(model.StatusClosed) {
		t.Fatalf("expected status %q, got %v", model.StatusClosed, event.Data["status"])
	}
}

func TestTicketReopenBroadcastsWSEvent(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	projectID := "ws-test"
	ticketID := "ws-test-reopen1"
	seedProject(t, dbPath, projectID)

	// Create closed ticket
	database, err := db.Open(dbPath)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = database.Exec(`
		INSERT INTO tickets (id, project_id, title, status, priority, issue_type, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ticketID, projectID, "Reopen test", "closed", 2, "task", "tester", now, now)
	if err != nil {
		t.Fatalf("failed to seed ticket: %v", err)
	}
	database.Close()

	baseURL, _, ch := startAPIServerWithWS(t, dbPath, projectID, ticketID)

	// Reopen ticket
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/tickets/"+ticketID+"/reopen", nil)
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

	// Verify WS event
	event := expectEvent(t, ch, ws.EventTicketUpdated, 2*time.Second)
	if event.ProjectID != projectID {
		t.Fatalf("expected project_id %q, got %q", projectID, event.ProjectID)
	}
	if event.TicketID != ticketID {
		t.Fatalf("expected ticket_id %q, got %q", ticketID, event.TicketID)
	}
	if event.Data["status"] != string(model.StatusOpen) {
		t.Fatalf("expected status %q, got %v", model.StatusOpen, event.Data["status"])
	}
}

func TestTicketDeleteBroadcastsWSEvent(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	projectID := "ws-test"
	ticketID := "ws-test-delete1"
	seedProject(t, dbPath, projectID)

	// Create ticket first
	database, err := db.Open(dbPath)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = database.Exec(`
		INSERT INTO tickets (id, project_id, title, status, priority, issue_type, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ticketID, projectID, "Delete test", "open", 2, "task", "tester", now, now)
	if err != nil {
		t.Fatalf("failed to seed ticket: %v", err)
	}
	database.Close()

	baseURL, _, ch := startAPIServerWithWS(t, dbPath, projectID, ticketID)

	// Delete ticket
	req, _ := http.NewRequest("DELETE", baseURL+"/api/v1/tickets/"+ticketID, nil)
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

	// Verify WS event
	event := expectEvent(t, ch, ws.EventTicketUpdated, 2*time.Second)
	if event.ProjectID != projectID {
		t.Fatalf("expected project_id %q, got %q", projectID, event.ProjectID)
	}
	if event.TicketID != ticketID {
		t.Fatalf("expected ticket_id %q, got %q", ticketID, event.TicketID)
	}
	if event.Data["action"] != "deleted" {
		t.Fatalf("expected action 'deleted', got %v", event.Data["action"])
	}
}

func TestTicketUpdateMultipleFieldsBroadcastsWSEvent(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	projectID := "ws-test"
	ticketID := "ws-test-multi1"
	seedProject(t, dbPath, projectID)

	// Create ticket first
	database, err := db.Open(dbPath)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = database.Exec(`
		INSERT INTO tickets (id, project_id, title, status, priority, issue_type, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ticketID, projectID, "Multi field test", "open", 2, "task", "tester", now, now)
	if err != nil {
		t.Fatalf("failed to seed ticket: %v", err)
	}
	database.Close()

	baseURL, _, ch := startAPIServerWithWS(t, dbPath, projectID, ticketID)

	// Update multiple fields including status
	body := `{"title":"Updated title","status":"in_progress","priority":1}`
	req, _ := http.NewRequest("PATCH", baseURL+"/api/v1/tickets/"+ticketID, bytes.NewBufferString(body))
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

	// Verify WS event - should broadcast the new status
	event := expectEvent(t, ch, ws.EventTicketUpdated, 2*time.Second)
	if event.ProjectID != projectID {
		t.Fatalf("expected project_id %q, got %q", projectID, event.ProjectID)
	}
	if event.TicketID != ticketID {
		t.Fatalf("expected ticket_id %q, got %q", ticketID, event.TicketID)
	}
	if event.Data["status"] != string(model.StatusInProgress) {
		t.Fatalf("expected status %q, got %v", model.StatusInProgress, event.Data["status"])
	}
}

func TestTicketUpdateNoWSHubDoesNotPanic(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	projectID := "ws-test"
	ticketID := "ws-test-nowshub1"
	seedProject(t, dbPath, projectID)

	// Create ticket first
	database, err := db.Open(dbPath)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = database.Exec(`
		INSERT INTO tickets (id, project_id, title, status, priority, issue_type, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ticketID, projectID, "No hub test", "open", 2, "task", "tester", now, now)
	if err != nil {
		t.Fatalf("failed to seed ticket: %v", err)
	}
	database.Close()

	// Start server WITHOUT WS hub (using startAPIServer instead of startAPIServerWithWS)
	baseURL := startAPIServer(t, dbPath)

	// Update ticket - should not panic even without hub
	body := `{"status":"in_progress"}`
	req, _ := http.NewRequest("PATCH", baseURL+"/api/v1/tickets/"+ticketID, bytes.NewBufferString(body))
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

	// If we got here without panicking, the test passes
}

func TestTicketWSEventsSubscriptionFiltering(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	if err := copyTemplateDB(dbPath); err != nil {
		t.Fatalf("failed to copy template DB: %v", err)
	}

	projectID := "ws-test"
	ticket1ID := "ws-test-filter1"
	ticket2ID := "ws-test-filter2"
	seedProject(t, dbPath, projectID)

	// Create both tickets
	database, err := db.Open(dbPath)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, tid := range []string{ticket1ID, ticket2ID} {
		_, err = database.Exec(`
			INSERT INTO tickets (id, project_id, title, status, priority, issue_type, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			tid, projectID, "Filter test", "open", 2, "task", "tester", now, now)
		if err != nil {
			t.Fatalf("failed to seed ticket %s: %v", tid, err)
		}
	}
	database.Close()

	// Create server
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	cfg := config.DefaultConfig()
	cfg.Server.CORSOrigins = []string{"*"}

	pool2, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(func() { pool2.Close() })

	srv := api.NewServer(cfg, dbPath, pool2)
	hub := srv.GetWSHub()

	go func() {
		_ = srv.Start(port)
	}()

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	// Wait for server
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/api/v1/projects")
		if err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Create two WS clients subscribed to different tickets
	client1, ch1 := ws.NewTestClient(hub, "client1")
	hub.Register(client1)
	hub.Subscribe(client1, projectID, ticket1ID)

	client2, ch2 := ws.NewTestClient(hub, "client2")
	hub.Register(client2)
	hub.Subscribe(client2, projectID, ticket2ID)

	t.Cleanup(func() {
		srv.Stop(nil)
	})

	// Update ticket1
	body := `{"status":"in_progress"}`
	req, _ := http.NewRequest("PATCH", baseURL+"/api/v1/tickets/"+ticket1ID, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", projectID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	// Client1 should receive event
	event := expectEvent(t, ch1, ws.EventTicketUpdated, 2*time.Second)
	if event.TicketID != ticket1ID {
		t.Fatalf("client1: expected ticket_id %q, got %q", ticket1ID, event.TicketID)
	}

	// Client2 should NOT receive event
	expectNoEvent(t, ch2, 200*time.Millisecond)

	// Update ticket2
	req, _ = http.NewRequest("PATCH", baseURL+"/api/v1/tickets/"+ticket2ID, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Project", projectID)

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	// Client2 should receive event
	event = expectEvent(t, ch2, ws.EventTicketUpdated, 2*time.Second)
	if event.TicketID != ticket2ID {
		t.Fatalf("client2: expected ticket_id %q, got %q", ticket2ID, event.TicketID)
	}

	// Client1 should NOT receive this event
	expectNoEvent(t, ch1, 200*time.Millisecond)
}
