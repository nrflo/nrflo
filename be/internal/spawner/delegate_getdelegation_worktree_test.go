package spawner

import (
	"context"
	"encoding/json"
	"testing"

	"be/internal/clock"
	"be/internal/repo"
	"be/internal/spawner/apirun/provider/mock"
)

// TestGetDelegation_ExtractorTier_NoWorktreeBlock verifies a non-isolated
// (extractor) delegation's payload has no "worktree" key at all — absent,
// not present-with-empty-values — since seedDelegationRow never sets
// branch_name for these tests.
func TestGetDelegation_ExtractorTier_NoWorktreeBlock(t *testing.T) {
	env := setupDelegateTestEnv(t)
	defer env.cleanup()

	delegationID := env.wfiID + ".noworktree"
	seedDelegationRow(t, env, delegationID, "extractor", []string{""}, nil, true)

	sp := buildDelegateSpawner(t, env, mock.New())

	raw, err := sp.GetDelegation(context.Background(), env.callerSessionID, delegationID)
	if err != nil {
		t.Fatalf("GetDelegation: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, hasWorktree := out["worktree"]; hasWorktree {
		t.Errorf("worktree = %v, want the key entirely absent for a non-isolated delegation", out["worktree"])
	}
}

// TestGetDelegation_IsolatedDelegation_WorktreeBlockPresent verifies an
// isolated delegation's terminal payload carries the branch/base_commit/
// merge_hint block, including the empty-changed-files case (no summary
// persisted yet — e.g. a poll racing finalizeDelegateWorktree).
func TestGetDelegation_IsolatedDelegation_WorktreeBlockPresent(t *testing.T) {
	env := setupDelegateTestEnv(t)
	defer env.cleanup()

	delegationID := env.wfiID + ".withworktree"
	seedDelegationRow(t, env, delegationID, "executor", []string{""}, nil, true)
	if err := repo.NewDelegationRepo(env.pool, clock.Real()).SetWorktree(delegationID, "/tmp/nrflo/worktrees/x", "nrdelegate/withworktree", "abc123"); err != nil {
		t.Fatalf("SetWorktree: %v", err)
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
	wt, ok := out["worktree"].(map[string]interface{})
	if !ok {
		t.Fatalf("worktree = %v, want a map", out["worktree"])
	}
	if wt["branch"] != "nrdelegate/withworktree" {
		t.Errorf("worktree.branch = %v, want nrdelegate/withworktree", wt["branch"])
	}
	if wt["base_commit"] != "abc123" {
		t.Errorf("worktree.base_commit = %v, want abc123", wt["base_commit"])
	}
	if wt["merge_hint"] != "git merge nrdelegate/withworktree" {
		t.Errorf("worktree.merge_hint = %v, want the git merge hint", wt["merge_hint"])
	}
	if _, hasChanged := wt["changed_files"]; hasChanged {
		t.Errorf("worktree.changed_files = %v, want absent (no summary persisted yet)", wt["changed_files"])
	}
}
