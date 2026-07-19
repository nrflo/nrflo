package spawner

import (
	"context"
	"encoding/json"
	"testing"

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

	// Tracking finding must be cleaned up once the delegation is terminal.
	var count int
	if err := env.pool.QueryRow(`SELECT COUNT(*) FROM findings WHERE key LIKE '_delegation_%'`).Scan(&count); err != nil {
		t.Fatalf("count tracking findings: %v", err)
	}
	if count != 0 {
		t.Errorf("tracking finding count = %d, want 0 (deleted once terminal)", count)
	}
	// Worker's _delegate_findings must be deleted (read+delete, like _consult_answer).
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
	sp := buildDelegateSpawner(t, env, mock.New(manyDelegateWorkerScripts(len(items), "ok")...))

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
