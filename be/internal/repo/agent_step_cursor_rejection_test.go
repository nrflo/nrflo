package repo

import (
	"testing"

	"be/internal/clock"
	"be/internal/model"
)

// TestAgentStepCursor_RecordRejectionIncrementsPerStep verifies
// RecordRejection increments the counter for the given step_id independently
// of other steps, and returns the new count each time.
func TestAgentStepCursor_RecordRejectionIncrementsPerStep(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-rej1", "wf-rej1")
	seedInstanceForCursor(t, pool, "proj-rej1", "wf-rej1", "wfi-rej1")

	r := NewAgentStepCursorRepo(pool, clock.Real())
	if err := r.Insert(&model.AgentStepCursor{
		WorkflowInstanceID: "wfi-rej1",
		NodeID:             "node-a",
		StepsSnapshot:      `[{"step_id":"s1"},{"step_id":"s2"}]`,
	}); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	for i, want := range []int{1, 2, 3} {
		got, err := r.RecordRejection("wfi-rej1", "node-a", "s1")
		if err != nil {
			t.Fatalf("RecordRejection #%d: %v", i, err)
		}
		if got != want {
			t.Errorf("RecordRejection #%d = %d, want %d", i, got, want)
		}
	}

	// A different step_id starts its own counter at 1, unaffected by s1's count.
	got, err := r.RecordRejection("wfi-rej1", "node-a", "s2")
	if err != nil {
		t.Fatalf("RecordRejection s2: %v", err)
	}
	if got != 1 {
		t.Errorf("RecordRejection s2 = %d, want 1 (independent counter)", got)
	}

	counts, err := r.Rejections("wfi-rej1", "node-a")
	if err != nil {
		t.Fatalf("Rejections: %v", err)
	}
	if counts["s1"] != 3 || counts["s2"] != 1 {
		t.Errorf("Rejections = %+v, want {s1:3 s2:1}", counts)
	}
}

// TestAgentStepCursor_RejectionsEmptyByDefault verifies a freshly-inserted
// cursor decodes to an empty (non-nil) map, not an error.
func TestAgentStepCursor_RejectionsEmptyByDefault(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-rej2", "wf-rej2")
	seedInstanceForCursor(t, pool, "proj-rej2", "wf-rej2", "wfi-rej2")

	r := NewAgentStepCursorRepo(pool, clock.Real())
	if err := r.Insert(&model.AgentStepCursor{
		WorkflowInstanceID: "wfi-rej2",
		NodeID:             "node-a",
		StepsSnapshot:      `[{"step_id":"s1"}]`,
	}); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	counts, err := r.Rejections("wfi-rej2", "node-a")
	if err != nil {
		t.Fatalf("Rejections: %v", err)
	}
	if len(counts) != 0 {
		t.Errorf("Rejections on fresh cursor = %+v, want empty", counts)
	}
}

// TestAgentStepCursor_RecordRejectionSurvivesAdvance verifies the rejection
// counter is untouched by Advance (a step transition never resets a prior
// step's rejection history — the counter is keyed by step_id, not
// current_index).
func TestAgentStepCursor_RecordRejectionSurvivesAdvance(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-rej3", "wf-rej3")
	seedInstanceForCursor(t, pool, "proj-rej3", "wf-rej3", "wfi-rej3")

	r := NewAgentStepCursorRepo(pool, clock.Real())
	if err := r.Insert(&model.AgentStepCursor{
		WorkflowInstanceID: "wfi-rej3",
		NodeID:             "node-a",
		StepsSnapshot:      `[{"step_id":"s1"},{"step_id":"s2"}]`,
	}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if _, err := r.RecordRejection("wfi-rej3", "node-a", "s1"); err != nil {
		t.Fatalf("RecordRejection: %v", err)
	}
	if _, err := r.RecordRejection("wfi-rej3", "node-a", "s1"); err != nil {
		t.Fatalf("RecordRejection: %v", err)
	}

	if ok, err := r.Advance("wfi-rej3", "node-a", 1, 0, `[{"step_id":"s1"}]`); err != nil || !ok {
		t.Fatalf("Advance: ok=%v err=%v", ok, err)
	}

	counts, err := r.Rejections("wfi-rej3", "node-a")
	if err != nil {
		t.Fatalf("Rejections: %v", err)
	}
	if counts["s1"] != 2 {
		t.Errorf("Rejections[s1] after Advance = %d, want 2 (unaffected)", counts["s1"])
	}
}
