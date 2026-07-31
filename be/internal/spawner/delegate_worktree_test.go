package spawner

import (
	"testing"

	"be/internal/clock"
	"be/internal/model"
	"be/internal/service"
	"be/internal/spawner/apirun/provider/mock"
)

// getSysDef loads a system agent definition row by tier id via the same
// service path Delegate() uses.
func getSysDef(t *testing.T, env *delegateTestEnv, id string) *model.SystemAgentDefinition {
	t.Helper()
	sysDef, err := service.NewSystemAgentDefinitionService(env.pool, clock.Real(), service.NewModelService(env.pool, clock.Real())).Get(id)
	if err != nil {
		t.Fatalf("load sysDef %q: %v", id, err)
	}
	return sysDef
}

// setProjectRootPath gives the test env's project a git root so
// prepareDelegateWorktree's project-resolution gate passes.
func setProjectRootPath(t *testing.T, env *delegateTestEnv, path string) {
	t.Helper()
	if _, err := env.database.Exec(`UPDATE projects SET root_path = ? WHERE id = ?`, path, env.projectID); err != nil {
		t.Fatalf("set project root_path: %v", err)
	}
}

// fakeWorktreeSetup swaps worktreeSetupFromHEAD for the duration of the test,
// restoring the real (service-backed) implementation on cleanup.
func fakeWorktreeSetup(t *testing.T, fn func(projectRoot, branchName string) (string, string, error)) {
	t.Helper()
	orig := worktreeSetupFromHEAD
	worktreeSetupFromHEAD = fn
	t.Cleanup(func() { worktreeSetupFromHEAD = orig })
}

// fakeWorktreeCommit swaps worktreeCommitAndCollect for the duration of the
// test, restoring the real implementation on cleanup.
func fakeWorktreeCommit(t *testing.T, fn func(projectRoot, worktreePath, branchName, baseCommit, delegationID, briefHead string) (*service.DelegateWorktreeSummary, error)) {
	t.Helper()
	orig := worktreeCommitAndCollect
	worktreeCommitAndCollect = fn
	t.Cleanup(func() { worktreeCommitAndCollect = orig })
}

// delegationIDToBranchSuffix mirrors prepareDelegateWorktree's dot->dash
// substitution so tests can predict the expected branch name without
// hardcoding the UUID suffix createDelegationRecord mints.
func delegationIDToBranchSuffix(delegationID string) string {
	out := []rune(delegationID)
	for i, r := range out {
		if r == '.' {
			out[i] = '-'
		}
	}
	return string(out)
}

type fakeErr string

func (e fakeErr) Error() string { return string(e) }

var errFakeWorktreeSetup = fakeErr("simulated worktree add failure")

// TestPrepareDelegateWorktree_ExecutorRunless_UsesWorktreeAsProjectRoot covers
// the isolated case: _t1_executor (isolate_worktree=1 by seed) + a run-less
// (console/host) caller + a resolvable project git root produces a worktree
// path that delegateWorkerProjectRoot then picks over the live root — the
// single seam every worker backend derives its cwd from.
func TestPrepareDelegateWorktree_ExecutorRunless_UsesWorktreeAsProjectRoot(t *testing.T) {
	env := setupDelegateTestEnv(t)
	defer env.cleanup()
	setProjectRootPath(t, env, "/tmp/fake-project-root")

	const fakePath, fakeBase = "/tmp/nrflo/worktrees/nrdelegate-fake", "cafef00d"
	fakeWorktreeSetup(t, func(projectRoot, branchName string) (string, string, error) {
		if projectRoot != "/tmp/fake-project-root" {
			t.Errorf("worktreeSetupFromHEAD projectRoot = %q, want the project's root_path", projectRoot)
		}
		return fakePath, fakeBase, nil
	})

	sp := buildDelegateSpawner(t, env, mock.New())
	sysDef := getSysDef(t, env, "_t1_executor")

	worktreePath, branchName, baseCommit := sp.prepareDelegateWorktree(env.pool, sysDef, true, env.projectID, "wfi-1.deadbeef")
	if worktreePath != fakePath {
		t.Errorf("worktreePath = %q, want %q", worktreePath, fakePath)
	}
	if baseCommit != fakeBase {
		t.Errorf("baseCommit = %q, want %q", baseCommit, fakeBase)
	}
	wantBranch := "nrdelegate/wfi-1-deadbeef"
	if branchName != wantBranch {
		t.Errorf("branchName = %q, want %q (dots replaced with dashes)", branchName, wantBranch)
	}
	if got := delegateWorkerProjectRoot("/live/root", worktreePath); got != fakePath {
		t.Errorf("delegateWorkerProjectRoot() = %q, want the worktree path %q", got, fakePath)
	}
}

