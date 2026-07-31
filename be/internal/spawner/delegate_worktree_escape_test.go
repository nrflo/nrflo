package spawner

import (
	"encoding/json"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/repo"
	"be/internal/service"
)

// fakeWorktreeLiveHead swaps worktreeLiveHead for the duration of the test,
// restoring the real (service-backed) implementation on cleanup.
func fakeWorktreeLiveHead(t *testing.T, fn func(projectRoot string) (string, error)) {
	t.Helper()
	orig := worktreeLiveHead
	worktreeLiveHead = fn
	t.Cleanup(func() { worktreeLiveHead = orig })
}

// prepFinalizeEnv seeds a delegation row with SetWorktree already applied
// (mirrors what prepareAndPersistDelegateWorktree does before finalize
// runs), so finalizeDelegateWorktree's SetWorktree("", ...)/SetWorktreeSummary
// calls have a real row to act on.
func prepFinalizeEnv(t *testing.T) (env *delegateTestEnv, delegationID string) {
	t.Helper()
	env = setupDelegateTestEnv(t)
	setProjectRootPath(t, env, "/tmp/fake-project-root")
	delegationID = env.wfiID + ".escape1"
	seedDelegationRow(t, env, delegationID, "executor", []string{""}, nil, true)
	if err := repo.NewDelegationRepo(env.pool, clock.Real()).SetWorktree(delegationID, "/tmp/wt", "nrdelegate/escape1", "base-sha-1"); err != nil {
		t.Fatalf("SetWorktree: %v", err)
	}
	return env, delegationID
}

func decodeSummary(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("unmarshal summary %q: %v", raw, err)
	}
	return out
}

