package stepengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
)

func stepWithFindings(rf ...model.RequiredFinding) model.StepDefinition {
	return model.StepDefinition{StepID: "s1", Title: "t", Instruction: "i", RequiredFindings: rf}
}

func TestValidateEvidence_MissingKeyNamesTheKey(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-ev1", "wfi-ev1", "", "")
	seedSession(t, pool, "sess-ev1", "proj-ev1", "wfi-ev1", "node-a")

	e := New(pool, clock.NewTest(time.Now()), nil)
	step := stepWithFindings(model.RequiredFinding{Key: "summary", Schema: model.FindingSchemaNonemptyText})

	res, err := e.ValidateEvidence(step, EvidenceContext{InstanceID: "wfi-ev1", NodeID: "node-a"})
	if err != nil {
		t.Fatalf("ValidateEvidence: %v", err)
	}
	if res.OK {
		t.Fatal("OK = true, want false for missing finding")
	}
	msg := res.RejectionMessage()
	if !strings.Contains(msg, "summary") {
		t.Errorf("RejectionMessage() = %q, want it to name the missing key %q", msg, "summary")
	}
}

func TestValidateEvidence_InvalidValueNamesKeyAndSchema(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-ev2", "wfi-ev2", "", "")
	seedSession(t, pool, "sess-ev2", "proj-ev2", "wfi-ev2", "node-a")
	seedFinding(t, pool, "wfi-ev2", "sess-ev2", "summary", "")

	e := New(pool, clock.NewTest(time.Now()), nil)
	step := stepWithFindings(model.RequiredFinding{Key: "summary", Schema: model.FindingSchemaNonemptyText})

	res, err := e.ValidateEvidence(step, EvidenceContext{InstanceID: "wfi-ev2", NodeID: "node-a"})
	if err != nil {
		t.Fatalf("ValidateEvidence: %v", err)
	}
	msg := res.RejectionMessage()
	if !strings.Contains(msg, "summary") || !strings.Contains(msg, model.FindingSchemaNonemptyText) {
		t.Errorf("RejectionMessage() = %q, want it to name both key and schema", msg)
	}
}

// TestValidateEvidence_AllProblemsReportedInOneMessage verifies every
// missing/invalid key is aggregated into a single RejectionMessage — never
// dribbled one problem per call.
func TestValidateEvidence_AllProblemsReportedInOneMessage(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-ev3", "wfi-ev3", "", "")
	seedSession(t, pool, "sess-ev3", "proj-ev3", "wfi-ev3", "node-a")
	seedFinding(t, pool, "wfi-ev3", "sess-ev3", "bad_key", "")

	e := New(pool, clock.NewTest(time.Now()), nil)
	step := stepWithFindings(
		model.RequiredFinding{Key: "missing_key", Schema: model.FindingSchemaNonemptyText},
		model.RequiredFinding{Key: "bad_key", Schema: model.FindingSchemaNonemptyText},
	)

	res, err := e.ValidateEvidence(step, EvidenceContext{InstanceID: "wfi-ev3", NodeID: "node-a"})
	if err != nil {
		t.Fatalf("ValidateEvidence: %v", err)
	}
	msg := res.RejectionMessage()
	if !strings.Contains(msg, "missing_key") || !strings.Contains(msg, "bad_key") {
		t.Errorf("RejectionMessage() = %q, want both problems in one message", msg)
	}
}

func TestValidateEvidence_UniquePathMatchNoFlag(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "foo.go"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	seedProjectAndWorkflow(t, pool, "proj-ev4", "wfi-ev4", "", "")
	seedSession(t, pool, "sess-ev4", "proj-ev4", "wfi-ev4", "node-a")
	seedFinding(t, pool, "wfi-ev4", "sess-ev4", "changes", []map[string]string{{"path": "foo.go", "change": "added"}})

	e := New(pool, clock.NewTest(time.Now()), nil)
	step := stepWithFindings(model.RequiredFinding{Key: "changes", Schema: model.FindingSchemaJSONArrayPathChange})

	res, err := e.ValidateEvidence(step, EvidenceContext{InstanceID: "wfi-ev4", NodeID: "node-a", RepoRoot: root})
	if err != nil {
		t.Fatalf("ValidateEvidence: %v", err)
	}
	if !res.OK {
		t.Errorf("OK = false, want true: %+v", res)
	}
	if len(res.Flags) != 0 {
		t.Errorf("Flags = %v, want none for a unique path match", res.Flags)
	}
}

