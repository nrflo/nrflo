package console

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner"
)

// newInvokableChatTestService is newChatTestService plus a real
// console.Deps wired onto ChatDeps.Tools (Pool/Clock/WSHub +
// WorkflowSvc/TicketSvc/ProjectFindingsSvc), matching newConsoleTestEnv
// (helpers_test.go) — additive: the fake engine never reads deps.Tools, so
// every existing chat_service* test is unaffected.
func newInvokableChatTestService(t *testing.T) (*ChatService, *fakeEngineFactory) {
	t.Helper()
	svc, pool, hub, factory := newChatTestService(t)
	clk := svc.deps.Clock
	svc.deps.Tools = Deps{
		Pool:               pool,
		Clock:              clk,
		WSHub:              hub,
		WorkflowSvc:        service.NewWorkflowService(pool, clk),
		TicketSvc:          service.NewTicketService(pool, clk),
		ProjectFindingsSvc: service.NewProjectFindingsService(pool, clk),
	}
	return svc, factory
}

func TestChatService_InvokeTool_HappyPath_ProjectList(t *testing.T) {
	t.Parallel()
	svc, _ := newInvokableChatTestService(t)
	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	res, err := svc.InvokeTool(context.Background(), sid, "project_list", json.RawMessage(`{}`), false)
	if err != nil {
		t.Fatalf("InvokeTool: %v", err)
	}
	if !res.OK {
		t.Errorf("OK = false, want true; result=%s", res.Result)
	}
	var projects []map[string]interface{}
	if jerr := json.Unmarshal([]byte(res.Result), &projects); jerr != nil {
		t.Errorf("Result does not unmarshal as array: %v (result=%s)", jerr, res.Result)
	}
	if res.DurationMs < 0 {
		t.Errorf("DurationMs = %d, want >= 0", res.DurationMs)
	}
}

func TestChatService_InvokeTool_TurnActive_ReturnsErrTurnActive(t *testing.T) {
	t.Parallel()
	svc, _ := newInvokableChatTestService(t)
	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.SendMessage(sid, "hi"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	_, err = svc.InvokeTool(context.Background(), sid, "project_list", json.RawMessage(`{}`), false)
	if err != spawner.ErrTurnActive {
		t.Errorf("InvokeTool while a turn is running: err = %v, want spawner.ErrTurnActive", err)
	}
}

func TestChatService_InvokeTool_UnknownTool_ReturnsErrToolNotFound(t *testing.T) {
	t.Parallel()
	svc, _ := newInvokableChatTestService(t)
	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = svc.InvokeTool(context.Background(), sid, "no_such_tool", json.RawMessage(`{}`), false)
	if err != ErrToolNotFound {
		t.Errorf("err = %v, want ErrToolNotFound", err)
	}
}

func TestChatService_InvokeTool_ToolReportedError_ReturnsOKFalse(t *testing.T) {
	t.Parallel()
	svc, _ := newInvokableChatTestService(t)
	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	res, err := svc.InvokeTool(context.Background(), sid, "ticket_get", json.RawMessage(`{}`), false)
	if err != nil {
		t.Fatalf("InvokeTool: unexpected Go error %v (want (result,nil) with OK=false)", err)
	}
	if res.OK {
		t.Error("OK = true, want false for a missing ticket_id")
	}
	if !strings.Contains(res.Result, "ticket_id is required") {
		t.Errorf("Result = %q, want it to contain %q", res.Result, "ticket_id is required")
	}
}

func TestChatService_InvokeTool_PersistsTranscriptRows(t *testing.T) {
	t.Parallel()
	svc, _ := newInvokableChatTestService(t)
	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := svc.InvokeTool(context.Background(), sid, "project_list", json.RawMessage(`{}`), false); err != nil {
		t.Fatalf("InvokeTool: %v", err)
	}

	rows, err := repo.NewAgentMessageRepo(svc.deps.Pool, svc.deps.Clock).GetBySessionPaginated(sid, 10, 0)
	if err != nil {
		t.Fatalf("GetBySessionPaginated: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	if rows[0].Category != "user_input" {
		t.Errorf("rows[0].Category = %q, want user_input", rows[0].Category)
	}
	if !strings.Contains(rows[0].Content, "/invoke project_list") {
		t.Errorf("rows[0].Content = %q, want it to start with /invoke project_list", rows[0].Content)
	}
	if rows[1].Category != "tool" {
		t.Errorf("rows[1].Category = %q, want tool", rows[1].Category)
	}
	if !strings.HasPrefix(rows[1].Content, "project_list → ") {
		t.Errorf("rows[1].Content = %q, want it to start with %q", rows[1].Content, "project_list → ")
	}
}

func TestChatService_InvokeTool_InformModel_AppendsSeedContextOnce(t *testing.T) {
	t.Parallel()
	svc, factory := newInvokableChatTestService(t)
	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	sess, ok := svc.get(sid)
	if !ok {
		t.Fatalf("session %s not found", sid)
	}

	res, err := svc.InvokeTool(context.Background(), sid, "project_list", json.RawMessage(`{}`), true)
	if err != nil {
		t.Fatalf("InvokeTool: %v", err)
	}
	if !res.Informed {
		t.Error("Informed = false, want true")
	}

	if _, err := svc.SendMessage(sid, "next question"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	eng := factory.last()
	if eng == nil || len(eng.turns) == 0 {
		t.Fatal("no turn recorded")
	}
	turnText := eng.turns[0]
	if strings.Count(turnText, "[console invoke]") != 1 {
		t.Errorf("turn text = %q, want the digest exactly once", turnText)
	}
	if !strings.HasSuffix(turnText, "next question") {
		t.Errorf("turn text = %q, want it to end with the plain message", turnText)
	}

	// Consumed exactly once: seedContext must be empty now.
	if got := sess.takeSeedContext(); got != "" {
		t.Errorf("seedContext after the informed turn = %q, want empty (consumed once)", got)
	}
}

func TestChatService_InvokeTool_InformModelFalse_LeavesSeedContextEmpty(t *testing.T) {
	t.Parallel()
	svc, _ := newInvokableChatTestService(t)
	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	sess, ok := svc.get(sid)
	if !ok {
		t.Fatalf("session %s not found", sid)
	}

	res, err := svc.InvokeTool(context.Background(), sid, "project_list", json.RawMessage(`{}`), false)
	if err != nil {
		t.Fatalf("InvokeTool: %v", err)
	}
	if res.Informed {
		t.Error("Informed = true, want false")
	}
	if got := sess.takeSeedContext(); got != "" {
		t.Errorf("seedContext = %q, want empty when inform_model is false", got)
	}
}

// TestChatService_InvokeTool_ProfileFilter_UnknownToolOffCatalogue pins the
// profile-catalogue filter: project_status is deliberately excluded from
// t0-decider's allowlist (profiles.go), so a t0-decider chat invoking it must
// see the same ErrToolNotFound an unregistered tool name would.
func TestChatService_InvokeTool_ProfileFilter_UnknownToolOffCatalogue(t *testing.T) {
	t.Parallel()
	svc, _ := newInvokableChatTestService(t)
	sid, err := svc.Create("claude", "", "", chatTestProjectID, "", "t0-decider", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = svc.InvokeTool(context.Background(), sid, "project_status", json.RawMessage(`{}`), false)
	if err != ErrToolNotFound {
		t.Errorf("err = %v, want ErrToolNotFound (project_status is off the t0-decider catalogue)", err)
	}
}
