package spawner

// Tests for appendStepwiseBlock/resolveStepwiseState via loadTemplate: the
// per-step prompt-assembly seam for prompt_mode='stepwise' agent defs
// (template_stepwise.go). Full-mode/legacy-def coverage stays byte-identical
// per template_injectable_prepend_test.go and friends — this file is
// additive, stepwise-only.

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestAppendStepwiseBlock_InitialRender covers the core "cannot see step N+1"
// guarantee: a fresh cursor at index 0 renders step 1's instruction, all 3
// step titles (outline), the guidance injectable's contract anchor exactly
// once — and never steps 2/3's instruction bodies.
func TestAppendStepwiseBlock_InitialRender(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "SW-" + uuid.New().String()[:6]
	wfiID := env.initWorkflow(t, ticketID)
	createStepwiseAgentDef(t, env, "analyzer", threeSteps())

	sp := env.newSpawner()
	def := sp.loadAgentDefinition("analyzer", env.project, "test")
	sp.snapshotStepCursor(context.Background(), def, wfiID, "analyzer")

	result, _, _, err := sp.loadTemplate("analyzer", ticketID, env.project, "p", "c", "test",
		"claude:sonnet-5", "analyzer", wfiID, nil, 0)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}

	if !strings.Contains(result, "step 1 of 3") {
		t.Errorf("expected 'step 1 of 3', got: %s", result)
	}
	for _, title := range []string{"Title One", "Title Two", "Title Three"} {
		if !strings.Contains(result, title) {
			t.Errorf("expected outline title %q present, got: %s", title, result)
		}
	}
	if !strings.Contains(result, "Instruction body one.") {
		t.Error("expected step 1's instruction present")
	}
	if strings.Contains(result, "Instruction body two.") || strings.Contains(result, "Instruction body three.") {
		t.Error("future steps' instructions must never be rendered")
	}
	if got := strings.Count(result, "## Stepwise Mode"); got != 1 {
		t.Errorf("guidance body ('## Stepwise Mode') count = %d, want exactly 1", got)
	}
	if !strings.Contains(result, "complete_step") {
		t.Error("expected the complete_step contract anchor present")
	}
}

// TestAppendStepwiseBlock_MidSequenceRender covers a cursor manually advanced
// to current_index=1: step 2 of 3, step 1 marked completed in the outline,
// step 2's instruction present, step 3's absent.
func TestAppendStepwiseBlock_MidSequenceRender(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "SW-" + uuid.New().String()[:6]
	wfiID := env.initWorkflow(t, ticketID)
	createStepwiseAgentDef(t, env, "analyzer", threeSteps())

	sp := env.newSpawner()
	def := sp.loadAgentDefinition("analyzer", env.project, "test")
	sp.snapshotStepCursor(context.Background(), def, wfiID, "analyzer")

	if _, err := env.pool.Exec(
		`UPDATE agent_step_cursors SET current_index = 1, completed = ? WHERE workflow_instance_id = ? AND node_id = ?`,
		`[{"step_id":"step-one","completed_at":"2026-01-01T00:00:00Z"}]`, wfiID, "analyzer"); err != nil {
		t.Fatalf("advance cursor: %v", err)
	}

	result, _, _, err := sp.loadTemplate("analyzer", ticketID, env.project, "p", "c", "test",
		"claude:sonnet-5", "analyzer", wfiID, nil, 0)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}

	if !strings.Contains(result, "step 2 of 3") {
		t.Errorf("expected 'step 2 of 3', got: %s", result)
	}
	if !strings.Contains(result, "[completed] Title One") {
		t.Errorf("expected step 1 marked completed in outline, got: %s", result)
	}
	if !strings.Contains(result, "Instruction body two.") {
		t.Error("expected step 2's instruction present")
	}
	if strings.Contains(result, "Instruction body three.") {
		t.Error("step 3's instruction must not be rendered")
	}
}

// TestAppendStepwiseBlock_SnapshotImmutability verifies a mid-run edit to
// agent_definitions.steps has no effect on an already-snapshotted cursor's
// rendered prompt — the snapshot, not the live def, is authoritative.
func TestAppendStepwiseBlock_SnapshotImmutability(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "SW-" + uuid.New().String()[:6]
	wfiID := env.initWorkflow(t, ticketID)
	createStepwiseAgentDef(t, env, "analyzer", threeSteps())

	sp := env.newSpawner()
	def := sp.loadAgentDefinition("analyzer", env.project, "test")
	sp.snapshotStepCursor(context.Background(), def, wfiID, "analyzer")

	// Simulate a live def edit after the snapshot was taken.
	if _, err := env.pool.Exec(
		`UPDATE agent_definitions SET steps = ? WHERE project_id = ? AND workflow_id = 'test' AND id = 'analyzer'`,
		`[{"step_id":"mutated","title":"Mutated Title","instruction":"Mutated instruction"}]`, env.project); err != nil {
		t.Fatalf("mutate live def steps: %v", err)
	}

	result, _, _, err := sp.loadTemplate("analyzer", ticketID, env.project, "p", "c", "test",
		"claude:sonnet-5", "analyzer", wfiID, nil, 0)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}

	if strings.Contains(result, "Mutated Title") || strings.Contains(result, "Mutated instruction") {
		t.Error("rendered prompt reflects the live def edit — snapshot immutability violated")
	}
	if !strings.Contains(result, "Title One") || !strings.Contains(result, "Instruction body one.") {
		t.Errorf("expected original snapshotted step content, got: %s", result)
	}
}

