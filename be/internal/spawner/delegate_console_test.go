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
	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider/mock"
)

// TestDelegate_ConsoleCaller_NoBoundInstance_MintsHiddenHost exercises the
// console (run-less caller) branch of Delegate: a session with no
// WorkflowInstanceID gets a fresh hidden `_delegate_host` instance minted for
// the call instead of erroring.
func TestDelegate_ConsoleCaller_NoBoundInstance_MintsHiddenHost(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-fake-key")

	env := setupDelegateTestEnv(t)
	defer env.cleanup()

	// createDelegateHostInstance inserts a workflows row under
	// service.GlobalProjectID; in a real server that reserved project row is
	// seeded once at every `serve` startup by EnsureGlobalDynamicWorkflow
	// (cli/serve.go), independent of the delegate builtin. Reproduce that
	// precondition here.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.database.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		service.GlobalProjectID, "Global Workflows", now, now); err != nil {
		t.Fatalf("seed global project: %v", err)
	}

	// A console session has no bound workflow instance.
	consoleSID := "console-session-delegate"
	if err := repo.NewAgentSessionRepo(env.database, clock.Real()).Create(&model.AgentSession{
		ID:                 consoleSID,
		ProjectID:          env.projectID,
		WorkflowInstanceID: "",
		AgentType:          "console",
		Status:             model.AgentSessionRunning,
		StartedAt:          sql.NullString{String: now, Valid: true},
	}); err != nil {
		t.Fatalf("create console session: %v", err)
	}

	sp := buildDelegateSpawner(t, env, mock.New(delegateWorkerScripts("hidden host answer")...))

	startRaw, err := sp.Delegate(context.Background(), consoleSID, apirun.DelegateRequest{
		Tier:  "extractor",
		Brief: "answer from console",
	})
	if err != nil {
		t.Fatalf("Delegate() error: %v", err)
	}
	var start map[string]interface{}
	if err := json.Unmarshal([]byte(startRaw), &start); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	delegationID := start["delegation_id"].(string)

	final := waitForDelegationDone(t, sp, consoleSID, delegationID)
	if final["status"] != "completed" {
		t.Errorf("final status = %v, want completed", final["status"])
	}

	// The hidden host workflow + a fresh instance must now exist.
	var wfCount int
	if err := env.pool.QueryRow(`SELECT COUNT(*) FROM workflows WHERE id = '_delegate_host'`).Scan(&wfCount); err != nil {
		t.Fatalf("count hidden workflow: %v", err)
	}
	if wfCount != 1 {
		t.Errorf("_delegate_host workflow count = %d, want 1", wfCount)
	}

	// The delegations row must be scoped to the minted host instance and
	// the console caller — not the base test-env workflow instance.
	d, err := repo.NewDelegationRepo(env.pool, clock.Real()).Get(delegationID)
	if err != nil {
		t.Fatalf("Get delegation row: %v", err)
	}
	if d.CallerSessionID != consoleSID {
		t.Errorf("delegation caller_session_id = %q, want %q", d.CallerSessionID, consoleSID)
	}
	if d.WorkflowInstanceID == env.wfiID || d.WorkflowInstanceID == "" {
		t.Errorf("delegation workflow_instance_id = %q, want a freshly minted host instance (not %q)", d.WorkflowInstanceID, env.wfiID)
	}
	var hostWfiCount int
	if err := env.pool.QueryRow(`SELECT COUNT(*) FROM workflow_instances WHERE id = ? AND workflow_id = '_delegate_host'`, d.WorkflowInstanceID).Scan(&hostWfiCount); err != nil {
		t.Fatalf("count host instance: %v", err)
	}
	if hostWfiCount != 1 {
		t.Errorf("host instance count for %q = %d, want 1", d.WorkflowInstanceID, hostWfiCount)
	}
}
