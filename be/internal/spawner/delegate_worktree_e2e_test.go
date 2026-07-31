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

// TestDelegate_WorktreeSetupFailure_DegradesInPlace_DelegationCompletes is
// the "degrade, never fail" guard end to end: a failed `worktree add` must
// not fail the delegation — it must run in-place and still complete
// normally, with no worktree metadata persisted or surfaced.
func TestDelegate_WorktreeSetupFailure_DegradesInPlace_DelegationCompletes(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-fake-key")

	env := setupDelegateTestEnv(t)
	defer env.cleanup()
	setProjectRootPath(t, env, "/tmp/fake-project-root")
	fakeWorktreeSetup(t, func(projectRoot, branchName string) (string, string, error) {
		return "", "", errFakeWorktreeSetup
	})

	sp := buildDelegateSpawner(t, env, mock.New(delegateWorkerScripts("in-place answer")...))

	startRaw, err := sp.Delegate(context.Background(), env.callerSessionID, apirun.DelegateRequest{
		Tier:  "executor",
		Brief: "do the work",
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
		t.Fatalf("final status = %v, want completed (setup failure must degrade, not fail)", final["status"])
	}
	if _, hasWorktree := final["worktree"]; hasWorktree {
		t.Errorf("final payload has a worktree block = %v, want absent (in-place run)", final["worktree"])
	}

	d, err := repo.NewDelegationRepo(env.pool, clock.Real()).Get(delegationID)
	if err != nil {
		t.Fatalf("Get delegation row: %v", err)
	}
	if d.WorktreePath != "" || d.BranchName != "" {
		t.Errorf("delegation row worktree fields = path=%q branch=%q, want both empty", d.WorktreePath, d.BranchName)
	}
}

// TestDelegate_ExecutorRunless_PersistsWorktreeAndReportsBranch is the
// success path end to end: an isolated executor delegation persists its
// worktree metadata on the delegations row and GetDelegation's terminal
// payload carries a "worktree" block naming the branch to merge.
func TestDelegate_ExecutorRunless_PersistsWorktreeAndReportsBranch(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-fake-key")

	env := setupDelegateTestEnv(t)
	defer env.cleanup()
	setProjectRootPath(t, env, "/tmp/fake-project-root")

	const fakePath, fakeBase = "/tmp/nrflo/worktrees/nrdelegate-fake2", "0ff1ce"
	fakeWorktreeSetup(t, func(projectRoot, branchName string) (string, string, error) {
		return fakePath, fakeBase, nil
	})
	fakeWorktreeCommit(t, func(projectRoot, worktreePath, branchName, baseCommit, delegationID, briefHead string) (*service.DelegateWorktreeSummary, error) {
		return &service.DelegateWorktreeSummary{Committed: true, ChangedFiles: []string{"impl.go"}, Diffstat: " 1 file changed, 3 insertions(+)"}, nil
	})

	// Console-style run-less caller (no bound workflow instance).
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := env.database.Exec(`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		service.GlobalProjectID, "Global Workflows", now, now); err != nil {
		t.Fatalf("seed global project: %v", err)
	}
	consoleSID := "console-session-worktree"
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

	sp := buildDelegateSpawner(t, env, mock.New(delegateWorkerScripts("isolated answer")...))

	startRaw, err := sp.Delegate(context.Background(), consoleSID, apirun.DelegateRequest{
		Tier:  "executor",
		Brief: "implement the feature",
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
		t.Fatalf("final status = %v, want completed", final["status"])
	}
	wt, ok := final["worktree"].(map[string]interface{})
	if !ok {
		t.Fatalf("final payload worktree block = %v, want a map", final["worktree"])
	}
	wantBranch := "nrdelegate/" + delegationIDToBranchSuffix(delegationID)
	if wt["branch"] != wantBranch {
		t.Errorf("worktree.branch = %v, want %q", wt["branch"], wantBranch)
	}
	if wt["merge_hint"] != "git merge "+wantBranch {
		t.Errorf("worktree.merge_hint = %v, want %q", wt["merge_hint"], "git merge "+wantBranch)
	}
	if wt["base_commit"] != fakeBase {
		t.Errorf("worktree.base_commit = %v, want %q", wt["base_commit"], fakeBase)
	}
	changed, _ := wt["changed_files"].([]interface{})
	if len(changed) != 1 || changed[0] != "impl.go" {
		t.Errorf("worktree.changed_files = %v, want [impl.go]", wt["changed_files"])
	}

	d, err := repo.NewDelegationRepo(env.pool, clock.Real()).Get(delegationID)
	if err != nil {
		t.Fatalf("Get delegation row: %v", err)
	}
	if d.WorktreePath != fakePath {
		t.Errorf("delegation row WorktreePath = %q, want %q", d.WorktreePath, fakePath)
	}
	if d.BranchName != wantBranch {
		t.Errorf("delegation row BranchName = %q, want %q", d.BranchName, wantBranch)
	}
	if d.BaseCommit != fakeBase {
		t.Errorf("delegation row BaseCommit = %q, want %q", d.BaseCommit, fakeBase)
	}
	if d.Summary == "" {
		t.Error("delegation row Summary is empty, want the persisted worktree summary JSON")
	}
}
