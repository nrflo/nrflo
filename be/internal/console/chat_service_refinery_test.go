package console

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	ptyPkg "be/internal/pty"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/ws"
)

// fakeRefineryLifecycle records Start/Stop/Flush calls so tests can assert
// ChatService's wiring without a real refinery.Manager.
type fakeRefineryLifecycle struct {
	mu      sync.Mutex
	started []string // sessionID
	stopped []string // sessionID
	flushed []string // sessionID
	touched []string // sessionID

	// onFlush, when set, is invoked outside the lock on every Flush call so a
	// test can simulate "the flush folded a digest" (e.g. upserting into
	// refinery_digests) without a real sidecar.
	onFlush func(sessionID string)
}

func (f *fakeRefineryLifecycle) Start(sessionID, projectID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started = append(f.started, sessionID)
}

func (f *fakeRefineryLifecycle) Stop(sessionID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped = append(f.stopped, sessionID)
}

func (f *fakeRefineryLifecycle) Flush(ctx context.Context, sessionID string) {
	f.mu.Lock()
	f.flushed = append(f.flushed, sessionID)
	f.mu.Unlock()
	if f.onFlush != nil {
		f.onFlush(sessionID)
	}
}

func (f *fakeRefineryLifecycle) Touch(sessionID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.touched = append(f.touched, sessionID)
}

func (f *fakeRefineryLifecycle) touchCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.touched)
}

func (f *fakeRefineryLifecycle) startCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.started)
}

func (f *fakeRefineryLifecycle) stopCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.stopped)
}

func (f *fakeRefineryLifecycle) flushCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.flushed)
}

// newChatTestServiceWithRefinery mirrors newChatTestService but wires a
// caller-supplied RefineryLifecycle (nil-able) into ChatDeps, so lifecycle
// tests can assert Start/Stop calls without a real refinery.Manager/DB fold.
func newChatTestServiceWithRefinery(t *testing.T, mgr RefineryLifecycle) (*ChatService, *db.Pool, *ws.Hub, *fakeEngineFactory) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "chat_test_refinery.db")
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
		Pool:        pool,
		Clock:       clk,
		WSHub:       hub,
		PTY:         ptyPkg.NewManager(),
		Hub:         spawner.NewConsoleHub(),
		ServerURL:   "http://127.0.0.1:6587",
		RefineryMgr: mgr,
	})
	factory := &fakeEngineFactory{}
	svc.SetEngineFactory(factory.factory)

	return svc, pool, hub, factory
}

func TestChatService_RefineryEnabled_StartsOnCreateAndStopsOnClose(t *testing.T) {
	t.Parallel()
	mgr := &fakeRefineryLifecycle{}
	svc, _, _, _ := newChatTestServiceWithRefinery(t, mgr)

	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", true)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if mgr.startCount() != 1 {
		t.Fatalf("Start calls after Create(refineryEnabled=true) = %d, want 1", mgr.startCount())
	}

	if err := svc.Close(sid); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if mgr.stopCount() != 1 {
		t.Errorf("Stop calls after Close = %d, want 1", mgr.stopCount())
	}
}

// TestChatService_RefineryDisabled_NeverStarts verifies a disabled chat never
// starts a sidecar. Close still issues an unconditional Stop when a
// RefineryMgr is wired at all (relying on RefineryLifecycle.Stop being a
// documented no-op for a session that was never Started, mirroring
// refinery.Manager.Stop) — so Stop firing here is not a fold ever having run.
func TestChatService_RefineryDisabled_NeverStarts(t *testing.T) {
	t.Parallel()
	mgr := &fakeRefineryLifecycle{}
	svc, _, _, _ := newChatTestServiceWithRefinery(t, mgr)

	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if mgr.startCount() != 0 {
		t.Errorf("Start calls after Create(refineryEnabled=false) = %d, want 0", mgr.startCount())
	}

	if err := svc.Close(sid); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// TestChatService_RefineryGlobalSetting_EnablesWithoutPerCreateFlag verifies
// the global refinery_enabled setting turns the sidecar on even when the
// per-create param is false.
func TestChatService_RefineryGlobalSetting_EnablesWithoutPerCreateFlag(t *testing.T) {
	t.Parallel()
	mgr := &fakeRefineryLifecycle{}
	svc, pool, _, _ := newChatTestServiceWithRefinery(t, mgr)
	if err := service.NewGlobalSettingsService(pool, svc.deps.Clock).Set("refinery_enabled", "true"); err != nil {
		t.Fatalf("set global refinery_enabled: %v", err)
	}

	if _, err := svc.Create("codex", "", "", chatTestProjectID, "", "", false); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if mgr.startCount() != 1 {
		t.Errorf("Start calls with global refinery_enabled=true = %d, want 1", mgr.startCount())
	}
}

// TestChatService_RefineryStop_FiresOnEngineExit verifies the sidecar is also
// torn down when the engine dies on its own (not via Close).
func TestChatService_RefineryStop_FiresOnEngineExit(t *testing.T) {
	t.Parallel()
	mgr := &fakeRefineryLifecycle{}
	svc, _, hub, factory := newChatTestServiceWithRefinery(t, mgr)

	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", true)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if mgr.startCount() != 1 {
		t.Fatalf("Start calls after Create = %d, want 1", mgr.startCount())
	}

	eng := factory.last()
	ch := subscribeChatSession(t, hub, sid)
	eng.Stop() // the engine dies on its own — nobody called Close
	waitForChatTurnState(t, ch, "idle", 2*time.Second)

	if mgr.stopCount() != 1 {
		t.Errorf("Stop calls after engine exit = %d, want 1", mgr.stopCount())
	}
}

