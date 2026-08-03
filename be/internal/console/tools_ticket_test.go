package console

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/model"
	"be/internal/service"
)

// seedTicket inserts a ticket with an explicit status/issue_type into projectID.
func (e *consoleTestEnv) seedTicket(t *testing.T, projectID, id, status, issueType string) {
	t.Helper()
	now := e.clk.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, e.pool, `INSERT INTO tickets (id, project_id, title, description, status, issue_type, created_at, updated_at, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'test')`,
		id, projectID, "Title "+id, "Desc "+id, status, issueType, now, now)
}

// seedDependency makes issueID depend on dependsOnID within projectID.
func (e *consoleTestEnv) seedDependency(t *testing.T, projectID, issueID, dependsOnID string) {
	t.Helper()
	now := e.clk.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, e.pool, `INSERT INTO dependencies (project_id, issue_id, depends_on_id, created_at, created_by)
		VALUES (?, ?, ?, ?, 'test')`, projectID, issueID, dependsOnID, now)
}

func listTickets(t *testing.T, env *consoleTestEnv, projectID, args string) []map[string]interface{} {
	t.Helper()
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", projectID, model.AgentSessionKindConsole)
	out, isErr, err := invoke(t, reg, toolEnv, "ticket_list", args)
	if err != nil || isErr {
		t.Fatalf("ticket_list err=%v isErr=%v out=%s", err, isErr, out)
	}
	var items []map[string]interface{}
	if jerr := json.Unmarshal([]byte(out), &items); jerr != nil {
		t.Fatalf("output does not unmarshal: %v (out=%s)", jerr, out)
	}
	return items
}

type idSet map[string]bool

func (s idSet) Contains(id string) bool { return s[id] }

func idsOf(items []map[string]interface{}) idSet {
	set := make(idSet, len(items))
	for _, it := range items {
		if id, ok := it["id"].(string); ok {
			set[id] = true
		}
	}
	return set
}

func TestTicketList_CompactProjection(t *testing.T) {
	env := newConsoleTestEnv(t)
	items := listTickets(t, env, testProjectID, `{}`)
	if !idsOf(items).Contains(testTicketID) {
		t.Fatalf("ticket_list = %v, want to contain %q", items, testTicketID)
	}
	var row map[string]interface{}
	for _, it := range items {
		if it["id"] == testTicketID {
			row = it
		}
	}
	// Picker fields present; the heavy description is not — that's ticket_get's job.
	for _, k := range []string{"id", "title", "status", "issue_type", "priority", "is_blocked"} {
		if _, ok := row[k]; !ok {
			t.Errorf("row missing %q: %v", k, row)
		}
	}
	if _, ok := row["description"]; ok {
		t.Errorf("row should not carry description (compact projection): %v", row)
	}
}

func TestTicketList_StatusFilter(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedTicket(t, testProjectID, "T-inprog", "in_progress", "task")
	env.seedTicket(t, testProjectID, "T-open-x", "open", "task")

	ids := idsOf(listTickets(t, env, testProjectID, `{"status":"in_progress"}`))
	if !ids.Contains("T-inprog") {
		t.Errorf("status=in_progress missing T-inprog: %v", ids)
	}
	for _, unwanted := range []string{"T-open-x", testTicketID} {
		if ids.Contains(unwanted) {
			t.Errorf("status=in_progress should not contain %q: %v", unwanted, ids)
		}
	}
}

func TestTicketList_TypeFilter(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedTicket(t, testProjectID, "T-bug", "open", "bug")
	env.seedTicket(t, testProjectID, "T-task", "open", "task")

	ids := idsOf(listTickets(t, env, testProjectID, `{"type":"bug"}`))
	if !ids.Contains("T-bug") {
		t.Errorf("type=bug missing T-bug: %v", ids)
	}
	if ids.Contains("T-task") {
		t.Errorf("type=bug should not contain T-task: %v", ids)
	}
}

