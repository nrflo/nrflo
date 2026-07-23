package stepengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
)

// twoStepDef returns a two-step stepwise def: step "s1" requires a
// nonempty_text "summary" finding, step "s2" requires a
// json_array_path_change "changes" finding.
func twoStepDef() *model.AgentDefinition {
	steps := []model.StepDefinition{
		{StepID: "s1", Title: "First", Instruction: "do first thing",
			RequiredFindings: []model.RequiredFinding{{Key: "summary", Schema: model.FindingSchemaNonemptyText}}},
		{StepID: "s2", Title: "Second", Instruction: "do second thing",
			RequiredFindings: []model.RequiredFinding{{Key: "changes", Schema: model.FindingSchemaJSONArrayPathChange}}},
	}
	b, _ := json.Marshal(steps)
	s := string(b)
	return &model.AgentDefinition{PromptMode: promptModeStepwise, Steps: &s}
}

func completedJSON(t *testing.T, entries ...model.CompletedStep) string {
	t.Helper()
	b, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal completed: %v", err)
	}
	return string(b)
}

// TestCompletedEvidence_NoCursor verifies ErrNoCursor when no cursor row
// exists for (instanceID, nodeID).
func TestCompletedEvidence_NoCursor(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-ce1", "wfi-ce1", "", "")

	e := New(pool, clock.NewTest(time.Now()), nil)
	_, err := e.CompletedEvidence("wfi-ce1", "node-a")
	if err != ErrNoCursor {
		t.Fatalf("CompletedEvidence() err = %v, want ErrNoCursor", err)
	}
}

// TestCompletedEvidence_ZeroCompleted_EmptySliceNoError verifies a cursor
// with nothing completed yet returns (nil, nil).
func TestCompletedEvidence_ZeroCompleted_EmptySliceNoError(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-ce2", "wfi-ce2", "", "")
	seedSession(t, pool, "sess-ce2", "proj-ce2", "wfi-ce2", "node-a")

	e := New(pool, clock.NewTest(time.Now()), nil)
	if _, err := e.Snapshot("wfi-ce2", "node-a", twoStepDef()); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	evidence, err := e.CompletedEvidence("wfi-ce2", "node-a")
	if err != nil {
		t.Fatalf("CompletedEvidence: %v", err)
	}
	if len(evidence) != 0 {
		t.Errorf("evidence = %+v, want empty slice", evidence)
	}
}

// TestCompletedEvidence_OrderingAndSnapshotDeclaredKeys verifies evidence
// entries come back in completed order, using the snapshot-declared
// required_findings keys (never CompletedStep.EvidenceKeys — the agent's own
// self-report is deliberately ignored).
func TestCompletedEvidence_OrderingAndSnapshotDeclaredKeys(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-ce3", "wfi-ce3", "", "")
	seedSession(t, pool, "sess-ce3", "proj-ce3", "wfi-ce3", "node-a")
	seedFinding(t, pool, "wfi-ce3", "sess-ce3", "summary", "did the first thing")

	e := New(pool, clock.NewTest(time.Now()), nil)
	if _, err := e.Snapshot("wfi-ce3", "node-a", twoStepDef()); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	completed := completedJSON(t, model.CompletedStep{
		StepID: "s1", Summary: "finished step one", CompletedAt: "2026-01-01T00:00:00Z",
		// EvidenceKeys deliberately names a key NOT in required_findings — must be ignored.
		EvidenceKeys: []string{"bogus_agent_reported_key"},
	})
	if _, err := pool.Exec(
		`UPDATE agent_step_cursors SET current_index = 1, completed = ? WHERE workflow_instance_id = ? AND node_id = ?`,
		completed, "wfi-ce3", "node-a"); err != nil {
		t.Fatalf("advance cursor: %v", err)
	}

	evidence, err := e.CompletedEvidence("wfi-ce3", "node-a")
	if err != nil {
		t.Fatalf("CompletedEvidence: %v", err)
	}
	if len(evidence) != 1 {
		t.Fatalf("evidence len = %d, want 1", len(evidence))
	}
	ev := evidence[0]
	if ev.Index != 0 || ev.StepID != "s1" || ev.Title != "First" {
		t.Errorf("evidence[0] = %+v, want Index=0 StepID=s1 Title=First", ev)
	}
	if ev.Summary != "finished step one" {
		t.Errorf("Summary = %q, want %q", ev.Summary, "finished step one")
	}
	if len(ev.Findings) != 1 || ev.Findings[0].Key != "summary" {
		t.Fatalf("Findings = %+v, want exactly the snapshot-declared 'summary' key", ev.Findings)
	}
	if ev.Findings[0].Value != "did the first thing" {
		t.Errorf("Findings[0].Value = %q, want %q", ev.Findings[0].Value, "did the first thing")
	}
	for _, f := range ev.Findings {
		if f.Key == "bogus_agent_reported_key" {
			t.Error("agent-supplied EvidenceKeys leaked into structured evidence")
		}
	}
}

