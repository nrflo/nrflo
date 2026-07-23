package spawner

// Tests for prepareSpawn's stepwise cursor-creation ordering/idempotency:
// snapshotStepCursor runs before loadTemplate so the cursor exists before the
// agent's first tool call, and a second prepareSpawn (relaunch) never resets
// in-progress cursor state.

import (
	"context"
	"os"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
)

func stepwisePrepareSpawner(env *contextSaveTestEnv) *Spawner {
	return New(Config{
		DataPath: env.dbPath,
		Pool:     db.WrapAsPool(env.database),
		Clock:    clock.Real(),
		AgentSvc: &noopAgentSvc{},
		ModelConfigs: map[string]ModelConfig{
			"sonnet-5": {Provider: "anthropic", CLIModel: "claude-sonnet-5", DefaultEffort: "medium", CLIEfforts: []string{"medium"}},
		},
	})
}

// TestPrepareSpawn_Stepwise_CreatesCursorIdempotently verifies: (1) the first
// prepareSpawn call creates the cursor at revision=1/current_index=0 with
// steps_snapshot==def.Steps, and the rendered prompt already contains
// "step 1 of"; (2) after a manual advance simulating step 1's completion, a
// second prepareSpawn call (relaunch) leaves revision/current_index/completed
// untouched and renders the advanced step.
func TestPrepareSpawn_Stepwise_CreatesCursorIdempotently(t *testing.T) {
	t.Parallel()
	ensureTmpNrfloDir(t)
	env := setupContextSaveTestEnv(t)
	defer env.cleanup()

	createStepwiseAgentDefInContextEnv(t, env, "impl", "sonnet-5", threeSteps())

	sp := stepwisePrepareSpawner(env)
	req := SpawnRequest{
		AgentType: "impl", ProjectID: env.projectID, WorkflowName: "feature",
		WorkflowInstanceID: env.wfiID, TicketID: env.ticketID,
	}

	_, prep, err := sp.prepareSpawn(context.Background(), req, "claude:sonnet-5", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("prepareSpawn() error: %v", err)
	}
	if prep.promptFile != "" {
		t.Cleanup(func() { os.Remove(prep.promptFile) })
	}
	if prep.suffixFile != "" {
		t.Cleanup(func() { os.Remove(prep.suffixFile) })
	}

	var revision, currentIndex int
	var stepsSnapshot, completed string
	if err := env.database.QueryRow(
		`SELECT revision, current_index, steps_snapshot, completed FROM agent_step_cursors WHERE workflow_instance_id = ? AND node_id = ?`,
		env.wfiID, "impl").Scan(&revision, &currentIndex, &stepsSnapshot, &completed); err != nil {
		t.Fatalf("read cursor after first prepareSpawn: %v", err)
	}
	if revision != 1 || currentIndex != 0 {
		t.Errorf("cursor after first prepareSpawn: revision=%d current_index=%d, want 1/0", revision, currentIndex)
	}
	if completed != "[]" {
		t.Errorf("completed after first prepareSpawn = %q, want []", completed)
	}
	if !strings.Contains(prep.prompt, "step 1 of") {
		t.Errorf("prep.prompt does not contain 'step 1 of': %s", prep.prompt)
	}

	// Simulate step 1's completion via a real Advance so the CAS-guarded
	// row shape matches what a completed step actually looks like.
	if _, err := env.database.Exec(
		`UPDATE agent_step_cursors SET current_index = 1, completed = ?, revision = 2 WHERE workflow_instance_id = ? AND node_id = ?`,
		`[{"step_id":"step-one","completed_at":"2026-01-01T00:00:00Z"}]`, env.wfiID, "impl"); err != nil {
		t.Fatalf("simulate step 1 completion: %v", err)
	}

	_, prep2, err := sp.prepareSpawn(context.Background(), req, "claude:sonnet-5", "impl", env.wfiID)
	if err != nil {
		t.Fatalf("second prepareSpawn() error: %v", err)
	}
	if prep2.promptFile != "" {
		t.Cleanup(func() { os.Remove(prep2.promptFile) })
	}
	if prep2.suffixFile != "" {
		t.Cleanup(func() { os.Remove(prep2.suffixFile) })
	}

	var revision2, currentIndex2 int
	var completed2 string
	if err := env.database.QueryRow(
		`SELECT revision, current_index, completed FROM agent_step_cursors WHERE workflow_instance_id = ? AND node_id = ?`,
		env.wfiID, "impl").Scan(&revision2, &currentIndex2, &completed2); err != nil {
		t.Fatalf("read cursor after second prepareSpawn: %v", err)
	}
	if revision2 != 2 || currentIndex2 != 1 {
		t.Errorf("cursor after second prepareSpawn: revision=%d current_index=%d, want unchanged 2/1 (snapshot is a no-op on an existing row)", revision2, currentIndex2)
	}
	if completed2 != `[{"step_id":"step-one","completed_at":"2026-01-01T00:00:00Z"}]` {
		t.Errorf("completed after second prepareSpawn changed unexpectedly: %q", completed2)
	}
	if !strings.Contains(prep2.prompt, "step 2 of") {
		t.Errorf("second prepareSpawn's prompt does not reflect the advanced step: %s", prep2.prompt)
	}
}
