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

var _ apirun.Delegator = (*Spawner)(nil)

func TestDelegate_SyncRoundTrip_SingleWorker(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-fake-key")

	env := setupDelegateTestEnv(t)
	defer env.cleanup()

	sp := buildDelegateSpawner(t, env, mock.New(delegateWorkerScripts("the finding")...))

	startRaw, err := sp.Delegate(context.Background(), env.callerSessionID, apirun.DelegateRequest{
		Tier:  "extractor",
		Brief: "extract the version number",
	})
	if err != nil {
		t.Fatalf("Delegate() error: %v", err)
	}
	var start map[string]interface{}
	if err := json.Unmarshal([]byte(startRaw), &start); err != nil {
		t.Fatalf("unmarshal Delegate result: %v", err)
	}
	if start["status"] != "running" {
		t.Errorf("initial status = %v, want running", start["status"])
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
	entry := results[0].(map[string]interface{})
	if entry["status"] != "completed" {
		t.Errorf("worker status = %v, want completed", entry["status"])
	}
	if entry["findings"] == nil {
		t.Errorf("worker entry missing findings: %+v", entry)
	}

	// The durable delegations row survives a terminal GetDelegation — it is
	// marked completed + consumed, never deleted (migration 000216).
	d, err := repo.NewDelegationRepo(env.pool, clock.Real()).Get(delegationID)
	if err != nil {
		t.Fatalf("Get delegation row after terminal read: %v", err)
	}
	if d.Status != "completed" {
		t.Errorf("delegation row status = %q, want completed", d.Status)
	}
	if d.ConsumedAt == nil {
		t.Error("delegation row consumed_at = nil, want set after terminal GetDelegation")
	}

	// A second GetDelegation must return the terminal status with no results
	// and no error, instead of re-reading (and re-deleting) worker findings.
	secondRaw, err := sp.GetDelegation(context.Background(), env.callerSessionID, delegationID)
	if err != nil {
		t.Fatalf("second GetDelegation() error: %v", err)
	}
	var second map[string]interface{}
	if err := json.Unmarshal([]byte(secondRaw), &second); err != nil {
		t.Fatalf("unmarshal second GetDelegation result: %v", err)
	}
	if second["status"] != "completed" {
		t.Errorf("second GetDelegation status = %v, want completed", second["status"])
	}
	if second["consumed"] != true {
		t.Errorf("second GetDelegation consumed = %v, want true", second["consumed"])
	}
	if _, hasResults := second["results"]; hasResults {
		t.Errorf("second GetDelegation results = %v, want absent (consumed reads return no results)", second["results"])
	}

	// Worker's _delegate_findings must be deleted (read+delete, like _consult_answer).
	var count int
	if err := env.pool.QueryRow(`SELECT COUNT(*) FROM findings WHERE key = '_delegate_findings'`).Scan(&count); err != nil {
		t.Fatalf("count delegate findings: %v", err)
	}
	if count != 0 {
		t.Errorf("_delegate_findings count = %d, want 0 (deleted after readback)", count)
	}
}

func TestDelegate_Fanout_SpawnsOneWorkerPerItem(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-fake-key")

	env := setupDelegateTestEnv(t)
	defer env.cleanup()

	items := []string{"a.go", "b.go", "c.go"}
	sp := buildDelegateSpawner(t, env, newItemRoutedProvider(items, "ok"))

	startRaw, err := sp.Delegate(context.Background(), env.callerSessionID, apirun.DelegateRequest{
		Tier:   "executor",
		Brief:  "review ${DELEGATE_ITEM}",
		Fanout: items,
	})
	if err != nil {
		t.Fatalf("Delegate() error: %v", err)
	}
	var start map[string]interface{}
	if err := json.Unmarshal([]byte(startRaw), &start); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	delegationID := start["delegation_id"].(string)

	final := waitForDelegationDone(t, sp, env.callerSessionID, delegationID)
	if final["status"] != "completed" {
		t.Errorf("final status = %v, want completed", final["status"])
	}
	results, ok := final["results"].([]interface{})
	if !ok || len(results) != len(items) {
		t.Fatalf("results = %v, want %d entries", final["results"], len(items))
	}
	for _, r := range results {
		entry := r.(map[string]interface{})
		if entry["status"] != "completed" {
			t.Errorf("worker entry = %+v, want status completed", entry)
		}
	}
}

func TestDelegate_UnknownTier_ReturnsError(t *testing.T) {
	env := setupDelegateTestEnv(t)
	defer env.cleanup()

	sp := buildDelegateSpawner(t, env, mock.New())

	_, err := sp.Delegate(context.Background(), env.callerSessionID, apirun.DelegateRequest{
		Tier:  "manager",
		Brief: "do something",
	})
	if err == nil {
		t.Fatal("Delegate() returned nil error; want error for unknown tier")
	}
}
