package api

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

// newShutdownTestServer builds a Server with the WS hub running but no HTTP listener.
func newShutdownTestServer(t *testing.T) *Server {
	t.Helper()
	srv := newStartTestServer(t)
	go srv.wsHub.Run()
	t.Cleanup(srv.wsHub.Stop)
	return srv
}

// sdProject inserts a minimal project row and returns its ID.
func sdProject(t *testing.T, s *Server) string {
	t.Helper()
	pid := "proj-sd-test"
	_, err := s.pool.Exec(
		`INSERT OR IGNORE INTO projects (id, name, created_at, updated_at) VALUES (?, 'Test', datetime('now'), datetime('now'))`,
		pid)
	if err != nil {
		t.Fatalf("sdProject: %v", err)
	}
	return pid
}

// sdTicket inserts a ticket in in_progress status and returns its ID.
func sdTicket(t *testing.T, s *Server, projectID string) string {
	t.Helper()
	tid := "tkt-sd-001"
	_, err := s.pool.Exec(
		`INSERT OR IGNORE INTO tickets
		(id, project_id, title, description, status, priority, issue_type, created_at, updated_at, closed_at, created_by, close_reason)
		VALUES (?, ?, 'T', '', 'in_progress', 'medium', 'task', datetime('now'), datetime('now'), NULL, 'admin', NULL)`,
		tid, projectID)
	if err != nil {
		t.Fatalf("sdTicket: %v", err)
	}
	return tid
}

// sdWorkflow inserts a minimal workflows row to satisfy the FK on workflow_instances.
// Uses INSERT OR IGNORE so repeated calls within the same test are safe.
func sdWorkflow(t *testing.T, s *Server, projectID, workflowID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.pool.Exec(
		`INSERT OR IGNORE INTO workflows (id, project_id, description, scope_type, groups, close_ticket_on_complete, created_at, updated_at)
		VALUES (?, ?, '', 'ticket', '[]', 1, ?, ?)`,
		workflowID, projectID, now, now)
	if err != nil {
		t.Fatalf("sdWorkflow(%s): %v", workflowID, err)
	}
}

// sdActiveWFI creates an active workflow instance via the repo.
func sdActiveWFI(t *testing.T, s *Server, projectID, ticketID, scopeType, id string) {
	t.Helper()
	sdWorkflow(t, s, projectID, "feature")
	wfiRepo := repo.NewWorkflowInstanceRepo(s.pool, s.clock)
	err := wfiRepo.Create(&model.WorkflowInstance{
		ID:         id,
		ProjectID:  projectID,
		TicketID:   ticketID,
		WorkflowID: "feature",
		ScopeType:  scopeType,
		Status:     model.WorkflowInstanceActive,
	})
	if err != nil {
		t.Fatalf("sdActiveWFI(%s): %v", id, err)
	}
}

// sdRunningSession creates a running agent session via the repo.
func sdRunningSession(t *testing.T, s *Server, projectID, ticketID, wfiID, id string) {
	t.Helper()
	sessRepo := repo.NewAgentSessionRepo(s.pool, s.clock)
	err := sessRepo.Create(&model.AgentSession{
		ID:                 id,
		ProjectID:          projectID,
		TicketID:           ticketID,
		WorkflowInstanceID: wfiID,
		Phase:              "implementor",
		AgentType:          "implementor",
		Status:             model.AgentSessionRunning,
	})
	if err != nil {
		t.Fatalf("sdRunningSession(%s): %v", id, err)
	}
}

// sdWSClient subscribes a test WS client to projectID (ticketID="" = all tickets).
// Calling Register blocks until the hub goroutine processes it, ensuring hub is ready.
func sdWSClient(t *testing.T, s *Server, projectID, ticketID string) <-chan []byte {
	t.Helper()
	client, ch := ws.NewTestClient(s.wsHub, "test-client")
	s.wsHub.Register(client)
	s.wsHub.Subscribe(client, projectID, ticketID)
	return ch
}

// waitForEvent polls ch until an event of wantType is received or timeout elapses.
func waitForEvent(t *testing.T, ch <-chan []byte, timeout time.Duration, wantType string) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return false
		}
		timer := time.NewTimer(remaining)
		select {
		case data, ok := <-ch:
			timer.Stop()
			if !ok {
				return false
			}
			var evt struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(data, &evt) == nil && evt.Type == wantType {
				return true
			}
		case <-timer.C:
			return false
		}
	}
}

