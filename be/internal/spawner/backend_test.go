package spawner

import (
	"context"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// TestCLIBackend_Name verifies the backend identifies as "cli".
func TestCLIBackend_Name(t *testing.T) {
	b := newCLIBackend(&ClaudeAdapter{}, nil)
	if got := b.Name(); got != "cli" {
		t.Errorf("Name() = %q, want %q", got, "cli")
	}
}

// TestCLIBackend_SupportsResume_MirrorsAdapter verifies SupportsResume reflects the adapter.
func TestCLIBackend_SupportsResume_MirrorsAdapter(t *testing.T) {
	tests := []struct {
		name    string
		adapter CLIAdapter
		want    bool
	}{
		{name: "claude", adapter: &ClaudeAdapter{}, want: true},
		{name: "opencode", adapter: &OpencodeAdapter{}, want: false},
		{name: "codex", adapter: &CodexAdapter{}, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := newCLIBackend(tc.adapter, nil)
			if got := b.SupportsResume(); got != tc.want {
				t.Errorf("SupportsResume() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestCLIBackend_SupportsTakeControl_MatchesSupportsResume verifies that take-control
// support is gated by SupportsResume — preserving the existing CLI behavior.
func TestCLIBackend_SupportsTakeControl_MatchesSupportsResume(t *testing.T) {
	tests := []struct {
		name    string
		adapter CLIAdapter
	}{
		{name: "claude", adapter: &ClaudeAdapter{}},
		{name: "opencode", adapter: &OpencodeAdapter{}},
		{name: "codex", adapter: &CodexAdapter{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := newCLIBackend(tc.adapter, nil)
			if b.SupportsTakeControl() != b.SupportsResume() {
				t.Errorf("SupportsTakeControl() = %v, SupportsResume() = %v; want equal",
					b.SupportsTakeControl(), b.SupportsResume())
			}
		})
	}
}

// TestCLIBackend_Kill_NilSafe verifies Kill on a proc with no cmd or no started
// process is a no-op (preserves existing safe-kill semantics at six call sites).
func TestCLIBackend_Kill_NilSafe(t *testing.T) {
	b := newCLIBackend(&ClaudeAdapter{}, nil)
	ctx := context.Background()

	// nil cmd
	proc := &processInfo{}
	if err := b.Kill(ctx, proc, syscall.SIGTERM); err != nil {
		t.Errorf("Kill(nil cmd) = %v, want nil", err)
	}

	// cmd with nil Process (never started)
	proc.cmd = exec.Command("sleep", "60")
	if proc.cmd.Process != nil {
		t.Fatalf("expected un-started cmd to have nil Process")
	}
	if err := b.Kill(ctx, proc, syscall.SIGKILL); err != nil {
		t.Errorf("Kill(unstarted cmd) = %v, want nil", err)
	}
}

// TestCLIBackend_Kill_SIGTERM verifies SIGTERM signals a running process and the
// backend reports no error. Uses a short-running OS sleep — not a CLI binary.
func TestCLIBackend_Kill_SIGTERM(t *testing.T) {
	b := newCLIBackend(&ClaudeAdapter{}, nil)
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
		// process exited (terminated by SIGTERM)
	case <-time.After(2 * time.Second):
		t.Fatalf("process did not exit within 2s after SIGTERM")
	}
}

// TestCLIBackend_Kill_SIGKILL verifies SIGKILL routes to Process.Kill() and
// terminates an unkillable-by-SIGTERM process within the wait window.
func TestCLIBackend_Kill_SIGKILL(t *testing.T) {
	b := newCLIBackend(&ClaudeAdapter{}, nil)
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
