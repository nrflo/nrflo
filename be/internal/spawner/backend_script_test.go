package spawner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"be/internal/clock"
)

// TestScriptBackend_Identification verifies all interface properties for scriptBackend.
func TestScriptBackend_Identification(t *testing.T) {
	t.Parallel()
	s := New(Config{Clock: clock.NewTest(time.Now())})
	b := newScriptBackend(s)

	if got := b.Name(); got != "script" {
		t.Errorf("Name() = %q, want %q", got, "script")
	}
	if b.SupportsResume() {
		t.Errorf("SupportsResume() = true, want false")
	}
	if b.SupportsTakeControl() {
		t.Errorf("SupportsTakeControl() = true, want false")
	}
	if b.RequiresPrompt() {
		t.Errorf("RequiresPrompt() = true, want false")
	}
	if b.TracksContext() {
		t.Errorf("TracksContext() = true, want false")
	}
	if b.ParsesStructuredOutput() {
		t.Errorf("ParsesStructuredOutput() = true, want false")
	}
}

// TestScriptBackend_Kill_NilSafe verifies Kill is a no-op when proc has no cmd
// or the cmd was never started (nil Process).
func TestScriptBackend_Kill_NilSafe(t *testing.T) {
	t.Parallel()
	s := New(Config{Clock: clock.NewTest(time.Now())})
	b := newScriptBackend(s)
	ctx := context.Background()

	proc := &processInfo{}
	if err := b.Kill(ctx, proc, syscall.SIGTERM); err != nil {
		t.Errorf("Kill(nil cmd) = %v, want nil", err)
	}

	proc.cmd = exec.Command("sleep", "60")
	if proc.cmd.Process != nil {
		t.Fatalf("expected un-started cmd to have nil Process")
	}
	if err := b.Kill(ctx, proc, syscall.SIGKILL); err != nil {
		t.Errorf("Kill(unstarted cmd) = %v, want nil", err)
	}
}

// TestScriptBackend_Kill_SIGTERM verifies SIGTERM signals a running process.
func TestScriptBackend_Kill_SIGTERM(t *testing.T) {
	t.Parallel()
	s := New(Config{Clock: clock.NewTest(time.Now())})
	b := newScriptBackend(s)

	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	proc := &processInfo{cmd: cmd}
	if err := b.Kill(context.Background(), proc, syscall.SIGTERM); err != nil {
		t.Fatalf("Kill SIGTERM: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("process did not exit within 2s after SIGTERM")
	}
}

// TestScriptBackend_Kill_SIGKILL verifies SIGKILL routes to Process.Kill().
func TestScriptBackend_Kill_SIGKILL(t *testing.T) {
	t.Parallel()
	s := New(Config{Clock: clock.NewTest(time.Now())})
	b := newScriptBackend(s)

	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	proc := &processInfo{cmd: cmd}
	if err := b.Kill(context.Background(), proc, syscall.SIGKILL); err != nil {
		t.Fatalf("Kill SIGKILL: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("process did not exit within 2s after SIGKILL")
	}
}

// TestScriptBackend_ProcessOutput_JSONLineTrackedAsText verifies that a JSON-shaped
// stdout line is NOT parsed — it is tracked as-is with category "text". This confirms
// ParsesStructuredOutput=false is respected by processOutput.
func TestScriptBackend_ProcessOutput_JSONLineTrackedAsText(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})
	b := newScriptBackend(s)

	proc := &processInfo{backend: b}
	jsonLine := `{"type":"assistant","message":{"content":[]}}`
	s.processOutput(proc, jsonLine)

	if len(proc.pendingMessages) != 1 {
		t.Fatalf("pendingMessages = %d, want 1", len(proc.pendingMessages))
	}
	if proc.pendingMessages[0].Content != jsonLine {
		t.Errorf("Content = %q, want %q", proc.pendingMessages[0].Content, jsonLine)
	}
	if proc.pendingMessages[0].Category != "text" {
		t.Errorf("Category = %q, want %q", proc.pendingMessages[0].Category, "text")
	}
}

// TestScriptBackend_ProcessOutput_PlainTextLine verifies that plain text stdout
// is tracked as a "text" message.
func TestScriptBackend_ProcessOutput_PlainTextLine(t *testing.T) {
	t.Parallel()
	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})
	b := newScriptBackend(s)

	proc := &processInfo{backend: b}
	s.processOutput(proc, "hello from script")

	if len(proc.pendingMessages) != 1 {
		t.Fatalf("pendingMessages = %d, want 1", len(proc.pendingMessages))
	}
	if proc.pendingMessages[0].Content != "hello from script" {
		t.Errorf("Content = %q, want %q", proc.pendingMessages[0].Content, "hello from script")
	}
	if proc.pendingMessages[0].Category != "text" {
		t.Errorf("Category = %q, want text", proc.pendingMessages[0].Category)
	}
}

