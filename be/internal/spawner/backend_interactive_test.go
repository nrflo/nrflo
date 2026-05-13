package spawner

import (
	"context"
	"strings"
	"syscall"
	"testing"
	"time"

	"be/internal/clock"
)

// =============================================================================
// cliInteractiveBackend property tests
// =============================================================================

func TestCLIInteractiveBackend_Name(t *testing.T) {
	t.Parallel()
	b := newCLIInteractiveBackend(&ClaudeAdapter{}, nil, nil)
	if got := b.Name(); got != "cli_interactive" {
		t.Errorf("Name() = %q, want cli_interactive", got)
	}
}

func TestCLIInteractiveBackend_SupportsTakeControl(t *testing.T) {
	t.Parallel()
	b := newCLIInteractiveBackend(&ClaudeAdapter{}, nil, nil)
	if !b.SupportsTakeControl() {
		t.Error("SupportsTakeControl() = false, want true")
	}
}

func TestCLIInteractiveBackend_SupportsResume_MirrorsAdapter(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		adapter CLIAdapter
		want    bool
	}{
		{"claude", &ClaudeAdapter{}, true},
		{"opencode", &OpencodeAdapter{}, false},
		{"codex", &CodexAdapter{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := newCLIInteractiveBackend(tc.adapter, nil, nil)
			if got := b.SupportsResume(); got != tc.want {
				t.Errorf("SupportsResume() = %v, want %v", got, tc.want)
			}
		})
	}
}

// =============================================================================
// Start tests
// =============================================================================

func TestCLIInteractiveBackend_Start_NilPtyManager_ReturnsError(t *testing.T) {
	t.Parallel()
	s := New(Config{Clock: clock.Real()})
	b := newCLIInteractiveBackend(&ClaudeAdapter{}, s, nil)
	proc := &processInfo{sessionID: "s", doneCh: make(chan struct{})}
	prep := &prepResult{opts: SpawnOptions{Model: "sonnet", WorkDir: "/tmp"}}
	if err := b.Start(context.Background(), proc, prep); err == nil {
		t.Error("Start() with nil PTYManager should return error")
	}
}

// TestCLIInteractiveBackend_Start_DeliverPrompt verifies that Start registers the
// command with the PTY manager, creates a session, and delivers prompt+\n via
// PTY stdin within ~1s (deliverPrompt has a 250ms readiness delay).
func TestCLIInteractiveBackend_Start_DeliverPrompt(t *testing.T) {
	t.Parallel()
	s := New(Config{Clock: clock.Real()})
	mgr := newMockPtyManager()
	b := newCLIInteractiveBackend(&ClaudeAdapter{}, s, mgr)

	proc := &processInfo{
		sessionID: "sess-deliver",
		doneCh:    make(chan struct{}),
	}
	prep := &prepResult{
		prompt: "run the tests",
		opts:   SpawnOptions{Model: "sonnet", WorkDir: "/tmp"},
	}

	// Cleanup: close session so goroutines (ferry + wait) exit even on test failure.
	t.Cleanup(func() {
		mgr.mu.Lock()
		sess := mgr.sessions["sess-deliver"]
		mgr.mu.Unlock()
		if sess != nil {
			_ = sess.Close()
		}
	})

	if err := b.Start(context.Background(), proc, prep); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// RegisterCommand must have been called synchronously in Start().
	mgr.mu.Lock()
	_, registered := mgr.registeredCmds["sess-deliver"]
	mgr.mu.Unlock()
	if !registered {
		t.Error("Start() did not call RegisterCommand")
	}

	// Poll for prompt delivery (deliverPrompt waits ~1.5s, writes body, settles
	// ~150ms, then submits with CR — total ~1.65s).
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(3 * time.Second)
	var written string
outer:
	for {
		select {
		case <-deadline:
			t.Fatal("prompt not delivered to PTY within 1s")
		case <-ticker.C:
			mgr.mu.Lock()
			sess := mgr.sessions["sess-deliver"]
			mgr.mu.Unlock()
			if sess == nil {
				continue
			}
			sess.mu.Lock()
			// Wait for both writes (body + submit CR) before declaring success.
			if strings.HasSuffix(string(sess.writtenBytes), "\r") {
				written = string(sess.writtenBytes)
				sess.mu.Unlock()
				break outer
			}
			sess.mu.Unlock()
		}
	}

	if written != "run the tests\r" {
		t.Errorf("delivered bytes = %q, want %q", written, "run the tests\r")
	}
}

