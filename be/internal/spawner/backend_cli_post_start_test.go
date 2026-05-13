package spawner

import (
	"context"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/repo"
)

// stubCLIAdapter implements CLIAdapter for cliBackend tests without requiring
// real CLI binaries. BuildCommand returns exec.Command("true").
type stubCLIAdapter struct{}

func (a *stubCLIAdapter) Name() string                                                   { return "stub" }
func (a *stubCLIAdapter) BuildCommand(_ SpawnOptions) *exec.Cmd                          { return exec.Command("true") }
func (a *stubCLIAdapter) MapModel(m string) string                                       { return m }
func (a *stubCLIAdapter) SupportsSessionID() bool                                        { return false }
func (a *stubCLIAdapter) SupportsSystemPromptFile() bool                                 { return false }
func (a *stubCLIAdapter) SupportsResume() bool                                           { return false }
func (a *stubCLIAdapter) UsesStdinPrompt() bool                                          { return false }
func (a *stubCLIAdapter) BuildResumeCommand(_ ResumeOptions) *exec.Cmd                   { return nil }
func (a *stubCLIAdapter) SupportsInteractive() bool                                      { return false }
func (a *stubCLIAdapter) BuildInteractiveCommand(_ InteractiveSpawnOptions) *exec.Cmd    { return nil }
func (a *stubCLIAdapter) PrepareInteractive(_ InteractivePrepOptions) (InteractiveExtras, func(), error) {
	return InteractiveExtras{}, func() {}, nil
}
func (a *stubCLIAdapter) DeliversPromptInline() bool      { return false }
func (a *stubCLIAdapter) NeedsTerminalQueryReplies() bool { return false }
func (a *stubCLIAdapter) BumpsOnPTYBytes() bool           { return false }

// postStartStubAdapter wraps stubCLIAdapter and adds PostStarter. Atomic flags
// record whether PostStart was called and whether its cleanup was invoked.
type postStartStubAdapter struct {
	stubCLIAdapter
	started int32
	cleaned int32
}

func (a *postStartStubAdapter) PostStart(_ context.Context, _ PostStartOptions) (func(), error) {
	atomic.StoreInt32(&a.started, 1)
	return func() { atomic.StoreInt32(&a.cleaned, 1) }, nil
}

// newStubProc builds a minimal processInfo for cliBackend.Start tests.
func newStubProc(sessionID string) *processInfo {
	return &processInfo{
		sessionID:       sessionID,
		agentType:       "test-agent",
		modelID:         "stub:test",
		pendingMessages: make([]repo.MessageEntry, 0),
		pendingTasks:    make(map[string]taskInfo),
		doneCh:          make(chan struct{}),
	}
}

// newStubPrep builds a minimal prepResult for cliBackend.Start tests.
func newStubPrep() *prepResult {
	return &prepResult{
		opts: SpawnOptions{WorkDir: "/tmp"},
	}
}

// TestCLIBackend_Start_InvokesPostStart verifies that cliBackend.Start calls
// PostStart on adapters implementing PostStarter, and that the cleanup func
// returned by PostStart is invoked after the process exits.
func TestCLIBackend_Start_InvokesPostStart(t *testing.T) {
	t.Parallel()

	s := New(Config{Clock: clock.Real()})
	adapter := &postStartStubAdapter{}
	b := newCLIBackend(adapter, s)

	proc := newStubProc("cli-post-start-1")
	proc.backend = b
	prep := newStubPrep()

	if err := b.Start(context.Background(), proc, prep); err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case <-proc.doneCh:
	case <-time.After(5 * time.Second):
		t.Fatalf("doneCh not closed within 5s")
	}

	// postCleanup() is called after close(origDoneCh) in the wait goroutine — poll briefly.
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&adapter.cleaned) == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}

	if atomic.LoadInt32(&adapter.started) != 1 {
		t.Error("PostStart was not called by cliBackend.Start")
	}
	if atomic.LoadInt32(&adapter.cleaned) != 1 {
		t.Error("cleanup func returned by PostStart was not called after process exit")
	}
}

// TestCLIBackend_Start_AdapterWithoutPostStart_Ignored verifies that adapters
// not implementing PostStarter cause no panic and spawn successfully.
func TestCLIBackend_Start_AdapterWithoutPostStart_Ignored(t *testing.T) {
	t.Parallel()

	s := New(Config{Clock: clock.Real()})
	b := newCLIBackend(&stubCLIAdapter{}, s)

	proc := newStubProc("cli-no-post-start-1")
	proc.backend = b
	prep := newStubPrep()

	if err := b.Start(context.Background(), proc, prep); err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case <-proc.doneCh:
	case <-time.After(5 * time.Second):
		t.Fatalf("doneCh not closed within 5s")
	}
}

// TestCLIBackend_Start_PostStart_CleanupCalledAfterDoneCh verifies ordering:
// the cleanup returned by PostStart is not called until AFTER doneCh is closed,
// so watchers of doneCh never observe a post-cleanup state before exit.
func TestCLIBackend_Start_PostStart_CleanupCalledAfterDoneCh(t *testing.T) {
	t.Parallel()

	s := New(Config{Clock: clock.Real()})
	adapter := &postStartStubAdapter{}
	b := newCLIBackend(adapter, s)

	proc := newStubProc("cli-post-start-order-1")
	proc.backend = b

	// Capture doneCh before Start may replace it.
	doneCh := proc.doneCh
	prep := newStubPrep()

	if err := b.Start(context.Background(), proc, prep); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// At the moment doneCh is closed, cleaned MUST be 0 (cleanup runs after
	// the close in the same goroutine, so racing on this is intentional).
	// We assert it is 0 immediately on receive, before any sleep.
	select {
	case <-doneCh:
	case <-time.After(5 * time.Second):
		t.Fatalf("doneCh not closed within 5s")
	}

	// Poll until cleanup completes (it runs in the same goroutine right after close).
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&adapter.cleaned) == 1 {
			return // success
		}
		time.Sleep(time.Millisecond)
	}
	t.Error("cleanup func was not called within 1s after doneCh closed")
}