// TestAppendStepwiseBlock_NoCursorFallback_PreviewReadsLiveDef verifies
// Preview (nodeID=="") renders the outline + step 1 straight from the live
// def.Steps when no cursor exists — the only path allowed to do so.
func TestAppendStepwiseBlock_NoCursorFallback_PreviewReadsLiveDef(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	createStepwiseAgentDef(t, env, "analyzer", threeSteps())

	sp := env.newSpawner()
	result, err := sp.Preview("analyzer", "PREVIEW-TICKET", env.project, "test")
	if err != nil {
		t.Fatalf("Preview failed: %v", err)
	}

	if !strings.Contains(result, "step 1 of 3") {
		t.Errorf("expected 'step 1 of 3' from def-steps fallback, got: %s", result)
	}
	if !strings.Contains(result, "Instruction body one.") {
		t.Error("expected step 1's instruction from def-steps fallback")
	}
}

// TestAppendStepwiseBlock_MissingInjectable_DegradesGracefully verifies a
// missing stepwise-guidance row degrades to "" (renderInjectable's warn-and-
// empty path) without breaking the outline/instruction render.
func TestAppendStepwiseBlock_MissingInjectable_DegradesGracefully(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "SW-" + uuid.New().String()[:6]
	wfiID := env.initWorkflow(t, ticketID)
	createStepwiseAgentDef(t, env, "analyzer", threeSteps())

	if _, err := env.pool.Exec(`DELETE FROM default_templates WHERE id = 'stepwise-guidance'`); err != nil {
		t.Fatalf("delete stepwise-guidance injectable: %v", err)
	}

	sp := env.newSpawner()
	def := sp.loadAgentDefinition("analyzer", env.project, "test")
	sp.snapshotStepCursor(context.Background(), def, wfiID, "analyzer")

	result, _, _, err := sp.loadTemplate("analyzer", ticketID, env.project, "p", "c", "test",
		"claude:sonnet-5", "analyzer", wfiID, nil, 0)
	if err != nil {
		t.Fatalf("loadTemplate should not fail with missing injectable: %v", err)
	}
	if strings.Contains(result, "## Stepwise Mode") {
		t.Error("guidance body should be absent when the injectable row is missing")
	}
	if !strings.Contains(result, "step 1 of 3") || !strings.Contains(result, "Instruction body one.") {
		t.Errorf("outline + current step instruction must still render, got: %s", result)
	}
}

// TestAppendStepwiseBlock_FullModeNoOp verifies a full-mode def's rendered
// prompt is unaffected by the stepwise seam and prepareSpawn's cursor
// snapshot step creates no agent_step_cursors row for it.
func TestAppendStepwiseBlock_FullModeNoOp(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	ticketID := "SW-" + uuid.New().String()[:6]
	wfiID := env.initWorkflow(t, ticketID)
	createAgentDef(t, env, "analyzer", "Main prompt body")

	sp := env.newSpawner()
	def := sp.loadAgentDefinition("analyzer", env.project, "test")
	sp.snapshotStepCursor(context.Background(), def, wfiID, "analyzer")

	var count int
	if err := env.pool.QueryRow(
		`SELECT COUNT(*) FROM agent_step_cursors WHERE workflow_instance_id = ? AND node_id = ?`,
		wfiID, "analyzer").Scan(&count); err != nil {
		t.Fatalf("count cursors: %v", err)
	}
	if count != 0 {
		t.Errorf("agent_step_cursors row count for full-mode def = %d, want 0", count)
	}

	result, _, _, err := sp.loadTemplate("analyzer", ticketID, env.project, "p", "c", "test",
		"claude:sonnet-5", "analyzer", wfiID, nil, 0)
	if err != nil {
		t.Fatalf("loadTemplate failed: %v", err)
	}
	if result != "Main prompt body" {
		t.Errorf("full-mode result = %q, want byte-identical 'Main prompt body'", result)
	}
}
