package console

import (
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	ptyPkg "be/internal/pty"
	"be/internal/repo"
	"be/internal/spawner"
	"be/internal/ws"
)

const chatTestProjectID = "proj-chat-t"

// newChatTestService assembles a ChatService backed by a real (migrated,
// per-test) DB pool and a running ws.Hub, with its engineFactory seam pointed
// at a fakeEngineFactory — no codex/claude binary is ever spawned.
func newChatTestService(t *testing.T) (*ChatService, *db.Pool, *ws.Hub, *fakeEngineFactory) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "chat_test.db")
	if err := copyConsoleTemplateDB(dbPath); err != nil {
		t.Fatalf("copyConsoleTemplateDB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("OpenPoolExisting: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	now := clk.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, pool, `INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		chatTestProjectID, chatTestProjectID, now, now)

	hub := ws.NewHub(clk)
	go hub.Run()
	t.Cleanup(hub.Stop)

	svc := NewChatService(ChatDeps{
		Pool:      pool,
		Clock:     clk,
		WSHub:     hub,
		PTY:       ptyPkg.NewManager(),
		Hub:       spawner.NewConsoleHub(),
		ServerURL: "http://127.0.0.1:6587",
	})
	factory := &fakeEngineFactory{}
	svc.SetEngineFactory(factory.factory)

	return svc, pool, hub, factory
}

func TestChatService_Create_MintsSessionAndStartsEngine(t *testing.T) {
	t.Parallel()
	svc, pool, _, factory := newChatTestService(t)

	sid, err := svc.Create("codex", "", chatTestProjectID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sid == "" {
		t.Fatal("Create returned empty session id")
	}

	row, err := repo.NewAgentSessionRepo(pool, clock.Real()).GetConsoleChat(sid)
	if err != nil {
		t.Fatalf("GetConsoleChat: %v", err)
	}
	if row == nil {
		t.Fatal("GetConsoleChat = nil, want row")
	}
	if row.Kind != model.AgentSessionKindConsoleChat {
		t.Errorf("Kind = %q, want %q", row.Kind, model.AgentSessionKindConsoleChat)
	}
	if row.Status != model.AgentSessionUserInteractive {
		t.Errorf("Status = %q, want user_interactive", row.Status)
	}
	if !row.SpawnToken.Valid || row.SpawnToken.String == "" {
		t.Error("SpawnToken not set")
	}

	eng := factory.last()
	if eng == nil || !eng.started {
		t.Fatal("engine.Start was not called")
	}
	spec := eng.spec()
	if spec.SessionID != sid {
		t.Errorf("spec.SessionID = %q, want %q", spec.SessionID, sid)
	}
	if spec.ProjectID != chatTestProjectID {
		t.Errorf("spec.ProjectID = %q, want %q", spec.ProjectID, chatTestProjectID)
	}
	if spec.MCPEnv["NRFLO_CONSOLE_SESSION_ID"] != sid {
		t.Errorf("MCPEnv[NRFLO_CONSOLE_SESSION_ID] = %q, want %q", spec.MCPEnv["NRFLO_CONSOLE_SESSION_ID"], sid)
	}
	if spec.MCPEnv["NRFLO_CONSOLE_TOKEN"] != row.SpawnToken.String {
		t.Errorf("MCPEnv[NRFLO_CONSOLE_TOKEN] = %q, want the minted spawn token", spec.MCPEnv["NRFLO_CONSOLE_TOKEN"])
	}
	if spec.MCPEnv["NRFLO_PROJECT"] != chatTestProjectID {
		t.Errorf("MCPEnv[NRFLO_PROJECT] = %q, want %q", spec.MCPEnv["NRFLO_PROJECT"], chatTestProjectID)
	}

	if _, ok := svc.get(sid); !ok {
		t.Error("Get(sid) = false after Create, want true")
	}
}

func TestChatService_Create_UnknownProject_ReturnsErrConsoleProjectNotFound(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newChatTestService(t)

	_, err := svc.Create("codex", "", "no-such-project")
	if err == nil {
		t.Fatal("Create with unknown project: want error, got nil")
	}
}

func TestChatService_Close_StopsEngineAndKillsToken(t *testing.T) {
	t.Parallel()
	svc, pool, _, factory := newChatTestService(t)

	sid, err := svc.Create("codex", "", chatTestProjectID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	eng := factory.last()

	if err := svc.Close(sid); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if !eng.isStopped() {
		t.Error("engine was not stopped by Close")
	}
	if _, ok := svc.get(sid); ok {
		t.Error("Get(sid) = true after Close, want false (session removed)")
	}

	row, err := repo.NewAgentSessionRepo(pool, clock.Real()).GetConsoleChat(sid)
	if err != nil {
		t.Fatalf("GetConsoleChat: %v", err)
	}
	if row.Status != model.AgentSessionInteractiveCompleted {
		t.Errorf("Status = %q, want interactive_completed", row.Status)
	}
	token := row.SpawnToken.String
	dead, err := repo.NewAgentSessionRepo(pool, clock.Real()).GetByToken(token)
	if err != nil {
		t.Fatalf("GetByToken: %v", err)
	}
	if dead != nil {
		t.Error("GetByToken(token) after Close = non-nil, want nil (token must die)")
	}
}

func TestChatService_Close_UnknownSession_ReturnsErrChatSessionNotFound(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newChatTestService(t)

	if err := svc.Close("no-such-session"); err != ErrChatSessionNotFound {
		t.Errorf("Close(unknown) = %v, want ErrChatSessionNotFound", err)
	}
}

func TestChatService_StopAll_StopsEveryEngineAndClearsSessions(t *testing.T) {
	t.Parallel()
	svc, _, _, factory := newChatTestService(t)

	sid1, err := svc.Create("codex", "", chatTestProjectID)
	if err != nil {
		t.Fatalf("Create 1: %v", err)
	}
	sid2, err := svc.Create("codex", "", chatTestProjectID)
	if err != nil {
		t.Fatalf("Create 2: %v", err)
	}

	svc.StopAll()

	factory.mu.Lock()
	engines := append([]*fakeConsoleEngine{}, factory.engines...)
	factory.mu.Unlock()
	if len(engines) != 2 {
		t.Fatalf("expected 2 engines created, got %d", len(engines))
	}
	for i, eng := range engines {
		if !eng.isStopped() {
			t.Errorf("engine %d was not stopped by StopAll", i)
		}
	}

	if _, ok := svc.get(sid1); ok {
		t.Error("Get(sid1) = true after StopAll, want false")
	}
	if _, ok := svc.get(sid2); ok {
		t.Error("Get(sid2) = true after StopAll, want false")
	}
}
