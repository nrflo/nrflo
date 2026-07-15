package socket

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"be/internal/clock"
	"be/internal/service"
	"be/internal/types"
)

type fakeConsoleChatCreator struct {
	engine, model, project string
}

func (f *fakeConsoleChatCreator) CreateAuthenticated(engine, model, project string) (string, string, error) {
	f.engine, f.model, f.project = engine, model, project
	return "chat-session-1", "chat-token-1", nil
}

// mintConsole calls console.session over the handler and returns the decoded
// reply plus the raw response.
func mintConsole(t *testing.T, env *handlerTestEnv, params map[string]string) (map[string]string, Response) {
	t.Helper()
	raw, _ := json.Marshal(params)
	resp := env.handler.Handle(Request{ID: "c1", Method: "console.session", Params: raw})
	out := map[string]string{}
	if resp.Error == nil && len(resp.Result) > 0 {
		if err := json.Unmarshal(resp.Result, &out); err != nil {
			t.Fatalf("decode result: %v", err)
		}
	}
	return out, resp
}

// TestConsoleSession_ExplicitProject mints a session for the given project and
// returns a non-empty session id + bearer.
func TestConsoleSession_ExplicitProject(t *testing.T) {
	env := newHandlerTestEnv(t)

	out, resp := mintConsole(t, env, map[string]string{"project": env.project})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if out["project_id"] != env.project {
		t.Errorf("project_id = %q, want %q", out["project_id"], env.project)
	}
	if out["session_id"] == "" || out["token"] == "" {
		t.Errorf("expected non-empty session_id and token, got %+v", out)
	}
	if out["ticket_id"] != "" {
		t.Errorf("ticket_id = %q, want empty", out["ticket_id"])
	}
}

// TestConsoleSession_CwdMatch resolves the project from cwd when no project hint
// is given.
func TestConsoleSession_CwdMatch(t *testing.T) {
	env := newHandlerTestEnv(t)

	root := t.TempDir()
	sub := filepath.Join(root, "sub", "dir")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	projSvc := service.NewProjectService(env.pool, clock.Real())
	if _, err := projSvc.Create("cwd-proj", &types.ProjectCreateRequest{Name: "Cwd Proj", RootPath: root}); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	out, resp := mintConsole(t, env, map[string]string{"cwd": sub})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if out["project_id"] != "cwd-proj" {
		t.Errorf("project_id = %q, want cwd-proj", out["project_id"])
	}
}

// TestConsoleSession_GlobalFallback resolves to the hidden global project when
// neither a project hint nor a matching cwd is available.
func TestConsoleSession_GlobalFallback(t *testing.T) {
	env := newHandlerTestEnv(t)

	// The template DB carries no global project; seed it so the fallback resolves.
	projSvc := service.NewProjectService(env.pool, clock.Real())
	if _, err := projSvc.Create(service.GlobalProjectID, &types.ProjectCreateRequest{Name: "Global"}); err != nil {
		t.Fatalf("seed global project: %v", err)
	}

	out, resp := mintConsole(t, env, map[string]string{"cwd": "/nonexistent/path/xyz"})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if out["project_id"] != service.GlobalProjectID {
		t.Errorf("project_id = %q, want %q", out["project_id"], service.GlobalProjectID)
	}
}

// TestConsoleSession_UnknownProject returns an error for a project that does not
// exist (mirrors the HTTP 404).
func TestConsoleSession_UnknownProject(t *testing.T) {
	env := newHandlerTestEnv(t)

	_, resp := mintConsole(t, env, map[string]string{"project": "does-not-exist"})
	if resp.Error == nil {
		t.Fatal("expected error for unknown project")
	}
}

// TestConsoleSession_TicketHint stores a known ticket and drops an unknown one.
func TestConsoleSession_TicketHint(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.createTicketAndWorkflow(t, "TEST-1")

	known, resp := mintConsole(t, env, map[string]string{"project": env.project, "ticket_id": "TEST-1"})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if known["ticket_id"] != "TEST-1" {
		t.Errorf("ticket_id = %q, want TEST-1", known["ticket_id"])
	}

	unknown, resp := mintConsole(t, env, map[string]string{"project": env.project, "ticket_id": "NOPE-9"})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if unknown["ticket_id"] != "" {
		t.Errorf("unknown ticket_id = %q, want dropped to empty", unknown["ticket_id"])
	}
}

// TestConsoleSession_UnknownAction returns method-not-found for a bad action.
func TestConsoleSession_UnknownAction(t *testing.T) {
	env := newHandlerTestEnv(t)
	resp := env.handler.Handle(Request{ID: "c1", Method: "console.bogus", Params: []byte("{}")})
	if resp.Error == nil || resp.Error.Code != -32601 {
		t.Fatalf("expected -32601 method not found, got %+v", resp.Error)
	}
}

func TestConsoleChat_MintsScopedBearer(t *testing.T) {
	env := newHandlerTestEnv(t)
	creator := &fakeConsoleChatCreator{}
	env.handler.consoleChat = creator
	params, _ := json.Marshal(map[string]string{
		"project": env.project, "engine": "codex", "model": "codex_gpt_high",
	})
	resp := env.handler.Handle(Request{ID: "chat-1", Method: "console.chat", Params: params})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	var result map[string]string
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result["session_id"] != "chat-session-1" || result["token"] != "chat-token-1" {
		t.Fatalf("result = %+v", result)
	}
	if creator.engine != "codex" || creator.model != "codex_gpt_high" || creator.project != env.project {
		t.Fatalf("creator args = engine=%q model=%q project=%q", creator.engine, creator.model, creator.project)
	}
}

var _ ConsoleChatCreator = (*fakeConsoleChatCreator)(nil)
