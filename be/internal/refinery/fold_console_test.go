package refinery

import (
	"context"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/repo"
	"be/internal/ws"
)

// TestFoldConsole_TicketCase_IncludesUserPromptAndDelegationFindings is the
// ticket's reproduction case: a console_chat session with one user_input
// turn and one tool row carrying a delegation-findings body must fold to a
// user text that contains BOTH the user's task subject and the findings
// content — not merely the "_delegate_host"/"_delegate_findings" WS
// event-metadata keys the pre-fix code relied on exclusively.
func TestFoldConsole_TicketCase_IncludesUserPromptAndDelegationFindings(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-console-ticket", "proj-console-ticket"
	seedConsoleChatSession(t, pool, sessionID, projectID)

	const userSubject = "add a cwd segment to the nrflo console status bar"
	const findingsBody = "_delegate_findings: implemented 5 files on branch nrdelegate/status-bar-cwd"
	seedMessagesWithCategory(t, pool, clk, sessionID,
		repo.MessageEntry{Content: userSubject, Category: "user_input"},
		repo.MessageEntry{Content: findingsBody, Category: "tool"},
	)

	mgr := NewManager(pool, clk)
	prov := newCapturingProvider("digest v1")
	stubBuildProvider(t, prov)
	mgr.Start(sessionID, projectID)
	t.Cleanup(func() { mgr.Stop(sessionID) })

	// findings.updated arrives as an _delegate_host/_delegate_findings WS
	// event carrying only metadata keys — Touch is the trigger that pulls in
	// the actual conversation content via agent_messages.
	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID, Data: map[string]interface{}{
		"agent_type": "_delegate_host", "key": "_delegate_findings", "action": "updated",
	}})
	mgr.Touch(sessionID)
	settle(50 * time.Millisecond)

	mgr.Flush(context.Background(), sessionID)

	text := prov.lastUserText()
	if !strings.Contains(text, userSubject) {
		t.Errorf("fold user text = %q, want it to contain the user prompt subject %q", text, userSubject)
	}
	if !strings.Contains(text, findingsBody) {
		t.Errorf("fold user text = %q, want it to contain the delegation findings body %q", text, findingsBody)
	}
	// The pre-fix bug: the fold input was built purely from the WS event's
	// compact metadata line (type/agent_type/key/action), never the actual
	// findings content — so it never mentioned the task subject at all.
	if !strings.Contains(text, "## Conversation") {
		t.Errorf("fold user text = %q, want a ## Conversation section carrying the actual turns", text)
	}
}

// TestFoldConsole_FindingsUpdatedOnlyTrigger_StillFoldsConversation is the
// regression guard for the observed arc: a findings.updated-only trigger
// (project-scoped WS event, no explicit Touch call) on a session that
// already has conversation rows must still fold real conversation content,
// not just the bare event line.
func TestFoldConsole_FindingsUpdatedOnlyTrigger_StillFoldsConversation(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-console-findings-only", "proj-console-findings-only"
	seedConsoleChatSession(t, pool, sessionID, projectID)

	const userSubject = "please refactor the status bar segment ordering"
	seedMessagesWithCategory(t, pool, clk, sessionID,
		repo.MessageEntry{Content: userSubject, Category: "user_input"},
	)

	mgr := NewManager(pool, clk)
	prov := newCapturingProvider("digest v1")
	stubBuildProvider(t, prov)
	mgr.Start(sessionID, projectID)
	t.Cleanup(func() { mgr.Stop(sessionID) })

	// Project-scoped findings.updated only, no Touch.
	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID})
	settle(50 * time.Millisecond)

	mgr.Flush(context.Background(), sessionID)

	text := prov.lastUserText()
	if !strings.Contains(text, userSubject) {
		t.Errorf("fold user text = %q, want it to contain the conversation content %q even though only findings.updated fired", text, userSubject)
	}
}

