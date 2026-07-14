package spawner

import (
	"context"
	"testing"
	"time"
)

// startTestClaudeEngine constructs a claudeEngine wired to a mockPtyManager
// (bypassing wrapPtyManager's *pty.Manager requirement, mirroring how
// backend_interactive tests inject ptyManagerIface directly) with every
// injectable timeout collapsed to test-safe values, then calls Start.
// Registers Stop as cleanup (idempotent — safe if a test also calls it).
func startTestClaudeEngine(t *testing.T, sink Sink, hub *ConsoleHub, spec EngineSpec) (*claudeEngine, *mockPtyManager) {
	t.Helper()
	mgr := newMockPtyManager()
	e := &claudeEngine{
		sink:                sink,
		hub:                 hub,
		nrfloPath:           "/opt/nrflo_server",
		pty:                 mgr,
		events:              make(chan EngineEvent, 256),
		ferryDone:           make(chan struct{}),
		tailDone:            make(chan struct{}),
		stopping:            make(chan struct{}),
		readyCh:             make(chan struct{}),
		approvals:           newClaudeApprovals(),
		approvalTimeout:     50 * time.Millisecond,
		sessionStartTimeout: 20 * time.Millisecond,
		bootstrapFloor:      0,
		tailInterval:        5 * time.Millisecond,
	}
	if spec.SessionID == "" {
		spec.SessionID = "sess-console-claude-1"
	}
	if spec.WorkDir == "" {
		spec.WorkDir = t.TempDir()
	}
	if err := e.Start(context.Background(), spec); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(e.Stop)
	return e, mgr
}

func TestClaudeEngine_Start_NoPTYManager_Errors(t *testing.T) {
	e := &claudeEngine{events: make(chan EngineEvent, 4), stopping: make(chan struct{})}
	if err := e.Start(context.Background(), EngineSpec{SessionID: "s", WorkDir: t.TempDir()}); err == nil {
		t.Error("expected an error when no PTY manager is configured")
	}
}

// TestClaudeEngine_Start_ArgvExcludesManagedSessionFlags mirrors
// cli_adapter_claude_disallowed_test.go's shape, inverted: a console session
// argv must NOT carry any managed-spawn flag.
func TestClaudeEngine_Start_ArgvExcludesManagedSessionFlags(t *testing.T) {
	sink := &testSink{}
	e, mgr := startTestClaudeEngine(t, sink, nil, EngineSpec{SessionID: "sess-argv-1", Model: "claude-opus-4-8"})

	launch := mgr.registeredLaunches[e.spec.SessionID]
	if launch.Command != "claude" {
		t.Errorf("Command = %q, want claude", launch.Command)
	}
	for _, banned := range []string{"--dangerously-skip-permissions", "--disallowedTools", "--strict-mcp-config"} {
		if findArgElement(launch.Args, banned) != -1 {
			t.Errorf("console argv %v must not contain managed-session flag %q", launch.Args, banned)
		}
	}
	pos := findArgElement(launch.Args, "--session-id")
	if pos == -1 || pos+1 >= len(launch.Args) || launch.Args[pos+1] != "sess-argv-1" {
		t.Errorf("argv %v missing --session-id sess-argv-1", launch.Args)
	}
	modelPos := findArgElement(launch.Args, "--model")
	if modelPos == -1 || modelPos+1 >= len(launch.Args) || launch.Args[modelPos+1] != "claude-opus-4-8" {
		t.Errorf("argv %v missing --model claude-opus-4-8", launch.Args)
	}
	settingsPos := findArgElement(launch.Args, "--settings")
	if settingsPos == -1 || settingsPos+1 >= len(launch.Args) {
		t.Fatalf("argv %v missing --settings <json>", launch.Args)
	}
	if launch.Args[settingsPos+1] != BuildConsoleSettingsJSON(e.nrfloPath) {
		t.Error("--settings value does not match BuildConsoleSettingsJSON(nrfloPath)")
	}
	if findArgElement(launch.Args, "--mcp-config") == -1 {
		t.Errorf("argv %v missing --mcp-config", launch.Args)
	}
}

