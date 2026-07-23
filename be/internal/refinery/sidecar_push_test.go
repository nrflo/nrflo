package refinery

import (
	"context"
	"testing"
	"time"

	"be/internal/clock"
)

// TestSidecarPush_FullTriggerChannel_ReturnsPromptlyAndKeepsAllBufferedLines
// pushes cap(triggerCh)+8 lines with no loop goroutine draining the channel:
// push must never block (its select/default drops the trigger send once the
// channel is full), yet every pushed line must still land in s.buffered —
// only fold timing is affected by a dropped trigger, never data.
func TestSidecarPush_FullTriggerChannel_ReturnsPromptlyAndKeepsAllBufferedLines(t *testing.T) {
	sc := newSidecar("sess-push-1", "proj-push-1", clock.Real(), func(context.Context, string, string, []string) {})
	// No sc.run(): nothing drains triggerCh, so its buffer fills after
	// cap(triggerCh) pushes and every push beyond that must hit the drop path.
	total := cap(sc.triggerCh) + 8

	done := make(chan struct{})
	go func() {
		for i := 0; i < total; i++ {
			sc.push("line", false)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("push did not return promptly with a full, undrained trigger channel")
	}

	sc.mu.Lock()
	got := len(sc.buffered)
	sc.mu.Unlock()
	if got != total {
		t.Errorf("len(s.buffered) = %d, want %d (every pushed line kept even when its trigger was dropped)", got, total)
	}
}

// TestSidecarPush_AfterStop_ReturnsPromptly verifies push on a stopped
// sidecar never blocks: stop() has already drained/closed the loop, so a
// push racing in after stop must still return without hanging.
func TestSidecarPush_AfterStop_ReturnsPromptly(t *testing.T) {
	sc := newSidecar("sess-push-2", "proj-push-2", clock.Real(), func(context.Context, string, string, []string) {})
	sc.run()
	sc.stop()

	done := make(chan struct{})
	go func() {
		sc.push("line after stop", false)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("push after stop() did not return promptly")
	}

	sc.mu.Lock()
	got := len(sc.buffered)
	sc.mu.Unlock()
	if got != 1 {
		t.Errorf("len(s.buffered) after push post-stop = %d, want 1 (the line is still recorded)", got)
	}
}
