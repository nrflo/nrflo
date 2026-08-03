package console

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/model"
	"be/internal/service"
)

func TestProjectList_ReturnsSeededProjects(t *testing.T) {
	env := newConsoleTestEnv(t)
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "project_list", `{}`)
	if err != nil || isErr {
		t.Fatalf("err=%v isErr=%v out=%s", err, isErr, out)
	}
	var projects []map[string]interface{}
	if jerr := json.Unmarshal([]byte(out), &projects); jerr != nil {
		t.Fatalf("output does not unmarshal: %v", jerr)
	}
	found := false
	for _, p := range projects {
		if p["id"] == testProjectID {
			found = true
		}
	}
	if !found {
		t.Errorf("projects = %v, want to contain %q", projects, testProjectID)
	}
}

// seedExtraTicket adds one more open ticket to projectID so
// project_status's counts.total can distinguish which project was queried.
func (e *consoleTestEnv) seedExtraTicket(t *testing.T, projectID, ticketID string) {
	t.Helper()
	now := e.clk.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, e.pool, `INSERT INTO tickets (id, project_id, title, created_at, updated_at, created_by) VALUES (?, ?, ?, ?, ?, 'test')`,
		ticketID, projectID, "Extra", now, now)
}

func projectStatusTotal(t *testing.T, out string) int {
	t.Helper()
	var status map[string]interface{}
	if err := json.Unmarshal([]byte(out), &status); err != nil {
		t.Fatalf("output does not unmarshal: %v", err)
	}
	counts, ok := status["counts"].(map[string]interface{})
	if !ok {
		t.Fatalf("status = %v, want a counts object", status)
	}
	total, _ := counts["total"].(float64)
	return int(total)
}

func TestProjectStatus_ProjectScopedSession_IgnoresProjectOverride(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedExtraTicket(t, testOtherProjectID, "T-other-1")
	env.seedExtraTicket(t, testOtherProjectID, "T-other-2")
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", testProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "project_status", `{"project":"`+testOtherProjectID+`"}`)
	if err != nil || isErr {
		t.Fatalf("err=%v isErr=%v out=%s", err, isErr, out)
	}
	// A project-scoped session's `project` override must be ignored — the
	// handler-level guard mirrors consoleToolProject's API-layer guard, so
	// the count reflects testProjectID (1 ticket), not testOtherProjectID (2).
	if got := projectStatusTotal(t, out); got != 1 {
		t.Errorf("counts.total = %d, want 1 (session project, override ignored)", got)
	}
}

func TestProjectStatus_GlobalScopeSession_HonorsProjectOverride(t *testing.T) {
	env := newConsoleTestEnv(t)
	env.seedExtraTicket(t, testOtherProjectID, "T-other-3")
	env.seedExtraTicket(t, testOtherProjectID, "T-other-4")
	reg, err := BuildRegistry(env.deps, nil)
	if err != nil {
		t.Fatalf("BuildRegistry: %v", err)
	}
	toolEnv := NewToolEnv(env.deps, "sess-1", service.GlobalProjectID, model.AgentSessionKindConsole)

	out, isErr, err := invoke(t, reg, toolEnv, "project_status", `{"project":"`+testOtherProjectID+`"}`)
	if err != nil || isErr {
		t.Fatalf("err=%v isErr=%v out=%s", err, isErr, out)
	}
	if got := projectStatusTotal(t, out); got != 2 {
		t.Errorf("counts.total = %d, want 2 (override honored for global-scope session)", got)
	}
}
