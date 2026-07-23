package stepengine

import (
	"errors"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
)

func stepwiseDef(id string, stepsJSON string) *model.AgentDefinition {
	return &model.AgentDefinition{
		ID:         id,
		PromptMode: promptModeStepwise,
		Steps:      &stepsJSON,
	}
}

func TestSnapshot_CreatesAtRevision1Index0EmptyCompleted(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-snap1", "wfi-snap1", "", "")

	e := New(pool, clock.NewTest(time.Now()), nil)
	def := stepwiseDef("def1", `[{"step_id":"s1","title":"t","instruction":"i"},{"step_id":"s2","title":"t2","instruction":"i2"}]`)

	got, err := e.Snapshot("wfi-snap1", "node-a", def)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if got.Revision != 1 {
		t.Errorf("Revision = %d, want 1", got.Revision)
	}
	if got.CurrentIndex != 0 {
		t.Errorf("CurrentIndex = %d, want 0", got.CurrentIndex)
	}
	if got.Completed != "[]" {
		t.Errorf("Completed = %q, want []", got.Completed)
	}
}

// TestSnapshot_RelaunchAfterAdvanceLeavesCursorUntouched is the
// relaunch-and-retry contract: a second Snapshot call after the cursor has
// advanced must not reset revision/current_index/completed.
func TestSnapshot_RelaunchAfterAdvanceLeavesCursorUntouched(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-snap2", "wfi-snap2", "", "")

	e := New(pool, clock.NewTest(time.Now()), nil)
	stepsJSON := `[{"step_id":"s1","title":"t","instruction":"i"},{"step_id":"s2","title":"t2","instruction":"i2"}]`
	def := stepwiseDef("def2", stepsJSON)

	if _, err := e.Snapshot("wfi-snap2", "node-a", def); err != nil {
		t.Fatalf("first Snapshot: %v", err)
	}

	ok, err := e.cursorRepo.Advance("wfi-snap2", "node-a", 1, 0, `[{"step_id":"s1","completed_at":"x"}]`)
	if err != nil || !ok {
		t.Fatalf("Advance setup: ok=%v err=%v", ok, err)
	}

	got, err := e.Snapshot("wfi-snap2", "node-a", def)
	if err != nil {
		t.Fatalf("second Snapshot: %v", err)
	}
	if got.Revision != 2 {
		t.Errorf("Revision after relaunch Snapshot = %d, want 2 (unchanged)", got.Revision)
	}
	if got.CurrentIndex != 1 {
		t.Errorf("CurrentIndex after relaunch Snapshot = %d, want 1 (unchanged)", got.CurrentIndex)
	}
	if got.Completed != `[{"step_id":"s1","completed_at":"x"}]` {
		t.Errorf("Completed after relaunch Snapshot = %q, want unchanged advanced value", got.Completed)
	}
}

func TestSnapshot_FullModeReturnsErrNotStepwise(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-snap3", "wfi-snap3", "", "")

	e := New(pool, clock.NewTest(time.Now()), nil)
	def := &model.AgentDefinition{ID: "def3", PromptMode: "full", Steps: nil}

	if _, err := e.Snapshot("wfi-snap3", "node-a", def); !errors.Is(err, ErrNotStepwise) {
		t.Errorf("Snapshot(full mode) error = %v, want ErrNotStepwise", err)
	}
}

func TestSnapshot_NilStepsReturnsErrNotStepwise(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-snap4", "wfi-snap4", "", "")

	e := New(pool, clock.NewTest(time.Now()), nil)
	def := &model.AgentDefinition{ID: "def4", PromptMode: promptModeStepwise, Steps: nil}

	if _, err := e.Snapshot("wfi-snap4", "node-a", def); !errors.Is(err, ErrNotStepwise) {
		t.Errorf("Snapshot(nil Steps) error = %v, want ErrNotStepwise", err)
	}
}

func TestSnapshot_NilDefReturnsErrNotStepwise(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-snap5", "wfi-snap5", "", "")

	e := New(pool, clock.NewTest(time.Now()), nil)
	if _, err := e.Snapshot("wfi-snap5", "node-a", nil); !errors.Is(err, ErrNotStepwise) {
		t.Errorf("Snapshot(nil def) error = %v, want ErrNotStepwise", err)
	}
}

func TestSnapshot_MalformedStepsJSONReturnsErrBadSnapshot(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-snap6", "wfi-snap6", "", "")

	e := New(pool, clock.NewTest(time.Now()), nil)

	tests := []struct {
		name  string
		steps string
	}{
		{"not JSON", "not json"},
		{"empty array", "[]"},
		{"empty string", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			def := stepwiseDef("def-bad-"+tc.name, tc.steps)
			if tc.steps == "" {
				def.Steps = nil // empty string Steps pointer already caught as ErrNotStepwise upstream
			}
			_, err := e.Snapshot("wfi-snap6", "node-"+tc.name, def)
			if tc.steps == "" {
				if !errors.Is(err, ErrNotStepwise) {
					t.Errorf("Snapshot(empty Steps string) error = %v, want ErrNotStepwise", err)
				}
				return
			}
			if !errors.Is(err, ErrBadSnapshot) {
				t.Errorf("Snapshot(%q) error = %v, want ErrBadSnapshot", tc.steps, err)
			}
		})
	}
}

func TestResolveWorktreeRoot_FallsBackFromWorktreePathToProjectRoot(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-wtr1", "wfi-wtr1", "/proj/root", "/instance/worktree")

	e := New(pool, clock.NewTest(time.Now()), nil)
	if got := e.resolveWorktreeRoot("wfi-wtr1"); got != "/instance/worktree" {
		t.Errorf("resolveWorktreeRoot = %q, want worktree_path to win", got)
	}
}

func TestResolveWorktreeRoot_UsesProjectRootWhenWorktreePathEmpty(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-wtr2", "wfi-wtr2", "/proj/root", "")

	e := New(pool, clock.NewTest(time.Now()), nil)
	if got := e.resolveWorktreeRoot("wfi-wtr2"); got != "/proj/root" {
		t.Errorf("resolveWorktreeRoot = %q, want project root_path fallback", got)
	}
}

func TestResolveWorktreeRoot_ToleratesBothEmpty(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-wtr3", "wfi-wtr3", "", "")

	e := New(pool, clock.NewTest(time.Now()), nil)
	if got := e.resolveWorktreeRoot("wfi-wtr3"); got != "" {
		t.Errorf("resolveWorktreeRoot = %q, want empty string when both unset", got)
	}
}

func TestResolveWorktreeRoot_UnknownInstanceDegradesToEmpty(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)

	e := New(pool, clock.NewTest(time.Now()), nil)
	if got := e.resolveWorktreeRoot("wfi-does-not-exist"); got != "" {
		t.Errorf("resolveWorktreeRoot(unknown) = %q, want empty (best-effort degrade)", got)
	}
}
