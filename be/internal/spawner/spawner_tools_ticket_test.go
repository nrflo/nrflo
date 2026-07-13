package spawner

// Regression test: ticket_create dispatched via Spawner.DispatchTool (the
// tools.call path) must persist the ticket and emit ticket.updated via WSHub.
// Covers the gap left by tools_builtin tests (which call handlers directly).

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/tools_builtin"
	"be/internal/ws"
)

// captureHub is a test service.WSHub that records all broadcast calls.
type captureHub struct {
	events []*ws.Event
}

func (h *captureHub) Broadcast(e *ws.Event) {
	h.events = append(h.events, e)
}

// registerTicketCreateProc seeds the spawner with a real ticket_create handler
// whose apiToolEnv is wired to pool, hub, and ticketSvc under the "proj" project.
func registerTicketCreateProc(t *testing.T, s *Spawner, sessionID string, hub service.WSHub) {
	t.Helper()
	pool := setupTestDB(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ticketSvc := service.NewTicketService(pool, clk)

	h := tools_builtin.Builtins()["ticket_create"]
	spec := h.Spec()

	toolEnv := apirun.ToolEnv{
		Pool:      pool,
		WSHub:     hub,
		Clock:     clk,
		ProjectID: "proj",
		Ticket:    ticketSvc,
	}
	proc := &processInfo{
		sessionID:   sessionID,
		apiTools:    []provider.ToolSpec{spec},
		apiHandlers: apirun.Registry{"ticket_create": h},
		apiToolEnv:  toolEnv,
	}
	s.registerSessionProc(sessionID, proc)
}

// TestDispatchTool_TicketCreate_BroadcastsViaDispatch proves that the full
// tools.call dispatch path (Spawner.DispatchTool → handler.Invoke) persists
// the ticket and emits ws.EventTicketUpdated with action="created", status="open".
func TestDispatchTool_TicketCreate_BroadcastsViaDispatch(t *testing.T) {
	hub := &captureHub{}
	s := New(Config{Clock: clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))})
	registerTicketCreateProc(t, s, "sess-tc-broadcast", hub)

	out, _, isErr, terminal, err := s.DispatchTool(
		"sess-tc-broadcast", "ticket_create",
		json.RawMessage(`{"title":"New ticket from dispatch","type":"feature","priority":1}`),
	)
	if err != nil {
		t.Fatalf("DispatchTool err: %v", err)
	}
	if isErr {
		t.Errorf("isError = true, output=%q, want false", out)
	}
	if terminal != "" {
		t.Errorf("terminal = %q, want empty", terminal)
	}

	var result struct {
		TicketID string `json:"ticket_id"`
		Title    string `json:"title"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parse output %q: %v", out, err)
	}
	if result.TicketID == "" {
		t.Errorf("ticket_id empty in output %q", out)
	}
	if result.Title != "New ticket from dispatch" {
		t.Errorf("title = %q, want 'New ticket from dispatch'", result.Title)
	}

	if len(hub.events) != 1 {
		t.Fatalf("broadcast count = %d, want 1; events: %+v", len(hub.events), hub.events)
	}
	ev := hub.events[0]
	if ev.Type != ws.EventTicketUpdated {
		t.Errorf("event type = %q, want %q", ev.Type, ws.EventTicketUpdated)
	}
	if ev.Data["action"] != "created" {
		t.Errorf("action = %v, want 'created'", ev.Data["action"])
	}
	if ev.Data["status"] != "open" {
		t.Errorf("status = %v, want 'open'", ev.Data["status"])
	}
}

// TestDispatchTool_TicketCreate_NilHub_NoBroadcast verifies that nil WSHub
// does not panic (BroadcastFromCtx is nil-safe) and the ticket is still persisted.
func TestDispatchTool_TicketCreate_NilHub_NoBroadcast(t *testing.T) {
	s := New(Config{Clock: clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))})
	registerTicketCreateProc(t, s, "sess-tc-nilhub", nil)

	out, _, isErr, terminal, err := s.DispatchTool(
		"sess-tc-nilhub", "ticket_create",
		json.RawMessage(`{"title":"No hub ticket"}`),
	)
	if err != nil {
		t.Fatalf("DispatchTool err: %v", err)
	}
	if isErr {
		t.Errorf("isError = true, want false; output=%q", out)
	}
	if terminal != "" {
		t.Errorf("terminal = %q, want empty", terminal)
	}

	var result struct {
		TicketID string `json:"ticket_id"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parse output %q: %v", out, err)
	}
	if result.TicketID == "" {
		t.Errorf("ticket_id empty, want non-empty")
	}
}

// TestDispatchTool_TicketCreate_MissingTitle_NoCreate verifies that a blank
// title returns isError=true with no WS broadcast (validation short-circuits
// before the DB write and the broadcast call).
func TestDispatchTool_TicketCreate_MissingTitle_NoCreate(t *testing.T) {
	hub := &captureHub{}
	s := New(Config{Clock: clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))})
	registerTicketCreateProc(t, s, "sess-tc-notitle", hub)

	out, _, isErr, _, err := s.DispatchTool(
		"sess-tc-notitle", "ticket_create",
		json.RawMessage(`{"description":"no title here"}`),
	)
	if err != nil {
		t.Fatalf("DispatchTool err: %v", err)
	}
	if !isErr {
		t.Errorf("isError = false, want true; output=%q", out)
	}
	if len(hub.events) != 0 {
		t.Errorf("broadcast count = %d, want 0 (no broadcast on validation failure)", len(hub.events))
	}
}