// TestFoldConsole_EmptyWorkingSet_NoProviderCallNoRunRow preserves the
// pre-existing empty-buffer invariant (TestManager_Flush_EmptyBufferDoesNotFold)
// after the foldNow change: no messages ever seeded and no buffered WS
// events must mean no provider call at all and no refinery_runs row.
func TestFoldConsole_EmptyWorkingSet_NoProviderCallNoRunRow(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-console-empty", "proj-console-empty"
	seedConsoleChatSession(t, pool, sessionID, projectID)

	mgr := NewManager(pool, clk)
	prov := newCapturingProvider("should never be used")
	stubBuildProvider(t, prov)
	mgr.Start(sessionID, projectID)
	t.Cleanup(func() { mgr.Stop(sessionID) })

	mgr.Flush(context.Background(), sessionID)

	if prov.lastUserText() != "" {
		t.Errorf("provider was called with %q, want no call at all on an empty working set", prov.lastUserText())
	}
	d, err := mgr.digestRepo.Get(sessionID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if d != nil {
		t.Errorf("digest after empty-working-set flush = %+v, want nil", d)
	}
	if rows := queryRefineryRuns(t, mgr.pool); len(rows) != 0 {
		t.Errorf("refinery_runs row count after empty-working-set flush = %d, want 0", len(rows))
	}
}

// TestFoldConsole_IncrementalSeq_SecondFoldContainsOnlyNewRow mirrors
// session_sidecar_test.go's autonomous delta test for the console path: a
// second fold must only see the row seeded after the first fold, not
// re-include the row already folded.
func TestFoldConsole_IncrementalSeq_SecondFoldContainsOnlyNewRow(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-console-incremental", "proj-console-incremental"
	seedConsoleChatSession(t, pool, sessionID, projectID)
	seedMessagesWithCategory(t, pool, clk, sessionID,
		repo.MessageEntry{Content: "first turn content", Category: "user_input"},
	)

	mgr := NewManager(pool, clk)
	prov := newCapturingProvider("digest after first fold")
	stubBuildProvider(t, prov)
	mgr.Start(sessionID, projectID)
	t.Cleanup(func() { mgr.Stop(sessionID) })

	mgr.Touch(sessionID)
	settle(50 * time.Millisecond)
	mgr.Flush(context.Background(), sessionID)

	firstText := prov.lastUserText()
	if !strings.Contains(firstText, "first turn content") {
		t.Errorf("first fold user text = %q, want it to contain %q", firstText, "first turn content")
	}

	seedMessagesWithCategory(t, pool, clk, sessionID,
		repo.MessageEntry{Content: "second turn content", Category: "user_input"},
	)
	prov.response.Content = prov.response.Content[:0]
	prov.response.Content = append(prov.response.Content, mockScript("digest after second fold").Final.Content...)

	mgr.Touch(sessionID)
	settle(50 * time.Millisecond)
	mgr.Flush(context.Background(), sessionID)

	secondText := prov.lastUserText()
	if !strings.Contains(secondText, "second turn content") {
		t.Errorf("second fold user text = %q, want it to contain %q", secondText, "second turn content")
	}
	if strings.Contains(secondText, "first turn content") {
		t.Errorf("second fold user text = %q, want it to NOT re-include %q (delta-only)", secondText, "first turn content")
	}
}

// TestFoldConsole_CompactionRunsBeforeTailKeepEviction is the compaction
// regression: a short user_input row followed by a ~20KB tool row (e.g. a
// full delegation-findings dump) must leave the short row present in the
// fold input. Without foldfmt.CapRows head-capping each row before
// JoinTail's tail-keep pass, the oversized newest row alone would exceed
// maxConsoleFoldDeltaChars and JoinTail's zero-kept branch would discard the
// short row entirely, keeping only a truncated slice of the huge one.
func TestFoldConsole_CompactionRunsBeforeTailKeepEviction(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-console-compaction", "proj-console-compaction"
	seedConsoleChatSession(t, pool, sessionID, projectID)

	const shortSubject = "add a cwd segment to the status bar"
	hugeToolBody := strings.Repeat("x", 20000)
	seedMessagesWithCategory(t, pool, clk, sessionID,
		repo.MessageEntry{Content: shortSubject, Category: "user_input"},
		repo.MessageEntry{Content: hugeToolBody, Category: "tool"},
	)

	mgr := NewManager(pool, clk)
	prov := newCapturingProvider("digest v1")
	stubBuildProvider(t, prov)
	mgr.Start(sessionID, projectID)
	t.Cleanup(func() { mgr.Stop(sessionID) })

	mgr.Touch(sessionID)
	settle(50 * time.Millisecond)
	mgr.Flush(context.Background(), sessionID)

	text := prov.lastUserText()
	if !strings.Contains(text, shortSubject) {
		t.Errorf("fold user text does not contain the short user_input row %q, want CapRows to have head-capped the huge tool row before JoinTail's eviction pass; text=%q", shortSubject, text)
	}
}
