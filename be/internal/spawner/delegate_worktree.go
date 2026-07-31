package spawner

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
)

// Swappable seams (package-level func vars, mirrors mergeRetryDelay) so
// spawner tests can fake git without a real repo. worktreeLiveHead mirrors
// worktreeSetupFromHEAD/worktreeCommitAndCollect for the live-tree-escape
// check in finalizeDelegateWorktree.
var (
	worktreeSetupFromHEAD = func(projectRoot, branchName string) (string, string, error) {
		return (&service.WorktreeService{}).SetupFromHEAD(projectRoot, branchName)
	}
	worktreeCommitAndCollect = func(projectRoot, worktreePath, branchName, baseCommit, delegationID, briefHead string) (*service.DelegateWorktreeSummary, error) {
		return (&service.WorktreeService{}).CommitAndCollect(projectRoot, worktreePath, branchName, baseCommit, delegationID, briefHead)
	}
	worktreeLiveHead = func(projectRoot string) (string, error) {
		return (&service.WorktreeService{}).LiveHead(projectRoot)
	}
)

// delegateBranchPrefix names every per-delegation worktree branch;
// delegation ids are "<wfiID>.<rand8>" so the dot is swapped for a dash to
// keep the ref name valid and unique per delegation (concurrent console
// delegations cannot collide).
const delegateBranchPrefix = "nrdelegate/"

// prepareDelegateWorktree decides whether this delegation should run under a
// per-delegation git worktree instead of the live project tree, and if so
// creates it. Isolation requires: the tier definition opts in
// (sysDef.IsolateWorktree), the caller is run-less (isHost — a
// workflow-bound caller already runs inside its own worktree and expects the
// delegate's diff in that same tree, orchestrator_lifecycle.go:55-57), the
// operator config toggle is on, and the project has a git root. Any setup
// failure degrades to in-place execution (logged, not returned) rather than
// failing the delegation.
func (s *Spawner) prepareDelegateWorktree(pool *db.Pool, sysDef *model.SystemAgentDefinition, isHost bool, projectID, delegationID string) (worktreePath, branchName, baseCommit string) {
	if !sysDef.IsolateWorktree || !isHost {
		return "", "", ""
	}
	if !service.DelegateWorktreeIsolation(pool, projectID) {
		return "", "", ""
	}

	project, err := repo.NewProjectRepo(pool, s.config.Clock).Get(projectID)
	if err != nil || !project.RootPath.Valid || project.RootPath.String == "" {
		return "", "", ""
	}

	branchName = delegateBranchPrefix + strings.ReplaceAll(delegationID, ".", "-")
	path, base, err := worktreeSetupFromHEAD(project.RootPath.String, branchName)
	if err != nil {
		log.Printf("delegate: worktree setup failed for %s, degrading to in-place: %v", delegationID, err)
		return "", "", ""
	}
	return path, branchName, base
}

// prepareAndPersistDelegateWorktree wraps prepareDelegateWorktree with the
// SetWorktree persist step, called synchronously before Delegate returns.
func (s *Spawner) prepareAndPersistDelegateWorktree(pool *db.Pool, sysDef *model.SystemAgentDefinition, isHost bool, projectID, delegationID string) (worktreePath, branchName, baseCommit string, err error) {
	worktreePath, branchName, baseCommit = s.prepareDelegateWorktree(pool, sysDef, isHost, projectID, delegationID)
	if worktreePath == "" {
		return "", "", "", nil
	}
	if err := repo.NewDelegationRepo(pool, s.config.Clock).SetWorktree(delegationID, worktreePath, branchName, baseCommit); err != nil {
		return "", "", "", err
	}
	return worktreePath, branchName, baseCommit, nil
}

// delegateWorkerProjectRoot picks the worktree over the live project root
// when set — the single seam every backend derives its cwd from
// (spawner_prepare.go, backend_interactive.go, cli adapters, apirun's FS
// jail), so this isolates every backend without touching per-backend code.
func delegateWorkerProjectRoot(liveRoot, worktreePath string) string {
	if worktreePath != "" {
		return worktreePath
	}
	return liveRoot
}

// finalizeDelegateWorktree commits the worktree's combined diff (server-
// owned: the executor is told never to commit itself) and persists the
// resulting summary once the fanout's workers have all finished (called
// after wg.Wait), so a worker that failed or timed out still leaves its
// partial work recoverable on the branch. No-op when this delegation was
// not isolated (worktreePath == "").
func (s *Spawner) finalizeDelegateWorktree(pool *db.Pool, projectID, delegationID, worktreePath, branchName, baseCommit, briefHead string) {
	if worktreePath == "" {
		return
	}
	project, err := repo.NewProjectRepo(pool, s.config.Clock).Get(projectID)
	if err != nil || !project.RootPath.Valid || project.RootPath.String == "" {
		log.Printf("delegate: worktree finalize failed for %s: cannot resolve project root: %v", delegationID, err)
		return
	}

	summary, err := worktreeCommitAndCollect(project.RootPath.String, worktreePath, branchName, baseCommit, delegationID, briefHead)
	if err != nil {
		log.Printf("delegate: worktree commit/collect failed for %s: %v", delegationID, err)
		return
	}

	delegationRepo := repo.NewDelegationRepo(pool, s.config.Clock)
	if !summary.Committed {
		// Nothing landed: CommitAndCollect already deleted the now-pointless
		// branch, so clear it here too or GetDelegation's merge hint would
		// point at a dangling ref.
		if err := delegationRepo.SetWorktree(delegationID, worktreePath, "", baseCommit); err != nil {
			log.Printf("delegate: failed to clear empty-commit branch for %s: %v", delegationID, err)
		}
		stampLiveTreeMutation(summary, project.RootPath.String, delegationID, baseCommit)
	}
	b, _ := json.Marshal(summary)
	if err := delegationRepo.SetWorktreeSummary(delegationID, string(b)); err != nil {
		log.Printf("delegate: failed to persist worktree summary for %s: %v", delegationID, err)
	}
}

// stampLiveTreeMutation checks whether the project live tree's HEAD moved
// during a no-commit delegation and, if so, stamps summary with
// live_tree_mutated/head_before/head_after and logs a WARN naming the
// delegation. A HEAD move is not proof of worker misbehavior — the user may
// have committed concurrently, or another delegation may have merged — it is
// only a signal, best-effort and silent on any error.
func stampLiveTreeMutation(summary *service.DelegateWorktreeSummary, projectRoot, delegationID, baseCommit string) {
	if baseCommit == "" {
		return
	}
	head, err := worktreeLiveHead(projectRoot)
	if err != nil {
		logger.Warn(context.Background(), "delegate: live-tree HEAD check skipped", "delegation_id", delegationID, "error", err)
		return
	}
	if head == "" || head == baseCommit {
		return
	}
	summary.LiveTreeMutated = true
	summary.HeadBefore = baseCommit
	summary.HeadAfter = head
	logger.Warn(context.Background(), "delegate: live tree HEAD moved during delegation",
		"delegation_id", delegationID, "head_before", baseCommit, "head_after", head, "worktree_committed", false)
}

// briefHead returns the first line of brief, truncated, for use in the
// server-authored commit message.
func briefHead(brief string) string {
	line := brief
	if i := strings.IndexByte(brief, '\n'); i >= 0 {
		line = brief[:i]
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return "(no brief)"
	}
	const maxLen = 72
	if len(line) > maxLen {
		line = line[:maxLen] + "…"
	}
	return line
}
