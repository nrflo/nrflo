package refinery

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
	"be/internal/spawner/apirun/provider"
	"be/internal/spawner/apirun/provider/mock"
	"be/internal/ws"
)

// seedAutonomousSession inserts the minimal projects + agent_sessions rows an
// autonomous fold needs: a normal (workflow_agent kind) session whose
// agent_messages the sidecar reads. No workflow_instances row is needed —
// the (workflow_instance_id, node_id) slot passed to StartSession is an
// independent identity from agent_sessions.workflow_instance_id, and the
// slot table (refinery_autonomous_digests) only FKs to projects.
//
// context_left is seeded at 10 (well under the default fold-start threshold
// of 40) so every existing fold-happy-path test stays gate-open without
// having to know about the gate; tests exercising the gate itself use
// setContextLeft to override it.
func seedAutonomousSession(t *testing.T, pool *db.Pool, sessionID, projectID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(
		`INSERT OR IGNORE INTO projects (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		projectID, projectID, now, now,
	); err != nil {
		t.Fatalf("seed project %s: %v", projectID, err)
	}
	if _, err := pool.Exec(
		`INSERT INTO agent_sessions (id, project_id, ticket_id, phase, node_id, agent_type, status, kind, context_left, created_at, updated_at)
		 VALUES (?, ?, 'TICKET-1', 'implementor', 'implementor', 'implementor', 'running', 'workflow_agent', 10, ?, ?)`,
		sessionID, projectID, now, now,
	); err != nil {
		t.Fatalf("seed autonomous session %s: %v", sessionID, err)
	}
}

// setContextLeft updates agent_sessions.context_left for sessionID, or sets
// it to NULL when left is nil — used by gate tests to drive foldGateOpen's
// threshold comparison.
func setContextLeft(t *testing.T, pool *db.Pool, sessionID string, left *int) {
	t.Helper()
	if _, err := pool.Exec(`UPDATE agent_sessions SET context_left = ? WHERE id = ?`, left, sessionID); err != nil {
		t.Fatalf("set context_left for %s: %v", sessionID, err)
	}
}

// seedMessages appends messages to sessionID via the real repo (ordered by
// seq), mirroring how the spawner writes agent_messages.
func seedMessages(t *testing.T, pool *db.Pool, clk clock.Clock, sessionID string, contents ...string) {
	t.Helper()
	entries := make([]repo.MessageEntry, len(contents))
	for i, c := range contents {
		entries[i] = repo.MessageEntry{Content: c, Category: "text"}
	}
	if err := repo.NewAgentMessageRepo(pool, clk).InsertBatch(sessionID, entries); err != nil {
		t.Fatalf("seed messages for %s: %v", sessionID, err)
	}
}

func getSlot(t *testing.T, mgr *Manager, wfiID, nodeID string) *slotResult {
	t.Helper()
	d, err := mgr.digestRepo.GetSlot(wfiID, nodeID)
	if err != nil {
		t.Fatalf("GetSlot(%s,%s): %v", wfiID, nodeID, err)
	}
	if d == nil {
		return nil
	}
	return &slotResult{Content: d.Content, Version: d.Version, FoldCount: d.FoldCount}
}

type slotResult struct {
	Content   string
	Version   int
	FoldCount int
}

// capturingProvider records the last Request handed to Run, so tests can
// assert the fold's user-text payload without a full mock.Script.
type capturingProvider struct {
	mu       sync.Mutex
	lastReq  provider.Request
	response provider.FinalResponse
}

func newCapturingProvider(text string) *capturingProvider {
	return &capturingProvider{response: provider.FinalResponse{
		StopReason: "end_turn",
		Content:    []provider.ContentBlock{{Type: "text", Text: text}},
		Usage:      provider.Usage{InputTokens: 11, OutputTokens: 22, CacheReadTokens: 3, CacheCreationTokens: 4},
	}}
}

func (p *capturingProvider) Name() string          { return "mock-capture" }
func (p *capturingProvider) MaxContext(string) int { return 200000 }
func (p *capturingProvider) lastUserText() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.lastReq.Messages) == 0 || len(p.lastReq.Messages[0].Content) == 0 {
		return ""
	}
	return p.lastReq.Messages[0].Content[0].Text
}

func (p *capturingProvider) Run(ctx context.Context, req provider.Request, sink provider.EventSink) (*provider.FinalResponse, error) {
	p.mu.Lock()
	p.lastReq = req
	resp := p.response
	p.mu.Unlock()
	return &resp, nil
}

var _ provider.Provider = (*capturingProvider)(nil)

func TestStartSession_FindingsUpdatedTriggersImmediateFold(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-auto-1", "proj-auto-1"
	wfiID, nodeID := "wfi-1", "node-1"
	seedAutonomousSession(t, pool, sessionID, projectID)
	seedMessages(t, pool, clk, sessionID, "hello", "world")

	mgr := NewManager(pool, clk)
	stubBuildProvider(t, mock.New(mockScript("slot digest v1")))
	mgr.StartSession(sessionID, projectID, wfiID, nodeID)
	t.Cleanup(func() { mgr.StopSession(sessionID) })

	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID, SessionID: sessionID})

	waitForCondition(t, 2*time.Second, func() bool {
		s := getSlot(t, mgr, wfiID, nodeID)
		return s != nil && s.FoldCount == 1
	})
	s := getSlot(t, mgr, wfiID, nodeID)
	if s.Content != "slot digest v1" {
		t.Errorf("Content = %q, want %q", s.Content, "slot digest v1")
	}
	if s.Version != 1 {
		t.Errorf("Version = %d, want 1", s.Version)
	}
}

func TestFoldAutonomous_DeltaContainsOnlyNewMessagesSinceLastFold(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sessionID, projectID := "sess-auto-delta", "proj-auto-delta"
	wfiID, nodeID := "wfi-delta", "node-delta"
	seedAutonomousSession(t, pool, sessionID, projectID)
	seedMessages(t, pool, clk, sessionID, "msg-one", "msg-two")

	mgr := NewManager(pool, clk)
	prov := newCapturingProvider("digest after first fold")
	stubBuildProvider(t, prov)
	mgr.StartSession(sessionID, projectID, wfiID, nodeID)
	t.Cleanup(func() { mgr.StopSession(sessionID) })

	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID, SessionID: sessionID})
	waitForCondition(t, 2*time.Second, func() bool {
		s := getSlot(t, mgr, wfiID, nodeID)
		return s != nil && s.FoldCount == 1
	})
	firstText := prov.lastUserText()
	if !strings.Contains(firstText, "msg-one") || !strings.Contains(firstText, "msg-two") {
		t.Errorf("first fold user text = %q, want it to contain both seed messages", firstText)
	}

	// Append exactly one new message and fire again: the second fold's user
	// text must contain ONLY the new message, not msg-one/msg-two again.
	seedMessages(t, pool, clk, sessionID, "msg-three")
	prov.response.Content = []provider.ContentBlock{{Type: "text", Text: "digest after second fold"}}
	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID, SessionID: sessionID})

	waitForCondition(t, 2*time.Second, func() bool {
		s := getSlot(t, mgr, wfiID, nodeID)
		return s != nil && s.FoldCount == 2
	})
	secondText := prov.lastUserText()
	if !strings.Contains(secondText, "msg-three") {
		t.Errorf("second fold user text = %q, want it to contain msg-three", secondText)
	}
	if strings.Contains(secondText, "msg-one") || strings.Contains(secondText, "msg-two") {
		t.Errorf("second fold user text = %q, want it to NOT re-include msg-one/msg-two (delta-only)", secondText)
	}

	s := getSlot(t, mgr, wfiID, nodeID)
	if s.Version != 2 {
		t.Errorf("Version after second fold = %d, want 2", s.Version)
	}
}

func TestRelaunch_SecondSessionInSameSlotSeesPriorDigestAsPrev(t *testing.T) {
	pool := newTestPool(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sid1, sid2, projectID := "sess-relaunch-1", "sess-relaunch-2", "proj-relaunch"
	wfiID, nodeID := "wfi-relaunch", "node-relaunch"
	seedAutonomousSession(t, pool, sid1, projectID)
	seedAutonomousSession(t, pool, sid2, projectID)
	seedMessages(t, pool, clk, sid1, "session one work")

	mgr := NewManager(pool, clk)
	prov := newCapturingProvider("digest v1 from session one")
	stubBuildProvider(t, prov)

	mgr.StartSession(sid1, projectID, wfiID, nodeID)
	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID, SessionID: sid1})
	waitForCondition(t, 2*time.Second, func() bool {
		s := getSlot(t, mgr, wfiID, nodeID)
		return s != nil && s.FoldCount == 1
	})

	mgr.StopSession(sid1)
	if s := getSlot(t, mgr, wfiID, nodeID); s == nil || s.Content != "digest v1 from session one" {
		t.Fatalf("slot after StopSession(sid1) = %+v, want the v1 digest to persist (no nil gap)", s)
	}

	// Relaunch: sid2 starts in the SAME slot and appends new messages.
	mgr.StartSession(sid2, projectID, wfiID, nodeID)
	t.Cleanup(func() { mgr.StopSession(sid2) })
	seedMessages(t, pool, clk, sid2, "session two work")
	prov.response.Content = []provider.ContentBlock{{Type: "text", Text: "digest v2 from session two"}}
	mgr.OnEvent(&ws.Event{Type: ws.EventFindingsUpdated, ProjectID: projectID, SessionID: sid2})

	waitForCondition(t, 2*time.Second, func() bool {
		s := getSlot(t, mgr, wfiID, nodeID)
		return s != nil && s.FoldCount == 2
	})
	// The fold's prev-content section must have fed the prior (v1) digest.
	if !strings.Contains(prov.lastUserText(), "digest v1 from session one") {
		t.Errorf("relaunch fold user text = %q, want it to include the prior slot digest as prev", prov.lastUserText())
	}
	if !strings.Contains(prov.lastUserText(), "session two work") {
		t.Errorf("relaunch fold user text = %q, want it to include sid2's new message", prov.lastUserText())
	}
	s := getSlot(t, mgr, wfiID, nodeID)
	if s.Content != "digest v2 from session two" {
		t.Errorf("slot content after relaunch fold = %q, want %q", s.Content, "digest v2 from session two")
	}
	if s.Version != 2 {
		t.Errorf("slot version after relaunch fold = %d, want 2 (continued from v1, not reset)", s.Version)
	}
}