func TestClaudeEngine_Start_ArgvOptionalFlags(t *testing.T) {
	sink := &testSink{}
	e, mgr := startTestClaudeEngine(t, sink, nil, EngineSpec{
		SessionID:       "sess-argv-2",
		ReasoningEffort: "xhigh",
		FallbackModels:  "sonnet,haiku",
	})
	launch := mgr.registeredLaunches[e.spec.SessionID]
	for _, want := range [][2]string{{"--effort", "xhigh"}, {"--fallback-model", "sonnet,haiku"}} {
		pos := findArgElement(launch.Args, want[0])
		if pos == -1 || pos+1 >= len(launch.Args) || launch.Args[pos+1] != want[1] {
			t.Errorf("argv %v missing %s %s", launch.Args, want[0], want[1])
		}
	}
}

func TestClaudeEngine_Start_RegistersWithHub(t *testing.T) {
	hub := NewConsoleHub()
	e, _ := startTestClaudeEngine(t, &testSink{}, hub, EngineSpec{})
	if _, ok := hub.get(e.spec.SessionID); !ok {
		t.Error("engine not registered with hub after Start")
	}
}

func TestClaudeEngine_SendUserTurn_NotStarted_Errors(t *testing.T) {
	e := &claudeEngine{events: make(chan EngineEvent, 4), stopping: make(chan struct{})}
	if err := e.SendUserTurn(context.Background(), "x"); err == nil {
		t.Error("expected an error when SendUserTurn is called before Start")
	}
}

// TestClaudeEngine_SendUserTurn_AlreadyReady_WritesPromptly asserts that once
// SessionStart has already fired, SendUserTurn does not wait out the full
// sessionStartTimeout before writing.
func TestClaudeEngine_SendUserTurn_AlreadyReady_WritesPromptly(t *testing.T) {
	sink := &testSink{}
	e, mgr := startTestClaudeEngine(t, sink, nil, EngineSpec{})
	e.sessionStartTimeout = 500 * time.Millisecond
	e.NotifySessionReady()

	start := time.Now()
	if err := e.SendUserTurn(context.Background(), "hello claude"); err != nil {
		t.Fatalf("SendUserTurn: %v", err)
	}
	if elapsed := time.Since(start); elapsed >= e.sessionStartTimeout {
		t.Errorf("SendUserTurn took %v, want well under sessionStartTimeout=%v since SessionStart already fired", elapsed, e.sessionStartTimeout)
	}

	sess := mgr.sessions[e.spec.SessionID]
	if got := string(sess.writtenBytes); got != "hello claude\r" {
		t.Errorf("PTY bytes = %q, want %q", got, "hello claude\r")
	}
}

// TestClaudeEngine_SendUserTurn_WaitsForSessionStartTimeout asserts that
// without a ready signal, SendUserTurn blocks for sessionStartTimeout before
// falling back and writing anyway.
func TestClaudeEngine_SendUserTurn_WaitsForSessionStartTimeout(t *testing.T) {
	sink := &testSink{}
	e, mgr := startTestClaudeEngine(t, sink, nil, EngineSpec{})
	e.sessionStartTimeout = 25 * time.Millisecond

	start := time.Now()
	if err := e.SendUserTurn(context.Background(), "hi"); err != nil {
		t.Fatalf("SendUserTurn: %v", err)
	}
	if elapsed := time.Since(start); elapsed < e.sessionStartTimeout {
		t.Errorf("SendUserTurn returned after %v, want to wait at least sessionStartTimeout=%v with no SessionStart signal", elapsed, e.sessionStartTimeout)
	}

	sess := mgr.sessions[e.spec.SessionID]
	if got := string(sess.writtenBytes); got != "hi\r" {
		t.Errorf("PTY bytes = %q, want %q", got, "hi\r")
	}
}

