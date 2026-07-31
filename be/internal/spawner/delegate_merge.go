package spawner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"be/internal/repo"
	"be/internal/service"
)

// worktreeMergeDelegateBranch is a swappable seam (mirrors
// worktreeSetupFromHEAD) so tests can fake git.
var worktreeMergeDelegateBranch = func(projectRoot, branchName string) (string, bool, error) {
	return (&service.WorktreeService{}).MergeDelegateBranch(projectRoot, branchName)
}

// MergeDelegation implements apirun.Delegator: merges an isolated
// delegation's server-committed branch into the live checkout's current
// branch, server-side. This is the sanctioned path for landing executor
// results — workers and callers never run git against the live tree
// themselves. Idempotent: an already-merged branch reports merged with
// already_merged=true.
func (s *Spawner) MergeDelegation(ctx context.Context, callerSessionID, delegationID string) (string, error) {
	pool := s.pool()
	if pool == nil {
		return "", fmt.Errorf("delegate: no database pool")
	}
	if !strings.Contains(delegationID, ".") {
		return "", fmt.Errorf("delegate: malformed delegation_id %q", delegationID)
	}

	callerSession, err := repo.NewAgentSessionRepo(pool, s.config.Clock).Get(callerSessionID)
	if err != nil {
		return "", fmt.Errorf("delegate: resolve caller session: %w", err)
	}

	delegationRepo := repo.NewDelegationRepo(pool, s.config.Clock)
	d, err := delegationRepo.Get(delegationID)
	if err != nil {
		return "", fmt.Errorf("delegate: unknown delegation %q", delegationID)
	}
	if !strings.EqualFold(d.ProjectID, callerSession.ProjectID) {
		return "", fmt.Errorf("delegate: delegation %q was not started by this caller", delegationID)
	}
	if d.Status == "running" {
		return "", fmt.Errorf("delegate: delegation %q is still running — collect it with get_delegation first", delegationID)
	}
	if d.BranchName == "" {
		return "", fmt.Errorf("delegate: delegation %q has no server-committed branch (ran in-place or committed nothing)", delegationID)
	}

	project, err := repo.NewProjectRepo(pool, s.config.Clock).Get(d.ProjectID)
	if err != nil || !project.RootPath.Valid || project.RootPath.String == "" {
		return "", fmt.Errorf("delegate: cannot resolve project root for %q", d.ProjectID)
	}

	mergeCommit, alreadyMerged, err := worktreeMergeDelegateBranch(project.RootPath.String, d.BranchName)
	if err != nil {
		return "", err
	}

	persistDelegationMerge(delegationRepo, d.ID, d.Summary, mergeCommit)

	out := map[string]interface{}{
		"delegation_id": delegationID,
		"status":        "merged",
		"branch":        d.BranchName,
		"merge_commit":  mergeCommit,
	}
	if alreadyMerged {
		out["already_merged"] = true
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

// persistDelegationMerge stamps merged/merge_commit onto the delegation's
// worktree summary JSON so the durable record reflects that the branch
// landed (best-effort, mirrors finalizeDelegateWorktree's persistence).
func persistDelegationMerge(delegationRepo *repo.DelegationRepo, delegationID, summary, mergeCommit string) {
	v := map[string]interface{}{}
	if summary != "" {
		json.Unmarshal([]byte(summary), &v) //nolint:errcheck
	}
	v["merged"] = true
	v["merge_commit"] = mergeCommit
	if b, err := json.Marshal(v); err == nil {
		delegationRepo.SetWorktreeSummary(delegationID, string(b)) //nolint:errcheck
	}
}
