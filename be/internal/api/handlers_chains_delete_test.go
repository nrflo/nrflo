package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/ws"
)

// newDeleteChainServer creates a minimal Server for handleDeleteChain tests.
func newDeleteChainServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "del_chain_test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return &Server{pool: pool, clock: clock.Real()}
}

// seedChainRow inserts a project (if not exists) and a chain_executions row.
func seedChainRow(t *testing.T, s *Server, projectID, chainID, status string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.pool.Exec(
		`INSERT OR IGNORE INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'Test', '/tmp', ?, ?)`,
		projectID, now, now,
	); err != nil {
		t.Fatalf("seed project %q: %v", projectID, err)
	}
	if _, err := s.pool.Exec(`
		INSERT INTO chain_executions (id, project_id, name, status, workflow_name, created_by, created_at, updated_at)
		VALUES (?, ?, 'Test Chain', ?, 'feature', 'test', ?, ?)`,
		chainID, projectID, status, now, now,
	); err != nil {
		t.Fatalf("seed chain %q: %v", chainID, err)
	}
}

// deleteChainHTTPReq builds a DELETE request for DELETE /api/v1/chains/{id}.
// Passes the project as ?project= query param (bypasses middleware, like other handler tests).
func deleteChainHTTPReq(t *testing.T, projectID, chainID string) *http.Request {
	t.Helper()
	url := "/api/v1/chains/" + chainID
	if projectID != "" {
		url = withProject(url, projectID)
	}
	req := httptest.NewRequest(http.MethodDelete, url, nil)
	req.SetPathValue("id", chainID)
	return req
}

// TestHandleDeleteChain_StatusCases checks 204 for deletable statuses, 409 for running.
func TestHandleDeleteChain_StatusCases(t *testing.T) {
	cases := []struct {
		status     string
		wantStatus int
	}{
		{"pending", http.StatusOK},
		{"completed", http.StatusOK},
		{"failed", http.StatusOK},
		{"canceled", http.StatusOK},
		{"running", http.StatusConflict},
	}
	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			s := newDeleteChainServer(t)
			projID := "proj-del-" + tc.status
			chainID := "chain-del-" + tc.status
			seedChainRow(t, s, projID, chainID, tc.status)

			rr := httptest.NewRecorder()
			s.handleDeleteChain(rr, deleteChainHTTPReq(t, projID, chainID))

			if rr.Code != tc.wantStatus {
				t.Errorf("status(%q) = %d, want %d; body: %s", tc.status, rr.Code, tc.wantStatus, rr.Body.String())
			}
		})
	}
}

// TestHandleDeleteChain_NotFound verifies 404 when chain doesn't exist.
func TestHandleDeleteChain_NotFound(t *testing.T) {
	s := newDeleteChainServer(t)

	rr := httptest.NewRecorder()
	s.handleDeleteChain(rr, deleteChainHTTPReq(t, "proj-nf", "no-such-chain"))

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body: %s", rr.Code, rr.Body.String())
	}
	assertErrorContains(t, rr, "not found")
}

// TestHandleDeleteChain_WrongProject verifies 404 when chain belongs to a different project.
func TestHandleDeleteChain_WrongProject(t *testing.T) {
	s := newDeleteChainServer(t)
	seedChainRow(t, s, "proj-real", "chain-wp", "completed")

	rr := httptest.NewRecorder()
	s.handleDeleteChain(rr, deleteChainHTTPReq(t, "proj-other", "chain-wp"))

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body: %s", rr.Code, rr.Body.String())
	}
}

// TestHandleDeleteChain_MissingProjectHeader verifies 400 when X-Project header is absent.
func TestHandleDeleteChain_MissingProjectHeader(t *testing.T) {
	s := newDeleteChainServer(t)

	rr := httptest.NewRecorder()
	s.handleDeleteChain(rr, deleteChainHTTPReq(t, "", "some-chain"))

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// TestHandleDeleteChain_RunningErrorBody verifies 409 error body contains "running".
func TestHandleDeleteChain_RunningErrorBody(t *testing.T) {
	s := newDeleteChainServer(t)
	seedChainRow(t, s, "proj-run", "chain-run", "running")

	rr := httptest.NewRecorder()
	s.handleDeleteChain(rr, deleteChainHTTPReq(t, "proj-run", "chain-run"))

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rr.Code)
	}
	assertErrorContains(t, rr, "running")
}