// TestCompletedEvidence_MissingFindingValueAbsentNoError verifies a
// snapshot-declared key with no recorded finding value renders as an absent
// (empty-value) entry rather than an error.
func TestCompletedEvidence_MissingFindingValueAbsentNoError(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-ce4", "wfi-ce4", "", "")
	seedSession(t, pool, "sess-ce4", "proj-ce4", "wfi-ce4", "node-a")
	// No "summary" finding recorded at all.

	e := New(pool, clock.NewTest(time.Now()), nil)
	if _, err := e.Snapshot("wfi-ce4", "node-a", twoStepDef()); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	completed := completedJSON(t, model.CompletedStep{StepID: "s1", CompletedAt: "2026-01-01T00:00:00Z"})
	if _, err := pool.Exec(
		`UPDATE agent_step_cursors SET current_index = 1, completed = ? WHERE workflow_instance_id = ? AND node_id = ?`,
		completed, "wfi-ce4", "node-a"); err != nil {
		t.Fatalf("advance cursor: %v", err)
	}

	evidence, err := e.CompletedEvidence("wfi-ce4", "node-a")
	if err != nil {
		t.Fatalf("CompletedEvidence: %v", err)
	}
	if len(evidence) != 1 || len(evidence[0].Findings) != 1 {
		t.Fatalf("evidence = %+v, want 1 entry with 1 declared (absent) finding", evidence)
	}
	if evidence[0].Findings[0].Key != "summary" || evidence[0].Findings[0].Value != "" {
		t.Errorf("Findings[0] = %+v, want Key=summary Value=\"\" for a missing value", evidence[0].Findings[0])
	}
}

// TestCompletedEvidence_PathSplit_RealAndBogus verifies a
// json_array_path_change value with one real file under the worktree and one
// bogus path splits into ResolvedPaths / UnresolvedPaths — never dropped,
// never fuzzy-matched.
func TestCompletedEvidence_PathSplit_RealAndBogus(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "real.go"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-ce5", "wfi-ce5", "", root)
	seedSession(t, pool, "sess-ce5", "proj-ce5", "wfi-ce5", "node-a")
	seedFinding(t, pool, "wfi-ce5", "sess-ce5", "changes", []map[string]string{
		{"path": "pkg/real.go", "change": "added"},
		{"path": "definitely/does/not/exist.go", "change": "added"},
	})

	e := New(pool, clock.NewTest(time.Now()), nil)
	if _, err := e.Snapshot("wfi-ce5", "node-a", twoStepDef()); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	completed := completedJSON(t, model.CompletedStep{StepID: "s2", CompletedAt: "2026-01-01T00:00:00Z"})
	// s2 is index 1 — advance current_index to 2 so CurrentIndex isn't relevant here.
	if _, err := pool.Exec(
		`UPDATE agent_step_cursors SET current_index = 2, completed = ? WHERE workflow_instance_id = ? AND node_id = ?`,
		completed, "wfi-ce5", "node-a"); err != nil {
		t.Fatalf("advance cursor: %v", err)
	}

	evidence, err := e.CompletedEvidence("wfi-ce5", "node-a")
	if err != nil {
		t.Fatalf("CompletedEvidence: %v", err)
	}
	if len(evidence) != 1 {
		t.Fatalf("evidence len = %d, want 1", len(evidence))
	}
	ev := evidence[0]
	if len(ev.ResolvedPaths) != 1 || ev.ResolvedPaths[0] != "pkg/real.go" {
		t.Errorf("ResolvedPaths = %v, want [pkg/real.go]", ev.ResolvedPaths)
	}
	if len(ev.UnresolvedPaths) != 1 || ev.UnresolvedPaths[0] != "definitely/does/not/exist.go" {
		t.Errorf("UnresolvedPaths = %v, want [definitely/does/not/exist.go]", ev.UnresolvedPaths)
	}
}

// TestCompletedEvidence_UnknownCompletedStepIDSkipped verifies a completed
// entry whose step_id no longer exists in the snapshot (should never happen,
// but defensively) is skipped rather than panicking.
func TestCompletedEvidence_UnknownCompletedStepIDSkipped(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-ce6", "wfi-ce6", "", "")
	seedSession(t, pool, "sess-ce6", "proj-ce6", "wfi-ce6", "node-a")

	e := New(pool, clock.NewTest(time.Now()), nil)
	if _, err := e.Snapshot("wfi-ce6", "node-a", twoStepDef()); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	completed := completedJSON(t, model.CompletedStep{StepID: "no-such-step", CompletedAt: "2026-01-01T00:00:00Z"})
	if _, err := pool.Exec(
		`UPDATE agent_step_cursors SET current_index = 1, completed = ? WHERE workflow_instance_id = ? AND node_id = ?`,
		completed, "wfi-ce6", "node-a"); err != nil {
		t.Fatalf("advance cursor: %v", err)
	}

	evidence, err := e.CompletedEvidence("wfi-ce6", "node-a")
	if err != nil {
		t.Fatalf("CompletedEvidence: %v", err)
	}
	if len(evidence) != 0 {
		t.Errorf("evidence = %+v, want empty (unknown step_id skipped)", evidence)
	}
}
