package console

import (
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/ws"
)

// waitForEventType blocks until an event of eventType arrives on ch.
func waitForEventType(t *testing.T, ch <-chan []byte, eventType string, timeout time.Duration) ws.Event {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case raw := <-ch:
			var ev ws.Event
			if err := json.Unmarshal(raw, &ev); err != nil {
				t.Fatalf("unmarshal WS event: %v", err)
			}
			if ev.Type == eventType {
				return ev
			}
		case <-deadline:
			t.Fatalf("timed out waiting for event type %q", eventType)
		}
	}
}

// setProactiveRestartConsolePct sets the console rotation threshold as a
// percentage of the live context window, so a session's simulated
// context-left percentage deterministically lands over or under it.
func setProactiveRestartConsolePct(t *testing.T, pool interface {
	SetConfig(string, string) error
}, pct string) {
	t.Helper()
	if err := pool.SetConfig("proactive_restart_console_pct", pct); err != nil {
		t.Fatalf("SetConfig proactive_restart_console_pct: %v", err)
	}
}

// TestChatService_MaybeRotate_NoDigest_NoOp verifies a session with no
// refinery digest never rotates, regardless of context usage — there is
// nothing to carry the conversation forward with.
func TestChatService_MaybeRotate_NoDigest_NoOp(t *testing.T) {
	t.Parallel()
	svc, pool, _, factory := newChatTestService(t)
	setProactiveRestartConsolePct(t, pool, "50")

	sid, err := svc.Create("claude", "opus-4-6", "high", chatTestProjectID, "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	sess, ok := svc.get(sid)
	if !ok {
		t.Fatal("session not found after Create")
	}
	sess.noteContextLeft(0) // 0% left => currentTokens = maxContext, way over threshold

	if svc.maybeRotate(sess) {
		t.Error("maybeRotate() = true with no refinery digest, want false")
	}
	if len(factory.engines) != 1 {
		t.Errorf("engines constructed = %d, want 1 (no rotation engine built)", len(factory.engines))
	}
}

// TestChatService_MaybeRotate_NoContextSignalYet_NoOp verifies a session
// that never received an EventTokenUsage (or reset) cannot rotate: there is
// no known token count to compare against the threshold.
func TestChatService_MaybeRotate_NoContextSignalYet_NoOp(t *testing.T) {
	t.Parallel()
	svc, pool, _, _ := newChatTestService(t)
	setProactiveRestartConsolePct(t, pool, "50")

	sid, err := svc.Create("claude", "opus-4-6", "high", chatTestProjectID, "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	digestRepo := repo.NewRefineryDigestRepo(pool, clock.Real())
	if _, err := digestRepo.Upsert(sid, chatTestProjectID, "some working-set digest"); err != nil {
		t.Fatalf("Upsert digest: %v", err)
	}
	sess, _ := svc.get(sid)

	if svc.maybeRotate(sess) {
		t.Error("maybeRotate() = true with no context signal yet, want false")
	}
}

// TestChatService_MaybeRotate_UnderThreshold_NoOp verifies a session with a
// digest and known context usage but still under threshold does not rotate.
func TestChatService_MaybeRotate_UnderThreshold_NoOp(t *testing.T) {
	t.Parallel()
	svc, pool, _, _ := newChatTestService(t)
	setProactiveRestartConsolePct(t, pool, "75") // opus-4-6 CLIContext=200000 => 150000 ceiling

	sid, err := svc.Create("claude", "opus-4-6", "high", chatTestProjectID, "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	digestRepo := repo.NewRefineryDigestRepo(pool, clock.Real())
	if _, err := digestRepo.Upsert(sid, chatTestProjectID, "some working-set digest"); err != nil {
		t.Fatalf("Upsert digest: %v", err)
	}
	sess, _ := svc.get(sid)
	sess.noteContextLeft(95) // 5% used of 200000 = 10000 tokens, well under the 150000 ceiling

	if svc.maybeRotate(sess) {
		t.Error("maybeRotate() = true while under threshold, want false")
	}
}

// TestChatService_Rotation_FullFlow drives the rotation end to end through
// the real EventTurnCompleted->pumpChatEvents->maybeRotate path (not a
// direct maybeRotate call): a claude-engine chat with a refinery digest and
// context usage over threshold rotates in place under the same session id,
// preserving history and the open row, emitting console.context_rotated,
// and answering the next SendMessage from the fresh engine.
func TestChatService_Rotation_FullFlow(t *testing.T) {
	t.Parallel()
	svc, pool, hub, factory := newChatTestService(t)
	setProactiveRestartConsolePct(t, pool, "50")
	if err := service.NewGlobalSettingsService(pool, svc.deps.Clock).Set("proactive_restart_min_interval_sec", "0"); err != nil {
		t.Fatalf("set min interval: %v", err)
	}

	sid, err := svc.Create("claude", "opus-4-6", "high", chatTestProjectID, "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	oldEng := factory.last()
	if oldEng.spec().MaxContext != 200000 {
		t.Fatalf("oldEng maxContext = %d, want 200000 (opus-4-6 CLIContext)", oldEng.spec().MaxContext)
	}

	digestRepo := repo.NewRefineryDigestRepo(pool, svc.deps.Clock)
	if _, err := digestRepo.Upsert(sid, chatTestProjectID, "carried-forward working set"); err != nil {
		t.Fatalf("Upsert digest: %v", err)
	}

	sess, ok := svc.get(sid)
	if !ok {
		t.Fatal("session not found after Create")
	}
	sess.noteContextLeft(0) // over threshold

	ch := subscribeChatSession(t, hub, sid)
	oldEng.simulateAssistantText(sid, chatTestProjectID, "hello from before rotation")

	oldEng.emit(spawner.EngineEvent{Type: spawner.EventTurnCompleted, SessionID: sid})
	rotatedEv := waitForEventType(t, ch, ws.EventConsoleContextRotated, 2*time.Second)
	if rotatedEv.Data["session_id"] != sid {
		t.Errorf("console.context_rotated session_id = %v, want %q", rotatedEv.Data["session_id"], sid)
	}

	if !oldEng.isStopped() {
		t.Error("old engine was not stopped by rotation")
	}

	factory.mu.Lock()
	engineCount := len(factory.engines)
	factory.mu.Unlock()
	if engineCount != 2 {
		t.Fatalf("engines constructed after rotation = %d, want 2", engineCount)
	}
	newEng := factory.last()
	if newEng == oldEng {
		t.Fatal("factory.last() after rotation returned the same engine instance")
	}
	if !newEng.started {
		t.Error("new engine was not started")
	}
	if newEng.spec().SessionID != sid {
		t.Errorf("new engine spec.SessionID = %q, want %q (stable console identity)", newEng.spec().SessionID, sid)
	}

	// Session row stays open under the same id (chat history preserved).
	row, err := repo.NewAgentSessionRepo(pool, svc.deps.Clock).GetConsoleChat(sid)
	if err != nil {
		t.Fatalf("GetConsoleChat: %v", err)
	}
	if row == nil {
		t.Fatal("GetConsoleChat = nil, want the row still present after rotation")
	}
	if row.Status != model.AgentSessionUserInteractive {
		t.Errorf("row.Status after rotation = %q, want user_interactive (not closed)", row.Status)
	}
	if row.EndedAt.Valid {
		t.Error("row.EndedAt set after rotation, want the session to stay open")
	}

	msgs, err := repo.NewAgentMessageRepo(pool, svc.deps.Clock).GetBySession(sid)
	if err != nil {
		t.Fatalf("GetBySession: %v", err)
	}
	found := false
	for _, m := range msgs {
		if m == "hello from before rotation" {
			found = true
		}
	}
	if !found {
		t.Error("agent_messages history lost the pre-rotation message")
	}

	// The next SendMessage must be answered by the fresh engine, not the old one.
	if err := svc.SendMessage(sid, "post-rotation message"); err != nil {
		t.Fatalf("SendMessage after rotation: %v", err)
	}
	if got := newEng.turnCount(); got != 1 {
		t.Errorf("new engine turn count = %d, want 1", got)
	}
	if got := oldEng.turnCount(); got != 0 {
		t.Errorf("old engine turn count after rotation = %d, want 0 (must not receive post-rotation turns)", got)
	}
}
