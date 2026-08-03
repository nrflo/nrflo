package spawner

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/spawner/apirun/provider/mock"
)

// markDelegationCompleted (the fanout-end completion stamp) flips the row out
// of 'running' without consuming: findings stay in place and consumed_at
// stays NULL, so an abandoned delegation no longer reads running forever
// while its result remains fetchable by a later GetDelegation.
func TestMarkDelegationCompleted_StampsCompletionWithoutConsuming(t *testing.T) {
	env := setupDelegateTestEnv(t)
	defer env.cleanup()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	workerSID := "worker-session-unpolled"
	if err := repo.NewAgentSessionRepo(env.database, clock.Real()).Create(&model.AgentSession{
		ID:                 workerSID,
		ProjectID:          env.projectID,
		WorkflowInstanceID: env.wfiID,
		AgentType:          "_t2_extractor",
		Status:             model.AgentSessionCompleted,
		Result:             sql.NullString{String: "pass", Valid: true},
		StartedAt:          sql.NullString{String: now, Valid: true},
	}); err != nil {
		t.Fatalf("create worker session: %v", err)
	}
	findingRepo := repo.NewFindingRepo(env.database, clock.Real())
	if err := findingRepo.Upsert("session", workerSID, "_delegate_findings", json.RawMessage(`"the answer"`), repo.Denorm{}, repo.Actor{Source: "test", ID: "t"}); err != nil {
		t.Fatalf("seed worker finding: %v", err)
	}

	delegationID := env.wfiID + ".abandoned"
	seedDelegationRow(t, env, delegationID, "extractor", []string{workerSID}, nil, true)

	sp := buildDelegateSpawner(t, env, mock.New())
	sp.markDelegationCompleted(delegationID)

	delegationRepo := repo.NewDelegationRepo(env.pool, clock.Real())
	d, err := delegationRepo.Get(delegationID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if d.Status != "completed" {
		t.Errorf("Status = %q, want completed (stamped at fanout end, no poll)", d.Status)
	}
	if d.CompletedAt == nil {
		t.Error("CompletedAt = nil, want set")
	}
	if d.ConsumedAt != nil {
		t.Error("ConsumedAt set by completion stamp, want nil (consumption belongs to GetDelegation)")
	}
	findings, err := findingRepo.GetOwn("session", workerSID)
	if err != nil {
		t.Fatalf("GetOwn: %v", err)
	}
	if _, ok := findings["_delegate_findings"]; !ok {
		t.Fatal("_delegate_findings gone after completion stamp, want preserved for the consuming read")
	}

	// The consuming read still works after completion: results (with the
	// preserved findings) come back once, and the row flips to consumed.
	raw, err := sp.GetDelegation(context.Background(), env.callerSessionID, delegationID)
	if err != nil {
		t.Fatalf("GetDelegation: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out["status"] != "completed" {
		t.Errorf("status = %v, want completed", out["status"])
	}
	entry := out["results"].([]interface{})[0].(map[string]interface{})
	if entry["findings"] == nil {
		t.Errorf("entry = %+v, want findings preserved through the completion stamp", entry)
	}
	d2, err := delegationRepo.Get(delegationID)
	if err != nil {
		t.Fatalf("Get after consume: %v", err)
	}
	if d2.ConsumedAt == nil {
		t.Error("ConsumedAt = nil after terminal GetDelegation, want set")
	}
}

// A worker still running at stamp time leaves the row untouched — completion
// only lands once every worker is terminal.
func TestMarkDelegationCompleted_RunningWorker_NoStamp(t *testing.T) {
	env := setupDelegateTestEnv(t)
	defer env.cleanup()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	workerSID := "worker-session-still-running"
	if err := repo.NewAgentSessionRepo(env.database, clock.Real()).Create(&model.AgentSession{
		ID:                 workerSID,
		ProjectID:          env.projectID,
		WorkflowInstanceID: env.wfiID,
		AgentType:          "_t2_extractor",
		Status:             model.AgentSessionRunning,
		StartedAt:          sql.NullString{String: now, Valid: true},
	}); err != nil {
		t.Fatalf("create worker session: %v", err)
	}

	delegationID := env.wfiID + ".stillrunning"
	seedDelegationRow(t, env, delegationID, "extractor", []string{workerSID}, nil, true)

	sp := buildDelegateSpawner(t, env, mock.New())
	sp.markDelegationCompleted(delegationID)

	d, err := repo.NewDelegationRepo(env.pool, clock.Real()).Get(delegationID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if d.Status != "running" || d.CompletedAt != nil {
		t.Errorf("Status/CompletedAt = %q/%v, want running/nil while a worker is live", d.Status, d.CompletedAt)
	}
}
