package refinery

import (
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

// seedMessagesWithCategory is seedMessages' category-aware sibling: each
// content gets its own explicit category instead of the "text" default, so
// tests can exercise foldAutonomous's per-category skip logic.
func seedMessagesWithCategory(t *testing.T, pool *db.Pool, clk clock.Clock, sessionID string, entries ...repo.MessageEntry) {
	t.Helper()
	if err := repo.NewAgentMessageRepo(pool, clk).InsertBatch(sessionID, entries); err != nil {
		t.Fatalf("seed messages with category for %s: %v", sessionID, err)
	}
}

// TestFoldAutonomous_DropsSystemNoticeKeepsTaskNotification verifies the
// fold-delta loop excludes model.MsgCategorySystemNotice rows from the
// rendered user text while keeping model.MsgCategoryTaskNotification rows.
func TestFoldAutonomous_DropsSystemNoticeKeepsTaskNotification(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-cat-1", "proj-cat-1"
	wfiID, nodeID := "wfi-cat-1", "node-cat-1"
	seedAutonomousSession(t, pool, sessionID, projectID)
	seedMessagesWithCategory(t, pool, clk, sessionID,
		repo.MessageEntry{Content: "kept assistant text", Category: "text"},
		repo.MessageEntry{Content: "dropped notice", Category: model.MsgCategorySystemNotice},
		repo.MessageEntry{Content: "kept task notification", Category: model.MsgCategoryTaskNotification},
	)

	mgr := NewManager(pool, clk)
	prov := newCapturingProvider("digest v1")
	stubBuildProvider(t, prov)
	mgr.StartSession(sessionID, projectID, wfiID, nodeID)
	t.Cleanup(func() { mgr.StopSession(sessionID) })

	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID, SessionID: sessionID})
	waitForCondition(t, 2*time.Second, func() bool {
		s := getSlot(t, mgr, wfiID, nodeID)
		return s != nil && s.FoldCount == 1
	})

	text := prov.lastUserText()
	if !strings.Contains(text, "kept assistant text") {
		t.Errorf("fold user text = %q, want it to contain the text-category row", text)
	}
	if !strings.Contains(text, "kept task notification") {
		t.Errorf("fold user text = %q, want it to contain the task_notification row", text)
	}
	if strings.Contains(text, "dropped notice") {
		t.Errorf("fold user text = %q, want it to NOT contain the system_notice row", text)
	}
}

// TestFoldAutonomous_AllRowsSkipped_NoOps verifies that when every row in the
// delta is a skipped category (system_notice), foldAutonomous returns before
// calling runFoldCore: no provider call, no slot write, nextFoldSeq unadvanced
// so the next trigger re-reads the same delta.
func TestFoldAutonomous_AllRowsSkipped_NoOps(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-cat-2", "proj-cat-2"
	wfiID, nodeID := "wfi-cat-2", "node-cat-2"
	seedAutonomousSession(t, pool, sessionID, projectID)
	seedMessagesWithCategory(t, pool, clk, sessionID,
		repo.MessageEntry{Content: "notice one", Category: model.MsgCategorySystemNotice},
		repo.MessageEntry{Content: "notice two", Category: model.MsgCategorySystemNotice},
	)

	mgr := NewManager(pool, clk)
	prov := newCapturingProvider("should never be used")
	stubBuildProvider(t, prov)
	mgr.StartSession(sessionID, projectID, wfiID, nodeID)
	t.Cleanup(func() { mgr.StopSession(sessionID) })

	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID, SessionID: sessionID})

	// Give the debounced/immediate trigger a moment to run, then assert no
	// slot was ever written — an all-skipped delta must no-op, not fold an
	// empty payload.
	settle(200 * time.Millisecond)
	if s := getSlot(t, mgr, wfiID, nodeID); s != nil {
		t.Errorf("slot = %+v, want nil (all-skipped delta must not fold)", s)
	}
	if prov.lastUserText() != "" {
		t.Errorf("provider was called with %q, want no call at all", prov.lastUserText())
	}
}
