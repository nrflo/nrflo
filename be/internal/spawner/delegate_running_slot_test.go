package spawner

import (
	"context"
	"encoding/json"
	"testing"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/spawner/apirun"
)

// TestDelegate_RunningWorkerSlot_ResolvesWhileStillRunning verifies that a
// held worker's slot is recorded (registration-time write, before its Spawn
// call returns) with its agent_sessions row still `running`, and that
// DepthForSession resolves the delegation's depth for that still-running
// worker — the second-order fix that closes the delegate_max_depth escape
// (be/internal/spawner/REFERENCE.md ## Delegate): a worker that itself
// delegates before finishing now seeds a depth-2 row instead of depth 1.
func TestDelegate_RunningWorkerSlot_ResolvesWhileStillRunning(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-fake-key")

	env := setupDelegateTestEnv(t)
	defer env.cleanup()

	prov := &itemGatedProvider{
		gateOnSubstring: "held-item",
		release:         make(chan struct{}),
		startedGated:    make(chan struct{}),
	}
	sp := buildDelegateSpawner(t, env, prov)

	startRaw, err := sp.Delegate(context.Background(), env.callerSessionID, apirun.DelegateRequest{
		Tier:   "executor",
		Brief:  "review ${DELEGATE_ITEM}",
		Fanout: []string{"held-item"},
	})
	if err != nil {
		t.Fatalf("Delegate() error: %v", err)
	}
	var start map[string]interface{}
	if err := json.Unmarshal([]byte(startRaw), &start); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	delegationID := start["delegation_id"].(string)

	// Wait for the worker to actually start running (gated inside Run()), then
	// wait for its slot to land — the registration-time write fires before
	// Spawn returns, so this must resolve while the worker is still held.
	<-prov.startedGated
	waitForSlotFilled(t, env, delegationID)

	d, err := repo.NewDelegationRepo(env.pool, clock.Real()).Get(delegationID)
	if err != nil {
		t.Fatalf("Get delegation row: %v", err)
	}
	if d.FanoutDone {
		t.Fatal("FanoutDone = true, want false while the held worker is still running")
	}
	var heldSID string
	for _, sid := range d.WorkerSessionIDs {
		if sid != "" {
			heldSID = sid
		}
	}
	if heldSID == "" {
		t.Fatal("held worker's slot was not recorded while it was still running")
	}

	// 1. The held worker's agent_sessions row must still be `running` at the
	// moment its slot was recorded — this is what makes DepthForSession able
	// to resolve a genuinely running worker instead of only a finished one.
	sessionRepo := repo.NewAgentSessionRepo(env.pool, clock.Real())
	heldSession, err := sessionRepo.Get(heldSID)
	if err != nil {
		t.Fatalf("Get(%q): %v", heldSID, err)
	}
	if heldSession.Status != model.AgentSessionRunning {
		t.Errorf("held worker session status = %q, want %q", heldSession.Status, model.AgentSessionRunning)
	}

	// 2. DepthForSession must resolve the delegation's depth (1) for the
	// still-running worker's session id.
	depth, err := repo.NewDelegationRepo(env.pool, clock.Real()).DepthForSession(heldSID)
	if err != nil {
		t.Fatalf("DepthForSession(%q): %v", heldSID, err)
	}
	if depth != 1 {
		t.Errorf("DepthForSession(heldSID) = %d, want 1 while the worker is mid-run", depth)
	}

	// 3. Nested sp.Delegate from the held worker's own session id — as if the
	// running worker itself delegated further — must seed a depth-2 row. Pre-fix,
	// DepthForSession never found the still-running worker, so this fell back
	// to depth 1 (the delegate_max_depth escape closed by the registration-time
	// write).
	nestedProv := &itemGatedProvider{
		gateOnSubstring: "never-matches",
		release:         make(chan struct{}),
		startedGated:    make(chan struct{}),
	}
	close(nestedProv.release) // never gated for this call, run to completion immediately
	nestedSp := buildDelegateSpawner(t, env, nestedProv)

	nestedStartRaw, err := nestedSp.Delegate(context.Background(), heldSID, apirun.DelegateRequest{
		Tier:  "extractor",
		Brief: "nested from running worker",
	})
	if err != nil {
		t.Fatalf("nested Delegate() error: %v", err)
	}
	var nestedStart map[string]interface{}
	if err := json.Unmarshal([]byte(nestedStartRaw), &nestedStart); err != nil {
		t.Fatalf("unmarshal nested start: %v", err)
	}
	nestedDelegationID := nestedStart["delegation_id"].(string)

	nestedRow, err := repo.NewDelegationRepo(env.pool, clock.Real()).Get(nestedDelegationID)
	if err != nil {
		t.Fatalf("Get nested delegation row: %v", err)
	}
	if nestedRow.Depth != 2 {
		t.Errorf("nested delegation depth = %d, want 2 (running worker's own depth 1 + 1)", nestedRow.Depth)
	}

	waitForDelegationDone(t, nestedSp, heldSID, nestedDelegationID)

	// Release the original held worker and let its delegation finish too.
	close(prov.release)
	waitForDelegationDone(t, sp, env.callerSessionID, delegationID)
}