func TestTicketList_BlockedFilter(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedTicket(t, testProjectID, "T-blocker", "open", "task")
	env.seedTicket(t, testProjectID, "T-blocked", "open", "task")
	env.seedDependency(t, testProjectID, "T-blocked", "T-blocker")

	items := listTickets(t, env, testProjectID, `{"status":"blocked"}`)
	ids := idsOf(items)
	if !ids.Contains("T-blocked") {
		t.Errorf("status=blocked missing T-blocked: %v", ids)
	}
	// The blocker itself has no open blockers, so it is not blocked.
	if ids.Contains("T-blocker") {
		t.Errorf("status=blocked should not contain the blocker T-blocker: %v", ids)
	}
	for _, it := range items {
		if it["id"] == "T-blocked" && it["is_blocked"] != true {
			t.Errorf("T-blocked is_blocked = %v, want true", it["is_blocked"])
		}
	}
}

func TestTicketList_LimitHonored(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedTicket(t, testProjectID, "T-a", "open", "task")
	env.seedTicket(t, testProjectID, "T-b", "open", "task")
	env.seedTicket(t, testProjectID, "T-c", "open", "task")

	items := listTickets(t, env, testProjectID, `{"limit":2}`)
	if len(items) != 2 {
		t.Errorf("limit=2 returned %d tickets, want 2", len(items))
	}
}

func TestTicketList_CrossProjectIsolation(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedTicket(t, testOtherProjectID, "T-foreign", "open", "task")

	ids := idsOf(listTickets(t, env, testProjectID, `{}`))
	if ids.Contains("T-foreign") {
		t.Errorf("a %s session must not see %s's ticket: %v", testProjectID, testOtherProjectID, ids)
	}
}

func TestTicketGet_ReturnsFullRow(t *testing.T) {
	env := newConsoleTestEnv(t)
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "ticket_get", `{"ticket_id":"`+testTicketID+`"}`)
	if err != nil || isErr {
		t.Fatalf("ticket_get err=%v isErr=%v out=%s", err, isErr, out)
	}
	var row map[string]interface{}
	if jerr := json.Unmarshal([]byte(out), &row); jerr != nil {
		t.Fatalf("output does not unmarshal: %v", jerr)
	}
	if row["id"] != testTicketID {
		t.Errorf("id = %v, want %q", row["id"], testTicketID)
	}
	// ticket_get is the detail view: description must be present.
	if _, ok := row["description"]; !ok {
		t.Errorf("ticket_get row missing description: %v", row)
	}
}

func TestTicketGet_CrossProjectIsolation(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedTicket(t, testOtherProjectID, "T-foreign", "open", "task")
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, _ := invoke(t, reg, toolEnv, "ticket_get", `{"ticket_id":"T-foreign"}`)
	if !isErr {
		t.Errorf("ticket_get on a foreign-project ticket should error, got out=%s", out)
	}
}

func TestTicketCurrent_ReturnsStampedTicket(t *testing.T) {
	env := newConsoleTestEnv(t)
	// Mint a real console session row carrying the current ticket.
	sid, _, err := service.NewConsoleService(env.pool, env.clk).CreateSession(testProjectID, testTicketID)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, sid, testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "ticket_current", `{}`)
	if err != nil || isErr {
		t.Fatalf("ticket_current err=%v isErr=%v out=%s", err, isErr, out)
	}
	var resp struct {
		CurrentTicket map[string]interface{} `json:"current_ticket"`
	}
	if jerr := json.Unmarshal([]byte(out), &resp); jerr != nil {
		t.Fatalf("output does not unmarshal: %v (out=%s)", jerr, out)
	}
	if resp.CurrentTicket == nil || resp.CurrentTicket["id"] != testTicketID {
		t.Errorf("current_ticket = %v, want ticket %q", resp.CurrentTicket, testTicketID)
	}
}

func TestTicketCurrent_NoTicket_ReturnsNull(t *testing.T) {
	env := newConsoleTestEnv(t)
	// A session with no stamped ticket.
	sid, _, err := service.NewConsoleService(env.pool, env.clk).CreateSession(testProjectID, "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, sid, testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "ticket_current", `{}`)
	if err != nil || isErr {
		t.Fatalf("ticket_current err=%v isErr=%v out=%s", err, isErr, out)
	}
	if out != `{"current_ticket":null}` {
		t.Errorf("out = %s, want {\"current_ticket\":null}", out)
	}
}

func TestTicketGet_MissingID(t *testing.T) {
	env := newConsoleTestEnv(t)
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, _ := invoke(t, reg, toolEnv, "ticket_get", `{}`)
	if !isErr {
		t.Errorf("ticket_get with no ticket_id should error, got out=%s", out)
	}
}
