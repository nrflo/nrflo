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

func TestGetDelegation_MalformedID_ReturnsError(t *testing.T) {
	env := setupDelegateTestEnv(t)
	defer env.cleanup()

	sp := buildDelegateSpawner(t, env, mock.New())

	_, err := sp.GetDelegation(context.Background(), env.callerSessionID, "not-a-valid-id")
	if err == nil {
		t.Fatal("GetDelegation() returned nil error; want error for malformed delegation_id")
	}
}

func TestGetDelegation_UnknownID_ReturnsError(t *testing.T) {
	env := setupDelegateTestEnv(t)
	defer env.cleanup()

	sp := buildDelegateSpawner(t, env, mock.New())

	_, err := sp.GetDelegation(context.Background(), env.callerSessionID, env.wfiID+".doesnotexist")
	if err == nil {
		t.Fatal("GetDelegation() returned nil error; want error for unknown delegation_id")
	}
}

func TestGetDelegation_CrossProjectCaller_Rejected(t *testing.T) {
	env := setupDelegateTestEnv(t)
	defer env.cleanup()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	otherProjectID := "other-project-delegate"
	if _, err := env.database.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		otherProjectID, "Other", now, now); err != nil {
		t.Fatalf("insert other project: %v", err)
	}
	otherSID := "other-project-session"
	if err := repo.NewAgentSessionRepo(env.database, clock.Real()).Create(&model.AgentSession{
		ID:        otherSID,
		ProjectID: otherProjectID,
		AgentType: "implementor",
		Status:    model.AgentSessionRunning,
		StartedAt: sql.NullString{String: now, Valid: true},
	}); err != nil {
		t.Fatalf("create other-project session: %v", err)
	}

	// Seed a tracking finding directly on the caller's own instance.
	findingRepo := repo.NewFindingRepo(env.pool, clock.Real())
	delegationID := env.wfiID + ".seed"
	val, _ := json.Marshal(map[string]interface{}{"tier": "extractor", "session_ids": []string{}})
	if err := findingRepo.Upsert("workflow_instance", env.wfiID, "_delegation_"+delegationID, val,
		repo.Denorm{ProjectID: env.projectID, WorkflowInstanceID: env.wfiID}, repo.Actor{Source: "system", ID: "delegate"}); err != nil {
		t.Fatalf("seed tracking finding: %v", err)
	}

	sp := buildDelegateSpawner(t, env, mock.New())

	_, err := sp.GetDelegation(context.Background(), otherSID, delegationID)
	if err == nil {
		t.Fatal("GetDelegation() returned nil error; want rejection for a caller from a different project")
	}
}

func TestGetDelegation_FailedWorker_AggregatesFailedStatus(t *testing.T) {
	env := setupDelegateTestEnv(t)
	defer env.cleanup()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	workerSID := "worker-session-failed"
	if err := repo.NewAgentSessionRepo(env.database, clock.Real()).Create(&model.AgentSession{
		ID:                 workerSID,
		ProjectID:          env.projectID,
		WorkflowInstanceID: env.wfiID,
		AgentType:          "_t2_extractor",
		Status:             model.AgentSessionFailed,
		Result:             sql.NullString{String: "fail", Valid: true},
		ResultReason:       sql.NullString{String: "could not answer", Valid: true},
		StartedAt:          sql.NullString{String: now, Valid: true},
	}); err != nil {
		t.Fatalf("create worker session: %v", err)
	}

	findingRepo := repo.NewFindingRepo(env.pool, clock.Real())
	delegationID := env.wfiID + ".failtest"
	val, _ := json.Marshal(map[string]interface{}{"tier": "extractor", "session_ids": []string{workerSID}, "done": true})
	if err := findingRepo.Upsert("workflow_instance", env.wfiID, "_delegation_"+delegationID, val,
		repo.Denorm{ProjectID: env.projectID, WorkflowInstanceID: env.wfiID}, repo.Actor{Source: "system", ID: "delegate"}); err != nil {
		t.Fatalf("seed tracking finding: %v", err)
	}

	sp := buildDelegateSpawner(t, env, mock.New())

	raw, err := sp.GetDelegation(context.Background(), env.callerSessionID, delegationID)
	if err != nil {
		t.Fatalf("GetDelegation: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out["status"] != "failed" {
		t.Errorf("status = %v, want failed", out["status"])
	}
	results := out["results"].([]interface{})
	if len(results) != 1 {
		t.Fatalf("results = %v, want 1 entry", results)
	}
	entry := results[0].(map[string]interface{})
	if entry["status"] != "failed" || entry["reason"] != "could not answer" {
		t.Errorf("entry = %+v, want status=failed reason='could not answer'", entry)
	}
}