// TestShutdownCleanup_TicketScope verifies that a ticket-scope active WI and running
// session are marked failed with reason=server_shutdown, the ticket is reopened,
// and EventOrchestrationFailed is broadcast.
func TestShutdownCleanup_TicketScope(t *testing.T) {
	srv := newShutdownTestServer(t)
	pid := sdProject(t, srv)
	tid := sdTicket(t, srv, pid)

	wfiID := "wfi-tkt-001"
	sdActiveWFI(t, srv, pid, tid, "ticket", wfiID)
	sdRunningSession(t, srv, pid, tid, wfiID, "sess-tkt-001")

	ch := sdWSClient(t, srv, pid, "")

	srv.shutdownCleanup(context.Background())

	wfiRepo := repo.NewWorkflowInstanceRepo(srv.pool, srv.clock)
	wfi, err := wfiRepo.Get(wfiID)
	if err != nil {
		t.Fatalf("Get WFI: %v", err)
	}
	if wfi.Status != model.WorkflowInstanceFailed {
		t.Errorf("WFI status = %q, want %q", wfi.Status, model.WorkflowInstanceFailed)
	}

	sessRepo := repo.NewAgentSessionRepo(srv.pool, srv.clock)
	sess, err := sessRepo.Get("sess-tkt-001")
	if err != nil {
		t.Fatalf("Get session: %v", err)
	}
	if sess.Status != model.AgentSessionFailed {
		t.Errorf("session status = %q, want failed", sess.Status)
	}
	if !sess.ResultReason.Valid || sess.ResultReason.String != "server_shutdown" {
		t.Errorf("result_reason = %q, want server_shutdown", sess.ResultReason.String)
	}

	var ticketStatus string
	if err := srv.pool.QueryRow(`SELECT status FROM tickets WHERE id = ?`, tid).Scan(&ticketStatus); err != nil {
		t.Fatalf("scan ticket status: %v", err)
	}
	if ticketStatus != "open" {
		t.Errorf("ticket status = %q, want open (reopened)", ticketStatus)
	}

	if !waitForEvent(t, ch, 2*time.Second, ws.EventOrchestrationFailed) {
		t.Error("EventOrchestrationFailed not received within 2s")
	}
}

// TestShutdownCleanup_ProjectScope verifies project-scope WIs are failed and no
// ticket operations occur (no ticket seeded, no reopen attempted).
func TestShutdownCleanup_ProjectScope(t *testing.T) {
	srv := newShutdownTestServer(t)
	pid := sdProject(t, srv)

	wfiID := "wfi-proj-001"
	sdActiveWFI(t, srv, pid, "", "project", wfiID)

	ch := sdWSClient(t, srv, pid, "")

	srv.shutdownCleanup(context.Background())

	wfiRepo := repo.NewWorkflowInstanceRepo(srv.pool, srv.clock)
	wfi, err := wfiRepo.Get(wfiID)
	if err != nil {
		t.Fatalf("Get WFI: %v", err)
	}
	if wfi.Status != model.WorkflowInstanceFailed {
		t.Errorf("WFI status = %q, want failed", wfi.Status)
	}

	if !waitForEvent(t, ch, 2*time.Second, ws.EventOrchestrationFailed) {
		t.Error("EventOrchestrationFailed not received for project-scope WFI")
	}

	var count int
	if err := srv.pool.QueryRow(`SELECT COUNT(*) FROM tickets WHERE project_id = ?`, pid).Scan(&count); err != nil {
		t.Fatalf("count tickets: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 tickets (no reopen attempted), got %d", count)
	}
}

// TestShutdownCleanup_CompletedWFI_NoOp verifies already-completed WIs are not touched.
func TestShutdownCleanup_CompletedWFI_NoOp(t *testing.T) {
	srv := newShutdownTestServer(t)
	pid := sdProject(t, srv)
	sdWorkflow(t, srv, pid, "feature")

	wfiRepo := repo.NewWorkflowInstanceRepo(srv.pool, srv.clock)
	if err := wfiRepo.Create(&model.WorkflowInstance{
		ID:         "wfi-done-001",
		ProjectID:  pid,
		WorkflowID: "feature",
		ScopeType:  "project",
		Status:     model.WorkflowInstanceCompleted,
	}); err != nil {
		t.Fatalf("Create completed WFI: %v", err)
	}

	srv.shutdownCleanup(context.Background())

	wfi, err := wfiRepo.Get("wfi-done-001")
	if err != nil {
		t.Fatalf("Get WFI: %v", err)
	}
	if wfi.Status != model.WorkflowInstanceCompleted {
		t.Errorf("WFI status changed from completed to %q (should be no-op)", wfi.Status)
	}
}

// TestShutdownCleanup_Idempotent verifies that calling shutdownCleanup twice does
// not double-process already-failed rows (idempotent sweeps).
func TestShutdownCleanup_Idempotent(t *testing.T) {
	srv := newShutdownTestServer(t)
	pid := sdProject(t, srv)
	tid := sdTicket(t, srv, pid)

	wfiID := "wfi-idem-001"
	sdActiveWFI(t, srv, pid, tid, "ticket", wfiID)
	sdRunningSession(t, srv, pid, tid, wfiID, "sess-idem-001")

	ctx := context.Background()
	srv.shutdownCleanup(ctx)
	srv.shutdownCleanup(ctx)

	wfiRepo := repo.NewWorkflowInstanceRepo(srv.pool, srv.clock)
	wfi, err := wfiRepo.Get(wfiID)
	if err != nil {
		t.Fatalf("Get WFI: %v", err)
	}
	if wfi.Status != model.WorkflowInstanceFailed {
		t.Errorf("WFI status = %q, want failed after idempotent calls", wfi.Status)
	}

	sessRepo := repo.NewAgentSessionRepo(srv.pool, srv.clock)
	sess, err := sessRepo.Get("sess-idem-001")
	if err != nil {
		t.Fatalf("Get session: %v", err)
	}
	if sess.Status != model.AgentSessionFailed {
		t.Errorf("session status = %q, want failed", sess.Status)
	}
	if !sess.ResultReason.Valid || sess.ResultReason.String != "server_shutdown" {
		t.Errorf("result_reason = %q, want server_shutdown", sess.ResultReason.String)
	}
}
