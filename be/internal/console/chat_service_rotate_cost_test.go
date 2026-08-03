package console

import (
	"testing"
	"time"

	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/ws"
)

// TestChatService_Rotation_PreservesLiveCostSnapshot drives the same
// EventTurnCompleted -> pumpChatEvents -> maybeRotate -> rotate path as
// TestChatService_Rotation_FullFlow, but asserts on the live
// spawner.SessionCost accounting rather than engine identity: rotation calls
// spawner.ResetSessionCostThread (clearing only the reported high water, see
// sessioncost_entry.go), so the session's accumulated cost snapshot must
// never regress across a rotation, and a fresh low report from the new
// thread must add on top of it rather than re-zeroing.
func TestChatService_Rotation_PreservesLiveCostSnapshot(t *testing.T) {
	t.Parallel()
	svc, pool, hub, factory := newChatTestService(t)
	setProactiveRestartConsolePct(t, pool, "50")
	if err := service.NewGlobalSettingsService(pool, svc.deps.Clock).Set("proactive_restart_min_interval_sec", "0"); err != nil {
		t.Fatalf("set min interval: %v", err)
	}

	sid, err := svc.Create("claude", "opus-4-6", "high", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { spawner.FinalizeSessionCost(sid) })

	// Simulate pre-rotation usage on the codex-cumulative shape (setUsage
	// drives the reported high water that resetReported clears).
	spawner.SetSessionCostUsage(sid, 300_000, 60_000, 0, 0)
	preRotation, ok := spawner.SessionCost(sid)
	if !ok {
		t.Fatal("SessionCost ok = false before rotation")
	}
	if preRotation.InputTokens == 0 {
		t.Fatal("pre-rotation snapshot is zero, setup failed")
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
	oldEng := factory.last()
	oldEng.emit(spawner.EngineEvent{Type: spawner.EventTurnCompleted, SessionID: sid})
	waitForEventType(t, ch, ws.EventConsoleContextRotated, 2*time.Second)

	// A fresh thread reports a low cumulative total post-rotation — must add
	// on top of the carried snapshot, never drop below preRotation.
	spawner.SetSessionCostUsage(sid, 5_000, 1_000, 0, 0)
	postRotation, ok := spawner.SessionCost(sid)
	if !ok {
		t.Fatal("SessionCost ok = false after rotation")
	}

	if postRotation.InputTokens < preRotation.InputTokens || postRotation.OutputTokens < preRotation.OutputTokens {
		t.Errorf("post-rotation snapshot = %+v, want at or above pre-rotation %+v (never a drop)", postRotation, preRotation)
	}
	wantIn := preRotation.InputTokens + 5_000
	wantOut := preRotation.OutputTokens + 1_000
	if postRotation.InputTokens != wantIn || postRotation.OutputTokens != wantOut {
		t.Errorf("post-rotation snapshot = in:%d out:%d, want in:%d out:%d (carried + fresh-thread delta)",
			postRotation.InputTokens, postRotation.OutputTokens, wantIn, wantOut)
	}
}