// TestHandleDeleteChain_Idempotent verifies second delete of the same chain returns 404.
func TestHandleDeleteChain_Idempotent(t *testing.T) {
	s := newDeleteChainServer(t)
	seedChainRow(t, s, "proj-idem", "chain-idem", "completed")

	rr1 := httptest.NewRecorder()
	s.handleDeleteChain(rr1, deleteChainHTTPReq(t, "proj-idem", "chain-idem"))
	if rr1.Code != http.StatusOK {
		t.Fatalf("first delete status = %d, want 204; body: %s", rr1.Code, rr1.Body.String())
	}

	rr2 := httptest.NewRecorder()
	s.handleDeleteChain(rr2, deleteChainHTTPReq(t, "proj-idem", "chain-idem"))
	if rr2.Code != http.StatusNotFound {
		t.Errorf("second delete status = %d, want 404 (chain gone)", rr2.Code)
	}
}

// TestHandleDeleteChain_WSBroadcast verifies chain.deleted WS event is broadcast on success.
func TestHandleDeleteChain_WSBroadcast(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "del_chain_ws_test.db")
	pool, err := db.NewPoolPath(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	hub := ws.NewHub(clock.Real())
	go hub.Run()
	t.Cleanup(hub.Stop)

	s := &Server{pool: pool, clock: clock.Real(), wsHub: hub}

	client, sendCh := ws.NewTestClient(hub, "ws-chain-client")
	hub.Register(client)
	hub.Subscribe(client, "proj-ws-chain", "")

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES ('proj-ws-chain', 'WS Test', '/tmp', ?, ?)`,
		now, now,
	); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := pool.Exec(`
		INSERT INTO chain_executions (id, project_id, name, status, workflow_name, created_by, created_at, updated_at)
		VALUES ('chain-ws', 'proj-ws-chain', 'Test', 'pending', 'feature', 'test', ?, ?)`,
		now, now,
	); err != nil {
		t.Fatalf("seed chain: %v", err)
	}

	rrWS := httptest.NewRecorder()
	s.handleDeleteChain(rrWS, deleteChainHTTPReq(t, "proj-ws-chain", "chain-ws"))
	if rrWS.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want 204; body: %s", rrWS.Code, rrWS.Body.String())
	}

	select {
	case msg := <-sendCh:
		var event ws.Event
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("unmarshal WS event: %v", err)
		}
		if event.Type != "chain.deleted" {
			t.Errorf("event.Type = %q, want %q", event.Type, "chain.deleted")
		}
		if event.ProjectID != "proj-ws-chain" {
			t.Errorf("event.ProjectID = %q, want %q", event.ProjectID, "proj-ws-chain")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for chain.deleted WS event")
	}
}

// TestHandleDeleteChain_CascadeDeletesItemsAndLocks verifies items+locks are removed via ON DELETE CASCADE.
func TestHandleDeleteChain_CascadeDeletesItemsAndLocks(t *testing.T) {
	s := newDeleteChainServer(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	projID := "proj-cascade"
	chainID := "chain-cascade"

	if _, err := s.pool.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, 'Cascade', '/tmp', ?, ?)`,
		projID, now, now,
	); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := s.pool.Exec(`
		INSERT INTO chain_executions (id, project_id, name, status, workflow_name, created_by, created_at, updated_at)
		VALUES (?, ?, 'Cascade Chain', 'completed', 'feature', 'test', ?, ?)`,
		chainID, projID, now, now,
	); err != nil {
		t.Fatalf("seed chain: %v", err)
	}
	if _, err := s.pool.Exec(`
		INSERT INTO chain_execution_items (id, chain_id, ticket_id, position, status, workflow_instance_id, started_at, ended_at)
		VALUES ('item-1', ?, 'ticket-a', 1, 'completed', NULL, NULL, NULL)`,
		chainID,
	); err != nil {
		t.Fatalf("seed chain item: %v", err)
	}
	if _, err := s.pool.Exec(
		`INSERT INTO chain_execution_locks (project_id, ticket_id, chain_id) VALUES (?, 'ticket-a', ?)`,
		projID, chainID,
	); err != nil {
		t.Fatalf("seed chain lock: %v", err)
	}

	rr := httptest.NewRecorder()
	s.handleDeleteChain(rr, deleteChainHTTPReq(t, projID, chainID))
	if rr.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want 204; body: %s", rr.Code, rr.Body.String())
	}

	var itemCount int
	if err := s.pool.QueryRow(`SELECT COUNT(*) FROM chain_execution_items WHERE chain_id = ?`, chainID).Scan(&itemCount); err != nil {
		t.Fatalf("count items: %v", err)
	}
	if itemCount != 0 {
		t.Errorf("chain_execution_items count = %d, want 0 after cascade delete", itemCount)
	}

	var lockCount int
	if err := s.pool.QueryRow(`SELECT COUNT(*) FROM chain_execution_locks WHERE chain_id = ?`, chainID).Scan(&lockCount); err != nil {
		t.Fatalf("count locks: %v", err)
	}
	if lockCount != 0 {
		t.Errorf("chain_execution_locks count = %d, want 0 after cascade delete", lockCount)
	}
}
