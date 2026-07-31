package refinery

import (
	"context"
	"testing"
	"time"

	"be/internal/ws"
)

// TestManager_Flush_FoldsBufferedEventsSynchronously proves Flush's fold is
// synchronous: it asserts foldCount right after Flush returns, with NO clock
// advance and NO waitForCondition polling loop. A waitForCondition here would
// hide the very thing being tested.
func TestManager_Flush_FoldsBufferedEventsSynchronously(t *testing.T) {
	mgr, _ := newTestManager(t)
	sessionID, projectID := "sess-flush-1", "proj-flush-1"
	seedConsoleChatSession(t, mgr.pool, sessionID, projectID)
	mgr.Start(sessionID, projectID)
	t.Cleanup(func() { mgr.Stop(sessionID) })

	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID})
	settle(50 * time.Millisecond) // let the sidecar goroutine buffer the event (below the 30s debounce floor)

	mgr.Flush(context.Background(), sessionID)

	if got := foldCount(t, mgr, sessionID); got != 1 {
		t.Errorf("fold_count immediately after Flush = %d, want 1", got)
	}
}

// TestManager_Flush_EmptyBufferDoesNotFold verifies Flush on a live session
// with nothing buffered is a no-op fold (no wasted provider call, no row).
// foldNow itself no longer gates on an empty buffer (it always calls
// s.fold) — the no-op here comes from foldConsole's own emptiness check
// (both the agent_messages delta and the buffered events are empty).
func TestManager_Flush_EmptyBufferDoesNotFold(t *testing.T) {
	mgr, _ := newTestManager(t)
	sessionID, projectID := "sess-flush-2", "proj-flush-2"
	seedConsoleChatSession(t, mgr.pool, sessionID, projectID)
	mgr.Start(sessionID, projectID)
	t.Cleanup(func() { mgr.Stop(sessionID) })

	mgr.Flush(context.Background(), sessionID)

	if got := foldCount(t, mgr, sessionID); got != 0 {
		t.Errorf("fold_count after Flush with empty buffer = %d, want 0", got)
	}
	if rows := queryRefineryRuns(t, mgr.pool); len(rows) != 0 {
		t.Errorf("refinery_runs row count after empty-buffer Flush = %d, want 0", len(rows))
	}
}

// TestManager_Flush_UnknownSessionIsSafeNoop mirrors
// TestManager_Stop_IsIdempotentForUnknownSession: Flush on a session that was
// never Started (or already stopped) must never panic.
func TestManager_Flush_UnknownSessionIsSafeNoop(t *testing.T) {
	mgr, _ := newTestManager(t)
	mgr.Flush(context.Background(), "no-such-session")
}

// TestManager_Flush_ExpiredCtxSkipsFoldAndWritesNoRunRow verifies the
// expired-ctx guard: a Flush whose ctx is already done when dequeued must
// skip the fold entirely rather than run a doomed provider.Run that writes a
// bogus refinery_runs status=failed row.
func TestManager_Flush_ExpiredCtxSkipsFoldAndWritesNoRunRow(t *testing.T) {
	mgr, _ := newTestManager(t)
	sessionID, projectID := "sess-flush-4", "proj-flush-4"
	seedConsoleChatSession(t, mgr.pool, sessionID, projectID)
	mgr.Start(sessionID, projectID)
	t.Cleanup(func() { mgr.Stop(sessionID) })

	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID})
	settle(50 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	mgr.Flush(ctx, sessionID)

	if got := foldCount(t, mgr, sessionID); got != 0 {
		t.Errorf("fold_count after Flush with an already-cancelled ctx = %d, want 0", got)
	}
	if rows := queryRefineryRuns(t, mgr.pool); len(rows) != 0 {
		t.Errorf("refinery_runs row count after expired-ctx Flush = %d, want 0 (no bogus failed row)", len(rows))
	}
}

// TestManager_Stop_PerformsFinalFold verifies Stop's bounded final fold runs
// synchronously before Stop returns — no clock advance needed.
func TestManager_Stop_PerformsFinalFold(t *testing.T) {
	mgr, _ := newTestManager(t)
	sessionID, projectID := "sess-flush-5", "proj-flush-5"
	seedConsoleChatSession(t, mgr.pool, sessionID, projectID)
	mgr.Start(sessionID, projectID)

	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID})
	settle(50 * time.Millisecond)

	mgr.Stop(sessionID)

	if got := foldCount(t, mgr, sessionID); got != 1 {
		t.Errorf("fold_count immediately after Stop = %d, want 1", got)
	}
}

// TestManager_Stop_EmptyBufferDoesNotFold prevents a per-close wasted
// provider call on every chat session that never saw a relevant event.
// As with Flush, the no-op comes from foldConsole's own emptiness check,
// not from foldNow gating on the buffer.
func TestManager_Stop_EmptyBufferDoesNotFold(t *testing.T) {
	mgr, _ := newTestManager(t)
	sessionID, projectID := "sess-flush-6", "proj-flush-6"
	seedConsoleChatSession(t, mgr.pool, sessionID, projectID)
	mgr.Start(sessionID, projectID)

	mgr.Stop(sessionID)

	if got := foldCount(t, mgr, sessionID); got != 0 {
		t.Errorf("fold_count after Stop with empty buffer = %d, want 0", got)
	}
	if rows := queryRefineryRuns(t, mgr.pool); len(rows) != 0 {
		t.Errorf("refinery_runs row count after empty-buffer Stop = %d, want 0", len(rows))
	}
}

// TestManager_Flush_AfterStopIsPromptNoop is the deadlock regression guard:
// once Stop has removed the sidecar from the map, a later Flush must return
// promptly rather than block forever trying to reach a dead loop goroutine.
func TestManager_Flush_AfterStopIsPromptNoop(t *testing.T) {
	mgr, _ := newTestManager(t)
	sessionID, projectID := "sess-flush-7", "proj-flush-7"
	seedConsoleChatSession(t, mgr.pool, sessionID, projectID)
	mgr.Start(sessionID, projectID)
	mgr.Stop(sessionID)

	done := make(chan struct{})
	go func() {
		mgr.Flush(context.Background(), sessionID)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Flush after Stop did not return promptly (possible deadlock)")
	}
}
