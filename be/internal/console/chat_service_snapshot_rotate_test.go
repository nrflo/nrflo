package console

import "testing"

// TestSnapshot_RotateAtPct verifies the detail snapshot reports the
// proactive-rotation ceiling as a percentage of the context window: the
// config pct for a profile-less chat, the (smaller) profile budget for a
// budgeted profile, and 0 when rotation is disabled.
func TestSnapshot_RotateAtPct(t *testing.T) {
	t.Parallel()
	svc, pool, _, _ := newChatTestService(t)
	setProactiveRestartConsolePct(t, pool, "50")

	sid, err := svc.Create("claude", "opus-4-6", "high", chatTestProjectID, "", "", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	snap, ok := svc.Snapshot(sid)
	if !ok {
		t.Fatal("Snapshot: no live session")
	}
	if snap.RotateAtPct != 50 {
		t.Errorf("RotateAtPct (no profile) = %d, want 50 (config pct of window)", snap.RotateAtPct)
	}

	// t0-decider's 50k budget caps the 200k-window ceiling at 25%.
	deciderSID, err := svc.Create("claude", "", "", chatTestProjectID, "", "t0-decider", false)
	if err != nil {
		t.Fatalf("Create t0-decider: %v", err)
	}
	snap, ok = svc.Snapshot(deciderSID)
	if !ok {
		t.Fatal("Snapshot: no live t0-decider session")
	}
	if snap.RotateAtPct != 25 {
		t.Errorf("RotateAtPct (t0-decider) = %d, want 25 (50000 budget / 200000 window)", snap.RotateAtPct)
	}

	setProactiveRestartConsolePct(t, pool, "0")
	snap, _ = svc.Snapshot(sid)
	if snap.RotateAtPct != 0 {
		t.Errorf("RotateAtPct (rotation disabled) = %d, want 0", snap.RotateAtPct)
	}
}
