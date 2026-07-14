package spawner

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
)

// newAPIEngineTestPool copies the package's pre-migrated template DB (see
// testmain_test.go) instead of running migrations per-test.
func newAPIEngineTestPool(t *testing.T) (*db.Pool, clock.Clock) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "api_engine_test.db")
	copyTemplateDB(t, dbPath)
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("OpenPoolExisting: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool, clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
}

// installFakeAPIProvider swaps the newConsoleAPIProvider test seam to return
// prov (or err), recording every call's args, and restores the original on
// cleanup. newConsoleAPIProvider is a shared package-level var, so tests using
// this seam must not run with t.Parallel().
func installFakeAPIProvider(t *testing.T, prov provider.Provider, err error) *fakeAPIProviderCalls {
	t.Helper()
	calls := &fakeAPIProviderCalls{}
	orig := newConsoleAPIProvider
	newConsoleAPIProvider = func(ctx context.Context, pool *db.Pool, clk clock.Clock, providerName, projectID string) (provider.Provider, error) {
		calls.mu.Lock()
		calls.n++
		calls.mu.Unlock()
		if err != nil {
			return nil, err
		}
		return prov, nil
	}
	t.Cleanup(func() { newConsoleAPIProvider = orig })
	return calls
}

type fakeAPIProviderCalls struct {
	mu sync.Mutex
	n  int
}

func (c *fakeAPIProviderCalls) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

// seedAPIEngineSession inserts a project row and a kind='console_chat'
// agent_sessions row for sessionID — agent_messages.session_id FKs to
// agent_sessions(id), so tests that pre-seed an invoke row need a real
// session row to attach it to.
func seedAPIEngineSession(t *testing.T, pool *db.Pool, clk clock.Clock, projectID, sessionID string) {
	t.Helper()
	now := clk.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(`INSERT OR IGNORE INTO projects (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		projectID, projectID, now, now); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	row := &model.AgentSession{
		ID:        sessionID,
		ProjectID: projectID,
		Phase:     "console_chat",
		NodeID:    "console_chat",
		AgentType: "console_chat",
		Status:    model.AgentSessionUserInteractive,
		Kind:      model.AgentSessionKindConsoleChat,
	}
	if err := repo.NewAgentSessionRepo(pool, clk).Create(row); err != nil {
		t.Fatalf("seed agent_sessions row: %v", err)
	}
}

func apiTestSpec(sessionID string) EngineSpec {
	return EngineSpec{
		SessionID:   sessionID,
		ProjectID:   "p1",
		Model:       "claude-x",
		APIProvider: "anthropic",
		MaxContext:  1000,
	}
}

// TestAPIConsoleEngine_Start_APIModeDisabled_ReturnsSentinelWithoutBuildingProvider
// verifies that with api_mode_enabled unset, Start returns
// service.ErrAPIModeDisabled (errors.Is) and the provider seam is never called.
func TestAPIConsoleEngine_Start_APIModeDisabled_ReturnsSentinelWithoutBuildingProvider(t *testing.T) {
	pool, clk := newAPIEngineTestPool(t)
	calls := installFakeAPIProvider(t, mock.New(), nil)

	sink := &testSink{}
	eng := newAPIConsoleEngine(EngineDeps{
		Sink: sink,
		API:  APIEngineDeps{Pool: pool, Clock: clk},
	})

	err := eng.Start(context.Background(), apiTestSpec("sess-disabled"))
	if !errors.Is(err, service.ErrAPIModeDisabled) {
		t.Fatalf("Start error = %v, want service.ErrAPIModeDisabled", err)
	}
	if got := calls.count(); got != 0 {
		t.Errorf("provider seam called %d times, want 0 (gate must short-circuit before building a provider)", got)
	}
}

// TestAPIConsoleEngine_HappyPath_EmitsTurnLifecycleAndKeepsEventsOpen verifies
// the event sequence turn_started -> text_delta(s) -> turn_completed, with
// Events() still open afterward.
func TestAPIConsoleEngine_HappyPath_EmitsTurnLifecycleAndKeepsEventsOpen(t *testing.T) {
	pool, clk := newAPIEngineTestPool(t)
	if err := service.NewGlobalSettingsService(pool, clk).Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("seed api_mode_enabled: %v", err)
	}
	prov := mock.New(mock.Script{
		Events: []mock.SinkEvent{
			{Kind: mock.EventText, Text: "hello"},
			{Kind: mock.EventText, Text: " world"},
		},
		Final: provider.FinalResponse{StopReason: "end_turn"},
	})
	installFakeAPIProvider(t, prov, nil)

	sink := &testSink{}
	eng := newAPIConsoleEngine(EngineDeps{
		Sink: sink,
		API:  APIEngineDeps{Pool: pool, Clock: clk},
	})
	if err := eng.Start(context.Background(), apiTestSpec("sess-happy")); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(eng.Stop)

	if err := eng.SendUserTurn(context.Background(), "hi"); err != nil {
		t.Fatalf("SendUserTurn: %v", err)
	}

	events := collectEventsUntil(t, eng.Events(), func(ev EngineEvent) bool {
		return ev.Type == EventTurnCompleted
	}, 2*time.Second)

	if len(events) == 0 || events[0].Type != EventTurnStarted {
		t.Fatalf("events[0] = %+v, want EventTurnStarted first; got %+v", events[0], events)
	}
	var textDeltas []string
	sawCompleted := false
	for _, ev := range events {
		if ev.Type == EventTextDelta {
			textDeltas = append(textDeltas, ev.Text)
		}
		if ev.Type == EventTurnCompleted {
			sawCompleted = true
		}
	}
	if len(textDeltas) == 0 {
		t.Errorf("no EventTextDelta observed; events = %+v", events)
	}
	if strings.Join(textDeltas, "") != "hello world" {
		t.Errorf("joined text deltas = %q, want %q", strings.Join(textDeltas, ""), "hello world")
	}
	if !sawCompleted {
		t.Errorf("no EventTurnCompleted observed; events = %+v", events)
	}
	if events[len(events)-1].Type != EventTurnCompleted {
		t.Errorf("last event = %+v, want EventTurnCompleted", events[len(events)-1])
	}

	// Events() must still be open — a subsequent turn should still be usable.
	select {
	case _, ok := <-eng.Events():
		if !ok {
			t.Fatal("Events() closed after a single turn, want still open")
		}
	default:
		// no buffered event pending, channel is simply empty — expected.
	}
}

// TestAPIConsoleEngine_ProviderError_EmitsEventError verifies a provider
// error surfaces as EventError (the turn's terminal status is not "PASS").
func TestAPIConsoleEngine_ProviderError_EmitsEventError(t *testing.T) {
	pool, clk := newAPIEngineTestPool(t)
	if err := service.NewGlobalSettingsService(pool, clk).Set("api_mode_enabled", "true"); err != nil {
		t.Fatalf("seed api_mode_enabled: %v", err)
	}
	prov := mock.New(mock.Script{Err: errors.New("provider boom")})
	installFakeAPIProvider(t, prov, nil)

	sink := &testSink{}
	eng := newAPIConsoleEngine(EngineDeps{
		Sink: sink,
		API:  APIEngineDeps{Pool: pool, Clock: clk},
	})
	if err := eng.Start(context.Background(), apiTestSpec("sess-provider-err")); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(eng.Stop)

	if err := eng.SendUserTurn(context.Background(), "hi"); err != nil {
		t.Fatalf("SendUserTurn: %v", err)
	}

	ev := waitForEventType(t, eng.Events(), EventError, 2*time.Second)
	if !ev.IsError {
		t.Errorf("EventError.IsError = false, want true")
	}
}
