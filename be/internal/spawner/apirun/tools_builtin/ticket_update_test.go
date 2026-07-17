package tools_builtin

import (
	"strings"
	"testing"

	"be/internal/ws"
)

func TestTicketUpdate_PersistsAndBroadcasts(t *testing.T) {
	e := newBuiltinTestEnv(t)
	id := createTicket(t, e, `{"title":"Old title","priority":2}`)

	out, isErr, err := invoke(t, e.env, "ticket_update",
		`{"ticket_id":"`+id+`","title":"New title","priority":1,"status":"in_progress"}`)
	if err != nil {
		t.Fatalf("ticket_update err: %v", err)
	}
	if isErr {
		t.Fatalf("ticket_update isErr=true, output=%q", out)
	}

	var title, status string
	var priority int
	if err := e.pool.QueryRow(`SELECT title, status, priority FROM tickets WHERE id=?`,
		strings.ToLower(id)).Scan(&title, &status, &priority); err != nil {
		t.Fatalf("read updated ticket: %v", err)
	}
	if title != "New title" || status != "in_progress" || priority != 1 {
		t.Errorf("got title=%q status=%q priority=%d", title, status, priority)
	}
	if len(e.hub.events) != 2 || e.hub.events[1].Type != ws.EventTicketUpdated {
		t.Fatalf("expected create + update events, got %+v", e.hub.events)
	}
	if e.hub.events[1].Data["action"] != "updated" {
		t.Errorf("action = %v, want updated", e.hub.events[1].Data["action"])
	}
}

func TestTicketUpdate_PartialFieldsKeepOthers(t *testing.T) {
	e := newBuiltinTestEnv(t)
	id := createTicket(t, e, `{"title":"Keep me","description":"old body","priority":3}`)

	out, isErr, _ := invoke(t, e.env, "ticket_update",
		`{"ticket_id":"`+id+`","description":"new body"}`)
	if isErr {
		t.Fatalf("ticket_update isErr=true, output=%q", out)
	}

	var title, description string
	var priority int
	if err := e.pool.QueryRow(`SELECT title, description, priority FROM tickets WHERE id=?`,
		strings.ToLower(id)).Scan(&title, &description, &priority); err != nil {
		t.Fatalf("read ticket: %v", err)
	}
	if title != "Keep me" || description != "new body" || priority != 3 {
		t.Errorf("got title=%q description=%q priority=%d, want unchanged title/priority", title, description, priority)
	}
}

func TestTicketUpdate_MissingTicketID(t *testing.T) {
	e := newBuiltinTestEnv(t)
	out, isErr, _ := invoke(t, e.env, "ticket_update", `{"title":"x"}`)
	if !isErr || !strings.Contains(out, "ticket_id is required") {
		t.Errorf("output=%q isErr=%v, want ticket_id required error", out, isErr)
	}
}

func TestTicketUpdate_NoFields(t *testing.T) {
	e := newBuiltinTestEnv(t)
	id := createTicket(t, e, `{"title":"Untouched"}`)
	out, isErr, _ := invoke(t, e.env, "ticket_update", `{"ticket_id":"`+id+`"}`)
	if !isErr || !strings.Contains(out, "at least one field") {
		t.Errorf("output=%q isErr=%v, want no-fields error", out, isErr)
	}
}

func TestTicketUpdate_InvalidValues(t *testing.T) {
	e := newBuiltinTestEnv(t)
	id := createTicket(t, e, `{"title":"Guarded"}`)
	for name, input := range map[string]string{
		"status":   `{"ticket_id":"` + id + `","status":"done"}`,
		"type":     `{"ticket_id":"` + id + `","type":"story"}`,
		"priority": `{"ticket_id":"` + id + `","priority":9}`,
	} {
		out, isErr, _ := invoke(t, e.env, "ticket_update", input)
		if !isErr || !strings.Contains(out, "invalid "+name) {
			t.Errorf("%s: output=%q isErr=%v, want invalid %s error", name, out, isErr, name)
		}
	}
}

func TestTicketUpdate_UnknownTicket(t *testing.T) {
	e := newBuiltinTestEnv(t)
	out, isErr, _ := invoke(t, e.env, "ticket_update", `{"ticket_id":"does-not-exist","title":"x"}`)
	if !isErr || !strings.Contains(out, "not found") {
		t.Errorf("output=%q isErr=%v, want not found error", out, isErr)
	}
	if len(e.hub.events) != 0 {
		t.Errorf("expected no broadcast on failed update, got %+v", e.hub.events)
	}
}