// TestValidateEvidence_AmbiguousBasenameFlagsButStillOK verifies two files
// sharing a basename under the worktree root produce a non-fatal ambiguous
// flag while OK stays true.
func TestValidateEvidence_AmbiguousBasenameFlagsButStillOK(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	root := t.TempDir()
	for _, dir := range []string{"a", "b"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, dir, "dup.go"), []byte("x"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	seedProjectAndWorkflow(t, pool, "proj-ev5", "wfi-ev5", "", "")
	seedSession(t, pool, "sess-ev5", "proj-ev5", "wfi-ev5", "node-a")
	seedFinding(t, pool, "wfi-ev5", "sess-ev5", "changes", []map[string]string{{"path": "dup.go", "change": "added"}})

	e := New(pool, clock.NewTest(time.Now()), nil)
	step := stepWithFindings(model.RequiredFinding{Key: "changes", Schema: model.FindingSchemaJSONArrayPathChange})

	res, err := e.ValidateEvidence(step, EvidenceContext{InstanceID: "wfi-ev5", NodeID: "node-a", RepoRoot: root})
	if err != nil {
		t.Fatalf("ValidateEvidence: %v", err)
	}
	if !res.OK {
		t.Error("OK = false, want true (ambiguous path is non-fatal)")
	}
	if len(res.Flags) != 1 {
		t.Fatalf("Flags = %v, want exactly 1 ambiguous flag", res.Flags)
	}
}

// TestValidateEvidence_NonexistentPathFlagsButStillOK covers the
// files_to_create case: a referenced path that legitimately does not exist
// yet must flag, non-fatally.
func TestValidateEvidence_NonexistentPathFlagsButStillOK(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	root := t.TempDir()
	seedProjectAndWorkflow(t, pool, "proj-ev6", "wfi-ev6", "", "")
	seedSession(t, pool, "sess-ev6", "proj-ev6", "wfi-ev6", "node-a")
	seedFinding(t, pool, "wfi-ev6", "sess-ev6", "changes", []map[string]string{{"path": "not_yet_created.go", "change": "will add"}})

	e := New(pool, clock.NewTest(time.Now()), nil)
	step := stepWithFindings(model.RequiredFinding{Key: "changes", Schema: model.FindingSchemaJSONArrayPathChange})

	res, err := e.ValidateEvidence(step, EvidenceContext{InstanceID: "wfi-ev6", NodeID: "node-a", RepoRoot: root})
	if err != nil {
		t.Fatalf("ValidateEvidence: %v", err)
	}
	if !res.OK {
		t.Error("OK = false, want true (unresolved path is non-fatal)")
	}
	if len(res.Flags) != 1 {
		t.Fatalf("Flags = %v, want exactly 1 unresolved flag", res.Flags)
	}
}

func TestValidateEvidence_UnknownNodeReturnsError(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-ev7", "wfi-ev7", "", "")

	e := New(pool, clock.NewTest(time.Now()), nil)
	step := stepWithFindings(model.RequiredFinding{Key: "summary", Schema: model.FindingSchemaNonemptyText})

	_, err := e.ValidateEvidence(step, EvidenceContext{InstanceID: "wfi-ev7", NodeID: "node-nobody-ever-ran"})
	if err == nil {
		t.Error("ValidateEvidence(unknown node) error = nil, want error")
	}
}

// TestValidateEvidence_EmptyRepoRootFlagsEveryPathButStillOK verifies an
// empty RepoRoot marks every path-bearing value unresolved (never
// synthesized) while the result stays OK.
func TestValidateEvidence_EmptyRepoRootFlagsEveryPathButStillOK(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-ev8", "wfi-ev8", "", "")
	seedSession(t, pool, "sess-ev8", "proj-ev8", "wfi-ev8", "node-a")
	seedFinding(t, pool, "wfi-ev8", "sess-ev8", "changes", []map[string]string{
		{"path": "a.go", "change": "x"},
		{"path": "b.go", "change": "y"},
	})

	e := New(pool, clock.NewTest(time.Now()), nil)
	step := stepWithFindings(model.RequiredFinding{Key: "changes", Schema: model.FindingSchemaJSONArrayPathChange})

	res, err := e.ValidateEvidence(step, EvidenceContext{InstanceID: "wfi-ev8", NodeID: "node-a", RepoRoot: ""})
	if err != nil {
		t.Fatalf("ValidateEvidence: %v", err)
	}
	if !res.OK {
		t.Error("OK = false, want true")
	}
	if len(res.Flags) != 2 {
		t.Errorf("Flags = %v, want 2 (one per path) when RepoRoot is empty", res.Flags)
	}
}
