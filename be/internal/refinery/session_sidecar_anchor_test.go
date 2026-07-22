package refinery

import (
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
	"be/internal/ws"
)

// setSessionPrompt sets agent_sessions.prompt directly, mirroring how
// createAgentSessionRow persists proc.prompt before StartSession fires.
func setSessionPrompt(t *testing.T, pool *db.Pool, sessionID, prompt string) {
	t.Helper()
	if _, err := pool.Exec(`UPDATE agent_sessions SET prompt = ? WHERE id = ?`, prompt, sessionID); err != nil {
		t.Fatalf("set prompt for %s: %v", sessionID, err)
	}
}

// TestFoldAutonomous_TaskAnchorAndCategoryLabelsReachFoldText is the
// end-to-end coverage for the task-anchor + categorized-delta rendering:
// seed agent_sessions.prompt as the anchor and mixed-category messages, then
// assert the fold's user text carries the anchor verbatim plus "[category]
// content" labeled lines (never a bare unlabeled content line).
func TestFoldAutonomous_TaskAnchorAndCategoryLabelsReachFoldText(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-auto-anchor", "proj-auto-anchor"
	wfiID, nodeID := "wfi-anchor", "node-anchor"
	seedAutonomousSession(t, pool, sessionID, projectID)
	setSessionPrompt(t, pool, sessionID, "Implement the widget feature end to end.")

	entries := []repo.MessageEntry{
		{Content: "please add a widget", Category: "user_input"},
		{Content: "ran ls -la", Category: "tool"},
	}
	if err := repo.NewAgentMessageRepo(pool, clk).InsertBatch(sessionID, entries); err != nil {
		t.Fatalf("seed categorized messages: %v", err)
	}

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
	if !strings.Contains(text, "## Task\n\nImplement the widget feature end to end.") {
		t.Errorf("fold user text = %q, want a ## Task section with the anchor verbatim", text)
	}
	if !strings.Contains(text, "[user_input] please add a widget") {
		t.Errorf("fold user text = %q, want the [user_input] labeled line", text)
	}
	if !strings.Contains(text, "[tool] ran ls -la") {
		t.Errorf("fold user text = %q, want the [tool] labeled line", text)
	}
	if strings.Contains(text, "\nplease add a widget\n") || strings.Contains(text, "\nran ls -la\n") {
		t.Errorf("fold user text = %q, want no bare unlabeled content line", text)
	}
}

// TestStartSession_NoPromptSeeded_EmptyAnchorOmitsTaskSection covers the
// zero-value case: a session with no agent_sessions.prompt set must fold
// with no ## Task section at all (StartSession's GetPrompt falls back to "").
func TestStartSession_NoPromptSeeded_EmptyAnchorOmitsTaskSection(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-auto-noanchor", "proj-auto-noanchor"
	wfiID, nodeID := "wfi-noanchor", "node-noanchor"
	seedAutonomousSession(t, pool, sessionID, projectID)
	seedMessages(t, pool, clk, sessionID, "hello")

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
	if strings.Contains(text, "## Task") {
		t.Errorf("fold user text with no prompt seeded = %q, want no ## Task section", text)
	}
	if !strings.Contains(text, "[text] hello") {
		t.Errorf("fold user text = %q, want the [text] labeled line", text)
	}
}
