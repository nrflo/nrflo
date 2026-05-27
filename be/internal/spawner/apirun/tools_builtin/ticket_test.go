package tools_builtin

import (
	"encoding/json"
	"strings"
	"testing"

	"be/internal/ws"
)

// createTicket invokes ticket_create and returns the new ticket id.
func createTicket(t *testing.T, e *builtinTestEnv, input string) string {
	t.Helper()
	out, isErr, err := invoke(t, e.env, "ticket_create", input)
	if err != nil {
		t.Fatalf("ticket_create err: %v", err)
	}
	if isErr {
		t.Fatalf("ticket_create isErr=true, output=%q", out)
	}
	var res struct {
		TicketID string `json:"ticket_id"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("parse ticket_create output %q: %v", out, err)
	}
	if res.TicketID == "" {
		t.Fatalf("ticket_create returned empty ticket_id: %q", out)
	}
	return res.TicketID
}

func TestTicketCreate_PersistsAndBroadcasts(t *testing.T) {
	e := newBuiltinTestEnv(t)
	id := createTicket(t, e, `{"title":"Add pagination","description":"body","type":"feature","priority":1}`)

	var title, issueType string
	var priority int
	err := e.pool.QueryRow(`SELECT title, issue_type, priority FROM tickets WHERE id=?`,
		strings.ToLower(id)).Scan(&title, &issueType, &priority)
	if err != nil {
		t.Fatalf("read created ticket: %v", err)
	}
	if title != "Add pagination" || issueType != "feature" || priority != 1 {
		t.Errorf("got title=%q type=%q priority=%d", title, issueType, priority)
	}
	if len(e.hub.events) != 1 || e.hub.events[0].Type != ws.EventTicketUpdated {
		t.Fatalf("expected ticket.updated event, got %+v", e.hub.events)
	}
	if e.hub.events[0].Data["action"] != "created" {
		t.Errorf("action = %v, want created", e.hub.events[0].Data["action"])
	}
}

func TestTicketCreate_Defaults(t *testing.T) {
	e := newBuiltinTestEnv(t)
	id := createTicket(t, e, `{"title":"Minimal"}`)

	var issueType string
	var priority int
	if err := e.pool.QueryRow(`SELECT issue_type, priority FROM tickets WHERE id=?`,
		strings.ToLower(id)).Scan(&issueType, &priority); err != nil {
		t.Fatalf("read ticket: %v", err)
	}
	if issueType != "task" || priority != 2 {
		t.Errorf("defaults: type=%q priority=%d, want task/2", issueType, priority)
	}
}

func TestTicketCreate_WithParent(t *testing.T) {
	e := newBuiltinTestEnv(t)
	parent := createTicket(t, e, `{"title":"Epic","type":"epic"}`)
	child := createTicket(t, e, `{"title":"Child","parent_id":"`+parent+`"}`)

	var gotParent string
	if err := e.pool.QueryRow(`SELECT parent_ticket_id FROM tickets WHERE id=?`,
		strings.ToLower(child)).Scan(&gotParent); err != nil {
		t.Fatalf("read child parent: %v", err)
	}
	if gotParent != strings.ToLower(parent) {
		t.Errorf("parent_ticket_id = %q, want %q", gotParent, strings.ToLower(parent))
	}
}

func TestTicketCreate_MissingTitle(t *testing.T) {
	e := newBuiltinTestEnv(t)
	out, isErr, err := invoke(t, e.env, "ticket_create", `{"description":"no title"}`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !isErr || !strings.Contains(out, "title is required") {
		t.Errorf("output=%q isErr=%v, want title required error", out, isErr)
	}
	if len(e.hub.events) != 0 {
		t.Errorf("expected no broadcast on validation failure, got %+v", e.hub.events)
	}
}

func TestTicketAddDependency_Persists(t *testing.T) {
	e := newBuiltinTestEnv(t)
	blocker := createTicket(t, e, `{"title":"Migration"}`)
	blocked := createTicket(t, e, `{"title":"API endpoint"}`)

	out, isErr, err := invoke(t, e.env, "ticket_add_dependency",
		`{"ticket_id":"`+blocked+`","depends_on_id":"`+blocker+`"}`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if isErr || out != "ok" {
		t.Fatalf("output=%q isErr=%v, want ok", out, isErr)
	}

	var depType string
	err = e.pool.QueryRow(`SELECT type FROM dependencies WHERE issue_id=? AND depends_on_id=?`,
		strings.ToLower(blocked), strings.ToLower(blocker)).Scan(&depType)
	if err != nil {
		t.Fatalf("read dependency: %v", err)
	}
	if depType != "blocks" {
		t.Errorf("dependency type = %q, want blocks", depType)
	}
}

func TestTicketAddDependency_MissingArgs(t *testing.T) {
	e := newBuiltinTestEnv(t)
	out, isErr, _ := invoke(t, e.env, "ticket_add_dependency", `{"ticket_id":"only-one"}`)
	if !isErr || !strings.Contains(out, "required") {
		t.Errorf("output=%q isErr=%v, want required error", out, isErr)
	}
}

func TestTicketAddDependency_UnknownTicket(t *testing.T) {
	e := newBuiltinTestEnv(t)
	blocked := createTicket(t, e, `{"title":"Real ticket"}`)
	out, isErr, _ := invoke(t, e.env, "ticket_add_dependency",
		`{"ticket_id":"`+blocked+`","depends_on_id":"does-not-exist"}`)
	if !isErr || !strings.Contains(out, "not found") {
		t.Errorf("output=%q isErr=%v, want not found error", out, isErr)
	}
}