// TestPrepareDelegateWorktree_Extractor_StaysInPlace verifies a tier that
// doesn't opt in (isolate_worktree=0 for _t2_extractor by seed) never gets a
// worktree, regardless of caller/config state.
func TestPrepareDelegateWorktree_Extractor_StaysInPlace(t *testing.T) {
	env := setupDelegateTestEnv(t)
	defer env.cleanup()
	setProjectRootPath(t, env, "/tmp/fake-project-root")

	sp := buildDelegateSpawner(t, env, mock.New())
	sysDef := getSysDef(t, env, "_t2_extractor")

	worktreePath, branchName, baseCommit := sp.prepareDelegateWorktree(env.pool, sysDef, true, env.projectID, "wfi-1.abc")
	if worktreePath != "" || branchName != "" || baseCommit != "" {
		t.Errorf("got (%q,%q,%q), want all empty for a non-isolating tier", worktreePath, branchName, baseCommit)
	}
	if got := delegateWorkerProjectRoot("/live/root", worktreePath); got != "/live/root" {
		t.Errorf("delegateWorkerProjectRoot() = %q, want the live root unchanged", got)
	}
}

// TestPrepareDelegateWorktree_WorkflowBoundCaller_StaysInPlace verifies a
// workflow-bound caller (isHost=false) never gets a worktree even for an
// isolate_worktree=1 tier — it already runs inside its own worktree
// (orchestrator_lifecycle.go), and isolating the delegate would hide the
// diff from the parent agent that asked for it (deliberate exclusion).
func TestPrepareDelegateWorktree_WorkflowBoundCaller_StaysInPlace(t *testing.T) {
	env := setupDelegateTestEnv(t)
	defer env.cleanup()
	setProjectRootPath(t, env, "/tmp/fake-project-root")

	sp := buildDelegateSpawner(t, env, mock.New())
	sysDef := getSysDef(t, env, "_t1_executor")

	worktreePath, branchName, baseCommit := sp.prepareDelegateWorktree(env.pool, sysDef, false, env.projectID, "wfi-1.abc")
	if worktreePath != "" || branchName != "" || baseCommit != "" {
		t.Errorf("got (%q,%q,%q), want all empty for a workflow-bound caller", worktreePath, branchName, baseCommit)
	}
}

// TestPrepareDelegateWorktree_IsolationDisabledByConfig_StaysInPlace verifies
// the operator-only delegate_worktree_isolation=false escape hatch degrades
// an otherwise-eligible executor delegation to in-place.
func TestPrepareDelegateWorktree_IsolationDisabledByConfig_StaysInPlace(t *testing.T) {
	env := setupDelegateTestEnv(t)
	defer env.cleanup()
	setProjectRootPath(t, env, "/tmp/fake-project-root")
	if err := env.pool.SetConfig(service.DelegateWorktreeIsolationKey, "false"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	sp := buildDelegateSpawner(t, env, mock.New())
	sysDef := getSysDef(t, env, "_t1_executor")

	worktreePath, _, _ := sp.prepareDelegateWorktree(env.pool, sysDef, true, env.projectID, "wfi-1.abc")
	if worktreePath != "" {
		t.Errorf("worktreePath = %q, want empty when isolation is disabled by config", worktreePath)
	}
}

// TestPrepareDelegateWorktree_NoProjectRoot_StaysInPlace verifies an
// unresolvable project git root (no root_path set) degrades to in-place
// rather than erroring.
func TestPrepareDelegateWorktree_NoProjectRoot_StaysInPlace(t *testing.T) {
	env := setupDelegateTestEnv(t)
	defer env.cleanup()
	// No root_path set on the test project (default state).

	sp := buildDelegateSpawner(t, env, mock.New())
	sysDef := getSysDef(t, env, "_t1_executor")

	worktreePath, _, _ := sp.prepareDelegateWorktree(env.pool, sysDef, true, env.projectID, "wfi-1.abc")
	if worktreePath != "" {
		t.Errorf("worktreePath = %q, want empty absent a project root_path", worktreePath)
	}
}