// TestFinalizeDelegateWorktree_MovedHead_StampsSummaryAndWarns covers the
// core acceptance-criterion-B path: a no-commit delegation whose live HEAD
// moved during the run gets its summary stamped and a WARN logged, naming
// the delegation and using the "not proof of misbehavior" wording.
func TestFinalizeDelegateWorktree_MovedHead_StampsSummaryAndWarns(t *testing.T) {
	env, delegationID := prepFinalizeEnv(t)
	defer env.cleanup()

	fakeWorktreeCommit(t, func(projectRoot, worktreePath, branchName, baseCommit, delegationIDArg, briefHead string) (*service.DelegateWorktreeSummary, error) {
		return &service.DelegateWorktreeSummary{Committed: false}, nil
	})
	fakeWorktreeLiveHead(t, func(projectRoot string) (string, error) {
		return "head-sha-2", nil
	})
	buf := captureLog(t)

	sp := buildDelegateSpawner(t, env, nil)
	sp.finalizeDelegateWorktree(env.pool, env.projectID, delegationID, "/tmp/wt", "nrdelegate/escape1", "base-sha-1", "brief")

	d, err := repo.NewDelegationRepo(env.pool, clock.Real()).Get(delegationID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	summary := decodeSummary(t, d.Summary)
	if summary["committed"] != false {
		t.Errorf("committed = %v, want false", summary["committed"])
	}
	if summary["live_tree_mutated"] != true {
		t.Errorf("live_tree_mutated = %v, want true", summary["live_tree_mutated"])
	}
	if summary["head_before"] != "base-sha-1" {
		t.Errorf("head_before = %v, want base-sha-1", summary["head_before"])
	}
	if summary["head_after"] != "head-sha-2" {
		t.Errorf("head_after = %v, want head-sha-2", summary["head_after"])
	}

	logOut := buf.String()
	if !strings.Contains(logOut, "WARN") {
		t.Errorf("log output missing WARN: %q", logOut)
	}
	if !strings.Contains(logOut, "live tree HEAD moved during delegation") {
		t.Errorf("log output missing expected wording: %q", logOut)
	}
	if !strings.Contains(logOut, delegationID) {
		t.Errorf("log output missing delegation id %q: %q", delegationID, logOut)
	}
}

// TestFinalizeDelegateWorktree_UnmovedHead_NoStampNoWarn covers the
// unmoved-HEAD case: the summary stays exactly {"committed":false} (the
// three new keys must be absent, not present-with-zero-value) and nothing
// is logged.
func TestFinalizeDelegateWorktree_UnmovedHead_NoStampNoWarn(t *testing.T) {
	env, delegationID := prepFinalizeEnv(t)
	defer env.cleanup()

	fakeWorktreeCommit(t, func(projectRoot, worktreePath, branchName, baseCommit, delegationIDArg, briefHead string) (*service.DelegateWorktreeSummary, error) {
		return &service.DelegateWorktreeSummary{Committed: false}, nil
	})
	fakeWorktreeLiveHead(t, func(projectRoot string) (string, error) {
		return "base-sha-1", nil
	})
	buf := captureLog(t)

	sp := buildDelegateSpawner(t, env, nil)
	sp.finalizeDelegateWorktree(env.pool, env.projectID, delegationID, "/tmp/wt", "nrdelegate/escape1", "base-sha-1", "brief")

	d, err := repo.NewDelegationRepo(env.pool, clock.Real()).Get(delegationID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if d.Summary != `{"committed":false}` {
		t.Errorf("Summary = %q, want exactly {\"committed\":false}", d.Summary)
	}
	if buf.Len() != 0 {
		t.Errorf("log output = %q, want empty (no WARN for unmoved HEAD)", buf.String())
	}
}

// TestFinalizeDelegateWorktree_Committed_NeverChecksLiveHead verifies the
// check is gated on !summary.Committed: a successful commit never invokes
// worktreeLiveHead and never stamps/warns.
func TestFinalizeDelegateWorktree_Committed_NeverChecksLiveHead(t *testing.T) {
	env, delegationID := prepFinalizeEnv(t)
	defer env.cleanup()

	fakeWorktreeCommit(t, func(projectRoot, worktreePath, branchName, baseCommit, delegationIDArg, briefHead string) (*service.DelegateWorktreeSummary, error) {
		return &service.DelegateWorktreeSummary{Committed: true, ChangedFiles: []string{"a.go"}}, nil
	})
	called := false
	fakeWorktreeLiveHead(t, func(projectRoot string) (string, error) {
		called = true
		return "irrelevant", nil
	})
	buf := captureLog(t)

	sp := buildDelegateSpawner(t, env, nil)
	sp.finalizeDelegateWorktree(env.pool, env.projectID, delegationID, "/tmp/wt", "nrdelegate/escape1", "base-sha-1", "brief")

	if called {
		t.Error("worktreeLiveHead was invoked for a committed run, want never called")
	}
	if buf.Len() != 0 {
		t.Errorf("log output = %q, want empty for a committed run", buf.String())
	}

	d, err := repo.NewDelegationRepo(env.pool, clock.Real()).Get(delegationID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	summary := decodeSummary(t, d.Summary)
	if _, ok := summary["live_tree_mutated"]; ok {
		t.Errorf("summary %v has live_tree_mutated key, want absent for a committed run", summary)
	}
}

// TestFinalizeDelegateWorktree_LiveHeadSeamError_DegradesSilently covers the
// best-effort degrade rule: a seam error must not stamp the summary, must
// not panic, and leaves the delegation otherwise intact.
func TestFinalizeDelegateWorktree_LiveHeadSeamError_DegradesSilently(t *testing.T) {
	env, delegationID := prepFinalizeEnv(t)
	defer env.cleanup()

	fakeWorktreeCommit(t, func(projectRoot, worktreePath, branchName, baseCommit, delegationIDArg, briefHead string) (*service.DelegateWorktreeSummary, error) {
		return &service.DelegateWorktreeSummary{Committed: false}, nil
	})
	fakeWorktreeLiveHead(t, func(projectRoot string) (string, error) {
		return "", fakeErr("simulated git failure")
	})

	sp := buildDelegateSpawner(t, env, nil)
	sp.finalizeDelegateWorktree(env.pool, env.projectID, delegationID, "/tmp/wt", "nrdelegate/escape1", "base-sha-1", "brief")

	d, err := repo.NewDelegationRepo(env.pool, clock.Real()).Get(delegationID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if d.Summary != `{"committed":false}` {
		t.Errorf("Summary = %q, want exactly {\"committed\":false} on seam error", d.Summary)
	}
}

// TestFinalizeDelegateWorktree_EmptyBaseCommit_DegradesSilently covers the
// other degrade case: an empty baseCommit (e.g. a delegation whose worktree
// setup never resolved one) must skip the live-head check entirely rather
// than comparing against "".
func TestFinalizeDelegateWorktree_EmptyBaseCommit_DegradesSilently(t *testing.T) {
	env, delegationID := prepFinalizeEnv(t)
	defer env.cleanup()

	fakeWorktreeCommit(t, func(projectRoot, worktreePath, branchName, baseCommit, delegationIDArg, briefHead string) (*service.DelegateWorktreeSummary, error) {
		return &service.DelegateWorktreeSummary{Committed: false}, nil
	})
	called := false
	fakeWorktreeLiveHead(t, func(projectRoot string) (string, error) {
		called = true
		return "some-sha", nil
	})

	sp := buildDelegateSpawner(t, env, nil)
	sp.finalizeDelegateWorktree(env.pool, env.projectID, delegationID, "/tmp/wt", "nrdelegate/escape1", "", "brief")

	if called {
		t.Error("worktreeLiveHead was invoked with an empty baseCommit, want never called")
	}
	d, err := repo.NewDelegationRepo(env.pool, clock.Real()).Get(delegationID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if d.Summary != `{"committed":false}` {
		t.Errorf("Summary = %q, want exactly {\"committed\":false}", d.Summary)
	}
}