// TestChatService_NilRefineryMgr_IsANoop verifies a nil ChatDeps.RefineryMgr
// (the default for tests/wiring that never set it) never panics on
// Create/Close, regardless of the refineryEnabled flag.
func TestChatService_NilRefineryMgr_IsANoop(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newChatTestServiceWithRefinery(t, nil)

	sid, err := svc.Create("codex", "", "", chatTestProjectID, "", "", true)
	if err != nil {
		t.Fatalf("Create with nil RefineryMgr: %v", err)
	}
	if err := svc.Close(sid); err != nil {
		t.Fatalf("Close with nil RefineryMgr: %v", err)
	}
}

// TestChatSink_RecordHookMessage_TouchesRefineryOnce verifies
// chatSink.RecordHookMessage is the single choke point that touches the
// session's refinery sidecar after a message row lands: one persisted
// engine message must yield exactly one Touch call.
func TestChatSink_RecordHookMessage_TouchesRefineryOnce(t *testing.T) {
	t.Parallel()
	mgr := &fakeRefineryLifecycle{}
	_, pool, _, _ := newChatTestServiceWithRefinery(t, mgr)

	sessionID := "sess-touch-1"
	insertTestAgentSession(t, pool, sessionID, chatTestProjectID)

	sink := &chatSink{pool: pool, clock: clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), sessionID: sessionID, projectID: chatTestProjectID, refinery: mgr}
	if _, _, _, err := sink.RecordHookMessage(sessionID, "hello", "text", ""); err != nil {
		t.Fatalf("RecordHookMessage: %v", err)
	}

	if got := mgr.touchCount(); got != 1 {
		t.Errorf("touchCount after one RecordHookMessage = %d, want 1", got)
	}
}

// TestChatSink_RecordHookMessage_NilRefineryIsNoop verifies a chatSink built
// with a nil refinery field (the zero value, mirroring an unwired
// ChatDeps.RefineryMgr) never panics on RecordHookMessage.
func TestChatSink_RecordHookMessage_NilRefineryIsNoop(t *testing.T) {
	t.Parallel()
	_, pool, _, _ := newChatTestServiceWithRefinery(t, nil)

	sessionID := "sess-touch-2"
	insertTestAgentSession(t, pool, sessionID, chatTestProjectID)

	sink := &chatSink{pool: pool, clock: clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), sessionID: sessionID, projectID: chatTestProjectID, refinery: nil}
	if _, _, _, err := sink.RecordHookMessage(sessionID, "hello", "text", ""); err != nil {
		t.Fatalf("RecordHookMessage with nil refinery: %v", err)
	}
}

// insertTestAgentSession seeds the minimal agent_sessions row
// RecordHookMessage's agent_messages insert needs (no FK on agent_messages
// itself, but keeps the fixture representative of a real console_chat row).
func insertTestAgentSession(t *testing.T, pool *db.Pool, sessionID, projectID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, pool, `INSERT INTO agent_sessions (id, project_id, ticket_id, phase, node_id, agent_type, status, kind, created_at, updated_at)
		 VALUES (?, ?, '', 'console_chat', 'console_chat', 'console_chat', 'user_interactive', 'console_chat', ?, ?)`,
		sessionID, projectID, now, now)
}
