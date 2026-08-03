package spawner

// Tests for workspaceContextBlock via loadTemplate: classification of
// Config.ProjectRoot vs projects.root_path into workspace-live-tree /
// workspace-worktree, and the unset-ProjectRoot no-op that keeps every
// pre-existing golden/prefix assertion elsewhere in this package byte-identical.

import (
	"context"
	"strings"
	"testing"

	"be/internal/clock"

	"github.com/google/uuid"
)

// newSpawnerWithRoot creates a Spawner with the test DB pool and a given
// Config.ProjectRoot (the seam workspaceContextBlock reads).
func (e *spawnerTestEnv) newSpawnerWithRoot(root string) *Spawner {
	return New(Config{
		DataPath:    e.dbPath,
		Pool:        e.pool,
		Clock:       clock.Real(),
		ProjectRoot: root,
	})
}

func TestWorkspaceContextBlock_LiveTree(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "WS-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)
	createAgentDef(t, env, "analyzer", "Main prompt body")

	root := env.projectRootPath(t)
	sp := env.newSpawnerWithRoot(root)
	result, _, _, err := sp.loadTemplate("analyzer", ticketID, env.project,
		"p", "c", "test", "claude:sonnet-5", "test-phase", "", nil, 0)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}

	if !strings.Contains(result, "## Workspace") {
		t.Fatalf("expected live-tree workspace block appended, got: %s", result)
	}
	if !strings.Contains(result, "live checkout") {
		t.Error("expected live-tree wording")
	}
	if !strings.Contains(result, "git branch --show-current") {
		t.Error("expected git branch --show-current anchor")
	}
	if !strings.Contains(result, "never derive a branch name from the ticket id") {
		t.Error("expected never-derive-from-ticket-id rule")
	}
	if strings.Contains(result, "worktree") {
		t.Errorf("live-tree block must not contain worktree wording, got: %s", result)
	}
	if !strings.Contains(result, root) {
		t.Errorf("expected WORK_ROOT (%s) expanded into block, got: %s", root, result)
	}
	if strings.Contains(result, "${") {
		t.Errorf("expected no residual ${...} placeholders, got: %s", result)
	}
}

func TestWorkspaceContextBlock_Worktree(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "WS-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)
	createAgentDef(t, env, "analyzer", "Main prompt body")

	worktreeRoot := t.TempDir()
	sp := env.newSpawnerWithRoot(worktreeRoot)
	result, _, _, err := sp.loadTemplate("analyzer", ticketID, env.project,
		"p", "c", "test", "claude:sonnet-5", "test-phase", "", nil, 0)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}

	if !strings.Contains(result, "isolated git worktree") {
		t.Errorf("expected worktree block appended, got: %s", result)
	}
	if strings.Contains(result, ticketID) {
		t.Errorf("worktree block must not contain the ticket id anywhere, got: %s", result)
	}
	if !strings.Contains(result, worktreeRoot) {
		t.Errorf("expected WORK_ROOT (%s) expanded into block, got: %s", worktreeRoot, result)
	}
	if strings.Contains(result, "${") {
		t.Errorf("expected no residual ${...} placeholders, got: %s", result)
	}
}

func TestWorkspaceContextBlock_UnsetProjectRoot_NoBlock(t *testing.T) {
	t.Parallel()
	for _, root := range []string{"", "."} {
		t.Run("root="+root, func(t *testing.T) {
			env := newSpawnerTestEnv(t)
			ticketID := "WS-" + uuid.New().String()[:6]
			env.initWorkflow(t, ticketID)
			createAgentDef(t, env, "analyzer", "Main prompt body")

			sp := env.newSpawnerWithRoot(root)
			result, _, _, err := sp.loadTemplate("analyzer", ticketID, env.project,
				"p", "c", "test", "claude:sonnet-5", "test-phase", "", nil, 0)
			if err != nil {
				t.Fatalf("loadTemplate failed: %v", err)
			}
			if result != "Main prompt body" {
				t.Errorf("result = %q, want byte-identical no-block render 'Main prompt body'", result)
			}
		})
	}
}

func TestWorkspaceContextBlock_MissingInjectable_DegradesGracefully(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "WS-" + uuid.New().String()[:6]
	env.initWorkflow(t, ticketID)
	createAgentDef(t, env, "analyzer", "Main prompt body")

	if _, err := env.pool.Exec(`DELETE FROM default_templates WHERE id IN ('workspace-live-tree', 'workspace-worktree')`); err != nil {
		t.Fatalf("delete workspace injectables: %v", err)
	}

	root := env.projectRootPath(t)
	sp := env.newSpawnerWithRoot(root)
	result, _, _, err := sp.loadTemplate("analyzer", ticketID, env.project,
		"p", "c", "test", "claude:sonnet-5", "test-phase", "", nil, 0)
	if err != nil {
		t.Fatalf("loadTemplate should not fail with missing injectable: %v", err)
	}
	if result != "Main prompt body" {
		t.Errorf("result = %q, want byte-identical no-block render 'Main prompt body'", result)
	}
}

// TestWorkspaceContextBlock_PrecedesStepwiseBlock verifies the workspace
// block is appended before appendStepwiseBlock's outline/current-step
// instruction, so the stepwise agent's current-step instruction stays the
// final text in the rendered prompt.
func TestWorkspaceContextBlock_PrecedesStepwiseBlock(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "WS-" + uuid.New().String()[:6]
	wfiID := env.initWorkflow(t, ticketID)
	createStepwiseAgentDef(t, env, "analyzer", threeSteps())

	root := env.projectRootPath(t)
	sp := env.newSpawnerWithRoot(root)
	def := sp.loadAgentDefinition("analyzer", env.project, "test")
	sp.snapshotStepCursor(context.Background(), def, wfiID, "analyzer")

	result, _, _, err := sp.loadTemplate("analyzer", ticketID, env.project, "p", "c", "test",
		"claude:sonnet-5", "analyzer", wfiID, nil, 0)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}

	wsIdx := strings.Index(result, "## Workspace")
	stepsIdx := strings.Index(result, "step 1 of 3")
	if wsIdx == -1 {
		t.Fatal("missing workspace block")
	}
	if stepsIdx == -1 {
		t.Fatal("missing stepwise outline")
	}
	if wsIdx >= stepsIdx {
		t.Errorf("workspace block (idx=%d) should precede stepwise outline (idx=%d)", wsIdx, stepsIdx)
	}
	if !strings.HasSuffix(result, "Instruction body one.") {
		t.Errorf("expected current-step instruction to remain the final text, got: %s", result)
	}
}

// projectRootPath returns the root_path seeded for env.project (set to a
// t.TempDir() by newSpawnerTestEnv).
func (e *spawnerTestEnv) projectRootPath(t *testing.T) string {
	t.Helper()
	var root string
	if err := e.pool.QueryRow(`SELECT root_path FROM projects WHERE id = ?`, e.project).Scan(&root); err != nil {
		t.Fatalf("projectRootPath: %v", err)
	}
	return root
}
