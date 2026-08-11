package spawner

import (
	"context"
	"encoding/json"
	"testing"

	"be/internal/clock"
	"be/internal/repo"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider/mock"
)

// The verifier tier resolves _t3_verifier (migration 000239, tier-2 chain)
// through the same fanout machinery as extractor: sync round trip, findings
// delivered, durable delegations row stamped with tier="verifier".
func TestDelegate_VerifierTier_RoundTrip(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-fake-key")

	env := setupDelegateTestEnv(t)
	defer env.cleanup()

	sp := buildDelegateSpawner(t, env, mock.New(delegateWorkerScripts("claim refuted")...))

	startRaw, err := sp.Delegate(context.Background(), env.callerSessionID, apirun.DelegateRequest{
		Tier:  "verifier",
		Brief: "adversarially verify the claim",
	})
	if err != nil {
		t.Fatalf("Delegate() error: %v", err)
	}
	var start map[string]interface{}
	if err := json.Unmarshal([]byte(startRaw), &start); err != nil {
		t.Fatalf("unmarshal Delegate result: %v", err)
	}
	delegationID, _ := start["delegation_id"].(string)
	if delegationID == "" {
		t.Fatal("delegation_id is empty")
	}

	final := waitForDelegationDone(t, sp, env.callerSessionID, delegationID)
	if final["status"] != "completed" {
		t.Errorf("final status = %v, want completed", final["status"])
	}
	results, ok := final["results"].([]interface{})
	if !ok || len(results) != 1 {
		t.Fatalf("results = %v, want a single-element array", final["results"])
	}
	if entry := results[0].(map[string]interface{}); entry["findings"] == nil {
		t.Errorf("worker entry missing findings: %+v", entry)
	}

	d, err := repo.NewDelegationRepo(env.pool, clock.Real()).Get(delegationID)
	if err != nil {
		t.Fatalf("Get delegation row: %v", err)
	}
	if d.Tier != "verifier" {
		t.Errorf("delegation row tier = %q, want verifier", d.Tier)
	}
}

func TestDelegate_UnknownTier_Errors(t *testing.T) {
	env := setupDelegateTestEnv(t)
	defer env.cleanup()

	sp := buildDelegateSpawner(t, env, mock.New())
	_, err := sp.Delegate(context.Background(), env.callerSessionID, apirun.DelegateRequest{
		Tier:  "manager",
		Brief: "boss around",
	})
	if err == nil {
		t.Fatal("Delegate() with unknown tier succeeded, want error")
	}
}
