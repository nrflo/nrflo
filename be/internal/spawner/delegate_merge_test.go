package spawner

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/repo"
	"be/internal/spawner/apirun/provider/mock"
)

// fakeMergeSeam installs a fake worktreeMergeDelegateBranch for the test's
// duration, returning a pointer to the branch name it was called with ("" =
// never called).
func fakeMergeSeam(t *testing.T, sha string, already bool, err error) *string {
	t.Helper()
	var calledBranch string
	orig := worktreeMergeDelegateBranch
	worktreeMergeDelegateBranch = func(projectRoot, branchName string) (string, bool, error) {
		calledBranch = branchName
		return sha, already, err
	}
	t.Cleanup(func() { worktreeMergeDelegateBranch = orig })
	return &calledBranch
}

func TestMergeDelegation_HappyPath_MergesAndStampsSummary(t *testing.T) {
	env := setupDelegateTestEnv(t)
	defer env.cleanup()
	setProjectRootPath(t, env, "/tmp/fake-project-root")

	delegationID := env.wfiID + ".merge1"
	seedDelegationRow(t, env, delegationID, "executor", []string{""}, nil, true)
	delegationRepo := repo.NewDelegationRepo(env.pool, clock.Real())
	if err := delegationRepo.SetWorktree(delegationID, "/tmp/x", "nrdelegate/merge1", "base1"); err != nil {
		t.Fatalf("SetWorktree: %v", err)
	}
	if _, err := delegationRepo.MarkCompleted(delegationID, "completed"); err != nil {
		t.Fatalf("MarkCompleted: %v", err)
	}
	calledBranch := fakeMergeSeam(t, "mergesha", false, nil)

	sp := buildDelegateSpawner(t, env, mock.New())
	raw, err := sp.MergeDelegation(context.Background(), env.callerSessionID, delegationID)
	if err != nil {
		t.Fatalf("MergeDelegation: %v", err)
	}
	if *calledBranch != "nrdelegate/merge1" {
		t.Errorf("seam called with branch %q, want nrdelegate/merge1", *calledBranch)
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out["status"] != "merged" || out["merge_commit"] != "mergesha" || out["branch"] != "nrdelegate/merge1" {
		t.Errorf("out = %v, want merged/mergesha/nrdelegate/merge1", out)
	}

	d, err := delegationRepo.Get(delegationID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	var summary map[string]interface{}
	if err := json.Unmarshal([]byte(d.Summary), &summary); err != nil {
		t.Fatalf("summary %q: %v", d.Summary, err)
	}
	if summary["merged"] != true || summary["merge_commit"] != "mergesha" {
		t.Errorf("summary = %v, want merged=true merge_commit=mergesha", summary)
	}
}

func TestMergeDelegation_RunningDelegation_Refused(t *testing.T) {
	env := setupDelegateTestEnv(t)
	defer env.cleanup()

	delegationID := env.wfiID + ".mergerun"
	seedDelegationRow(t, env, delegationID, "executor", []string{""}, nil, false)
	calledBranch := fakeMergeSeam(t, "", false, nil)

	sp := buildDelegateSpawner(t, env, mock.New())
	_, err := sp.MergeDelegation(context.Background(), env.callerSessionID, delegationID)
	if err == nil || !strings.Contains(err.Error(), "still running") {
		t.Fatalf("err = %v, want still-running refusal", err)
	}
	if *calledBranch != "" {
		t.Error("merge seam called for a running delegation")
	}
}

func TestMergeDelegation_NoBranch_Refused(t *testing.T) {
	env := setupDelegateTestEnv(t)
	defer env.cleanup()

	delegationID := env.wfiID + ".mergenb"
	seedDelegationRow(t, env, delegationID, "extractor", []string{""}, nil, true)
	if _, err := repo.NewDelegationRepo(env.pool, clock.Real()).MarkCompleted(delegationID, "completed"); err != nil {
		t.Fatalf("MarkCompleted: %v", err)
	}
	calledBranch := fakeMergeSeam(t, "", false, nil)

	sp := buildDelegateSpawner(t, env, mock.New())
	_, err := sp.MergeDelegation(context.Background(), env.callerSessionID, delegationID)
	if err == nil || !strings.Contains(err.Error(), "no server-committed branch") {
		t.Fatalf("err = %v, want no-branch refusal", err)
	}
	if *calledBranch != "" {
		t.Error("merge seam called for a branch-less delegation")
	}
}

func TestMergeDelegation_AlreadyMerged_Reported(t *testing.T) {
	env := setupDelegateTestEnv(t)
	defer env.cleanup()
	setProjectRootPath(t, env, "/tmp/fake-project-root")

	delegationID := env.wfiID + ".mergeam"
	seedDelegationRow(t, env, delegationID, "executor", []string{""}, nil, true)
	delegationRepo := repo.NewDelegationRepo(env.pool, clock.Real())
	if err := delegationRepo.SetWorktree(delegationID, "/tmp/x", "nrdelegate/mergeam", "base1"); err != nil {
		t.Fatalf("SetWorktree: %v", err)
	}
	if _, err := delegationRepo.MarkCompleted(delegationID, "completed"); err != nil {
		t.Fatalf("MarkCompleted: %v", err)
	}
	fakeMergeSeam(t, "headsha", true, nil)

	sp := buildDelegateSpawner(t, env, mock.New())
	raw, err := sp.MergeDelegation(context.Background(), env.callerSessionID, delegationID)
	if err != nil {
		t.Fatalf("MergeDelegation: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out["already_merged"] != true {
		t.Errorf("out = %v, want already_merged=true", out)
	}
}