func TestClaudeEngine_SendUserTurn_EmitsTurnStartedAndPersistsUserInput(t *testing.T) {
	sink := &testSink{}
	e, _ := startTestClaudeEngine(t, sink, nil, EngineSpec{})
	e.NotifySessionReady()

	if err := e.SendUserTurn(context.Background(), "do the thing"); err != nil {
		t.Fatalf("SendUserTurn: %v", err)
	}
	ev := waitForEventType(t, e.Events(), EventTurnStarted, time.Second)
	if ev.SessionID != e.spec.SessionID {
		t.Errorf("turn_started session = %q, want %q", ev.SessionID, e.spec.SessionID)
	}
	if n := countCategory(sink, "user_input"); n != 1 {
		t.Fatalf("user_input rows = %d, want 1", n)
	}
	var found bool
	for _, m := range sink.recordedMsgs {
		if m.category == "user_input" && m.content == "do the thing" {
			found = true
		}
	}
	if !found {
		t.Errorf("no user_input row with content %q: %+v", "do the thing", sink.recordedMsgs)
	}
}

func TestClaudeEngine_SendUserTurn_ErrTurnActive_NoPTYWriteOnSecondCall(t *testing.T) {
	sink := &testSink{}
	e, mgr := startTestClaudeEngine(t, sink, nil, EngineSpec{})
	e.NotifySessionReady()
	if err := e.SendUserTurn(context.Background(), "first"); err != nil {
		t.Fatalf("first SendUserTurn: %v", err)
	}
	sess := mgr.sessions[e.spec.SessionID]
	before := string(sess.writtenBytes)

	if err := e.SendUserTurn(context.Background(), "second"); err != ErrTurnActive {
		t.Errorf("second SendUserTurn err = %v, want ErrTurnActive", err)
	}
	if got := string(sess.writtenBytes); got != before {
		t.Errorf("PTY bytes changed after rejected second turn: got %q, want unchanged %q", got, before)
	}
}

func TestClaudeEngine_Stop_UnregistersAndClosesEvents(t *testing.T) {
	sink := &testSink{}
	hub := NewConsoleHub()
	e, mgr := startTestClaudeEngine(t, sink, hub, EngineSpec{})
	sessionID := e.spec.SessionID

	if _, ok := hub.get(sessionID); !ok {
		t.Fatal("engine not registered with hub after Start")
	}

	e.Stop()

	if _, ok := hub.get(sessionID); ok {
		t.Error("engine still registered with hub after Stop")
	}
	sess := mgr.sessions[sessionID]
	if sess.closeCnt != 1 {
		t.Errorf("PTY session Close count = %d, want 1", sess.closeCnt)
	}
	if _, ok := <-e.Events(); ok {
		t.Error("Events() should be closed after Stop")
	}
	e.Stop() // must not panic
}

// TestClaudeEngine_StopWithoutStart_DoesNotDeadlock guards teardown when Start
// never ran (or failed before launching its goroutines): the ferry/tailer
// done-channels are closed by those goroutines, so waiting on them
// unconditionally would wedge Stop's caller forever. Start commits `cancel`
// only once it is about to launch them, so cancel gates the waits.
func TestClaudeEngine_StopWithoutStart_DoesNotDeadlock(t *testing.T) {
	e := newClaudeEngine(EngineDeps{Sink: &testSink{}, Hub: NewConsoleHub()})

	done := make(chan struct{})
	go func() {
		defer close(done)
		e.Stop()
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() on a never-started engine deadlocked")
	}
	if _, ok := <-e.Events(); ok {
		t.Error("Events() should be closed after Stop")
	}
}

// TestClaudeEngine_StopAfterFailedStart_DoesNotDeadlock is the same guard for
// the real failure path: Start with no PTY manager errors out before any
// goroutine exists, and Stop must still return.
func TestClaudeEngine_StopAfterFailedStart_DoesNotDeadlock(t *testing.T) {
	e := newClaudeEngine(EngineDeps{Sink: &testSink{}, Hub: NewConsoleHub()})
	if err := e.Start(context.Background(), EngineSpec{SessionID: "sess-no-pty"}); err == nil {
		t.Fatal("Start with no PTY manager: want error, got nil")
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		e.Stop()
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() after a failed Start deadlocked")
	}
}
