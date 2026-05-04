package service

import (
	"testing"

	"be/internal/ws"
)

// fakeHub captures broadcast calls for assertion.
type fakeHub struct {
	events []*ws.Event
}

func (h *fakeHub) Broadcast(e *ws.Event) {
	h.events = append(h.events, e)
}

// TestBroadcastFromCtx_BuildsExpectedEvent verifies the helper unpacks
// BroadcastCtx and emits an event matching ws.NewEvent's output shape.
func TestBroadcastFromCtx_BuildsExpectedEvent(t *testing.T) {
	t.Parallel()
	hub := &fakeHub{}
	bc := BroadcastCtx{
		SessionID: "sess-1",
		ProjectID: "proj-1",
		TicketID:  "T-1",
		Workflow:  "feature",
		AgentType: "implementor",
		ModelID:   "claude:opus",
	}
	data := map[string]interface{}{
		"agent_type": bc.AgentType,
		"key":        "summary",
		"action":     "add",
	}

	BroadcastFromCtx(hub, ws.EventFindingsUpdated, bc, data)

	if len(hub.events) != 1 {
		t.Fatalf("events len = %d, want 1", len(hub.events))
	}
	got := hub.events[0]
	want := ws.NewEvent(ws.EventFindingsUpdated, bc.ProjectID, bc.TicketID, bc.Workflow, data)

	if got.Type != want.Type {
		t.Errorf("Type = %q, want %q", got.Type, want.Type)
	}
	if got.ProjectID != want.ProjectID {
		t.Errorf("ProjectID = %q, want %q", got.ProjectID, want.ProjectID)
	}
	if got.TicketID != want.TicketID {
		t.Errorf("TicketID = %q, want %q", got.TicketID, want.TicketID)
	}
	if got.Workflow != want.Workflow {
		t.Errorf("Workflow = %q, want %q", got.Workflow, want.Workflow)
	}
	if got.Data["agent_type"] != "implementor" {
		t.Errorf("Data[agent_type] = %v, want implementor", got.Data["agent_type"])
	}
	if got.Data["key"] != "summary" {
		t.Errorf("Data[key] = %v, want summary", got.Data["key"])
	}
}

// TestBroadcastFromCtx_NilHub is a no-op (matches existing socket-handler behavior
// where the hub may be nil in some test setups).
func TestBroadcastFromCtx_NilHub(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("BroadcastFromCtx(nil hub) panicked: %v", r)
		}
	}()
	BroadcastFromCtx(nil, ws.EventFindingsUpdated, BroadcastCtx{ProjectID: "p"}, nil)
}

// TestBroadcastFromCtx_PassesScopeFields verifies project/ticket/workflow are
// taken from the context (not the data map).
func TestBroadcastFromCtx_PassesScopeFields(t *testing.T) {
	t.Parallel()
	hub := &fakeHub{}
	bc := BroadcastCtx{ProjectID: "P", TicketID: "T", Workflow: "W"}
	BroadcastFromCtx(hub, "agent.completed", bc, map[string]interface{}{"x": 1})
	if len(hub.events) != 1 {
		t.Fatalf("events len = %d, want 1", len(hub.events))
	}
	e := hub.events[0]
	if e.ProjectID != "P" || e.TicketID != "T" || e.Workflow != "W" {
		t.Errorf("scope fields = (%q,%q,%q), want (P,T,W)", e.ProjectID, e.TicketID, e.Workflow)
	}
}