// newScriptProc builds a minimal processInfo for script backend Start tests.
// sessionID must be unique across parallel tests to avoid /tmp file collisions.
func newScriptProc(sessionID string) *processInfo {
	return &processInfo{
		sessionID:    sessionID,
		agentType:    "test-script-agent",
		modelID:      "script:ps-1",
		doneCh:       make(chan struct{}),
		pendingTasks: make(map[string]taskInfo),
	}
}

// newScriptPrep builds a prepResult for the script backend with the given Python code.
func newScriptPrep(t *testing.T, code string) *prepResult {
	t.Helper()
	return &prepResult{
		executionMode: "script",
		scriptCode:    code,
		scriptID:      "ps-1",
		opts: SpawnOptions{
			WorkDir: t.TempDir(),
		},
	}
}

// TestScriptBackend_Start_ExitZeroClosesDoneCh verifies a successful script (exit 0)
// closes doneCh and leaves waitErr nil. Also checks the temp file is removed.
func TestScriptBackend_Start_ExitZeroClosesDoneCh(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not in PATH")
	}

	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})
	b := newScriptBackend(s)

	proc := newScriptProc("script-test-exit0")
	proc.backend = b
	prep := newScriptPrep(t, "import sys; sys.exit(0)")
	scriptPath := filepath.Join("/tmp/nrflo/scripts", proc.sessionID+".py")

	if err := b.Start(context.Background(), proc, prep); err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case <-proc.doneCh:
	case <-time.After(5 * time.Second):
		t.Fatalf("doneCh not closed within 5s")
	}

	if proc.waitErr != nil {
		t.Errorf("waitErr = %v, want nil for exit(0)", proc.waitErr)
	}

	// The wait goroutine removes the file after closing doneCh — poll briefly.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := os.Stat(scriptPath); !os.IsNotExist(err) {
		t.Errorf("script file %s should be removed after exit, stat err: %v", scriptPath, err)
	}
}

// TestScriptBackend_Start_ExitNonZero_SetsWaitErr verifies a script that exits
// non-zero sets proc.waitErr.
func TestScriptBackend_Start_ExitNonZero_SetsWaitErr(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not in PATH")
	}

	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})
	b := newScriptBackend(s)

	proc := newScriptProc("script-test-exit1")
	proc.backend = b
	prep := newScriptPrep(t, "import sys; sys.exit(1)")

	if err := b.Start(context.Background(), proc, prep); err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case <-proc.doneCh:
	case <-time.After(5 * time.Second):
		t.Fatalf("doneCh not closed within 5s")
	}

	if proc.waitErr == nil {
		t.Errorf("waitErr = nil, want non-nil for exit(1)")
	}
}

// TestScriptBackend_Start_StdoutTrackedAsText verifies that stdout lines emitted
// by the script are tracked as plain "text" messages. A JSON-shaped line must also
// land as plain text (confirming ParsesStructuredOutput=false end-to-end).
func TestScriptBackend_Start_StdoutTrackedAsText(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not in PATH")
	}

	clk := clock.NewTest(time.Now())
	s := New(Config{Clock: clk})
	b := newScriptBackend(s)

	proc := newScriptProc("script-test-stdout")
	proc.backend = b
	// Emit both a plain line and a JSON-shaped line.
	code := `import json; print("plain line"); print(json.dumps({"type":"assistant"}))`
	prep := newScriptPrep(t, code)

	if err := b.Start(context.Background(), proc, prep); err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case <-proc.doneCh:
	case <-time.After(5 * time.Second):
		t.Fatalf("doneCh not closed within 5s")
	}

	// Poll briefly for monitorOutput goroutine to drain remaining output.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		proc.messagesMutex.Lock()
		n := len(proc.pendingMessages)
		proc.messagesMutex.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(time.Millisecond)
	}

	proc.messagesMutex.Lock()
	msgs := make([]string, len(proc.pendingMessages))
	cats := make([]string, len(proc.pendingMessages))
	for i, m := range proc.pendingMessages {
		msgs[i] = m.Content
		cats[i] = m.Category
	}
	proc.messagesMutex.Unlock()

	foundPlain, foundJSON := false, false
	for i, m := range msgs {
		if cats[i] != "text" {
			t.Errorf("message %d category = %q, want text", i, cats[i])
		}
		if m == "plain line" {
			foundPlain = true
		}
		if strings.HasPrefix(m, `{"type"`) {
			foundJSON = true
		}
	}
	if !foundPlain {
		t.Errorf("expected 'plain line' in messages; got %v", msgs)
	}
	if !foundJSON {
		t.Errorf("expected JSON-shaped line tracked as text; got %v", msgs)
	}
}