// =============================================================================
// Kill routing tests
// =============================================================================

func TestCLIInteractiveBackend_Kill_SIGKILL_RoutesToSessionKill(t *testing.T) {
	t.Parallel()
	mgr := newMockPtyManager()
	sess := newMockSession()
	mgr.mu.Lock()
	mgr.sessions["sess-kill"] = sess
	mgr.mu.Unlock()

	b := newCLIInteractiveBackend(&ClaudeAdapter{}, nil, mgr)
	proc := &processInfo{sessionID: "sess-kill"}

	if err := b.Kill(context.Background(), proc, syscall.SIGKILL); err != nil {
		t.Fatalf("Kill(SIGKILL) error: %v", err)
	}
	sess.mu.Lock()
	kc, cc := sess.killCnt, sess.closeCnt
	sess.mu.Unlock()
	if kc != 1 {
		t.Errorf("sess.Kill() called %d times, want 1", kc)
	}
	if cc != 0 {
		t.Errorf("sess.Close() called %d times, want 0 for SIGKILL", cc)
	}
}

func TestCLIInteractiveBackend_Kill_SIGTERM_RoutesToSessionClose(t *testing.T) {
	t.Parallel()
	mgr := newMockPtyManager()
	sess := newMockSession()
	mgr.mu.Lock()
	mgr.sessions["sess-term"] = sess
	mgr.mu.Unlock()

	b := newCLIInteractiveBackend(&ClaudeAdapter{}, nil, mgr)
	proc := &processInfo{sessionID: "sess-term"}

	if err := b.Kill(context.Background(), proc, syscall.SIGTERM); err != nil {
		t.Fatalf("Kill(SIGTERM) error: %v", err)
	}
	sess.mu.Lock()
	kc, cc := sess.killCnt, sess.closeCnt
	sess.mu.Unlock()
	if cc != 1 {
		t.Errorf("sess.Close() called %d times, want 1", cc)
	}
	if kc != 0 {
		t.Errorf("sess.Kill() called %d times, want 0 for SIGTERM", kc)
	}
}

func TestCLIInteractiveBackend_Kill_NilSession_IsNoop(t *testing.T) {
	t.Parallel()
	mgr := newMockPtyManager() // empty — no session registered
	b := newCLIInteractiveBackend(&ClaudeAdapter{}, nil, mgr)
	proc := &processInfo{sessionID: "no-such-session"}
	if err := b.Kill(context.Background(), proc, syscall.SIGTERM); err != nil {
		t.Errorf("Kill with nil session should return nil, got: %v", err)
	}
}

// =============================================================================
// exitCodeFromSession tests
// =============================================================================

func TestExitCodeFromSession_WithExitCoder(t *testing.T) {
	t.Parallel()
	sess := newMockSession()
	sess.exitCodeVal = 42
	if got := exitCodeFromSession(sess); got != 42 {
		t.Errorf("exitCodeFromSession = %d, want 42", got)
	}
}

func TestExitCodeFromSession_ZeroExitCode(t *testing.T) {
	t.Parallel()
	sess := newMockSession() // exitCodeVal defaults to 0
	if got := exitCodeFromSession(sess); got != 0 {
		t.Errorf("exitCodeFromSession = %d, want 0", got)
	}
}

// noExitCoder wraps a ptySessionIface but does NOT implement ExitCode().
type noExitCoder struct{ ptySessionIface }

func TestExitCodeFromSession_WithoutExitCoder_ReturnsZero(t *testing.T) {
	t.Parallel()
	wrapped := noExitCoder{newMockSession()}
	if got := exitCodeFromSession(wrapped); got != 0 {
		t.Errorf("exitCodeFromSession (no ExitCode) = %d, want 0", got)
	}
}
