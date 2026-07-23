package stepengine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"
)

func pathChangeJSON(t *testing.T, paths ...string) json.RawMessage {
	t.Helper()
	arr := make([]map[string]string, len(paths))
	for i, p := range paths {
		arr[i] = map[string]string{"path": p, "change": "did something"}
	}
	b, err := json.Marshal(arr)
	if err != nil {
		t.Fatalf("marshal path-change array: %v", err)
	}
	return b
}

func TestCheckPathOverlap(t *testing.T) {
	t.Parallel()
	rule := &model.PathOverlap{Left: []string{"be_a", "be_b"}, Right: []string{"fe_a", "fe_b"}}

	cases := []struct {
		name     string
		findings map[string]json.RawMessage
		rule     *model.PathOverlap
		want     []string
	}{
		{
			name: "nil rule always returns nil",
			findings: map[string]json.RawMessage{
				"be_a": pathChangeJSON(t, "x.go"),
			},
			rule: nil,
			want: nil,
		},
		{
			name: "disjoint paths, no overlap",
			findings: map[string]json.RawMessage{
				"be_a": pathChangeJSON(t, "be/x.go"),
				"fe_a": pathChangeJSON(t, "ui/x.tsx"),
			},
			rule: rule,
			want: nil,
		},
		{
			name: "one shared path is reported",
			findings: map[string]json.RawMessage{
				"be_a": pathChangeJSON(t, "shared/types.go"),
				"fe_a": pathChangeJSON(t, "shared/types.go"),
			},
			rule: rule,
			want: []string{"shared/types.go"},
		},
		{
			name: "shared path listed only on one side of the rule is not an overlap",
			findings: map[string]json.RawMessage{
				"be_a": pathChangeJSON(t, "shared/types.go"),
			},
			rule: rule,
			want: nil,
		},
		{
			name: "missing key on either side contributes no paths",
			findings: map[string]json.RawMessage{
				"be_a": pathChangeJSON(t, "x.go"),
			},
			rule: rule,
			want: nil,
		},
		{
			name: "unparsable value contributes no paths, no panic",
			findings: map[string]json.RawMessage{
				"be_a": json.RawMessage(`"not an array"`),
				"fe_a": pathChangeJSON(t, "x.go"),
			},
			rule: rule,
			want: nil,
		},
		{
			name: "paths normalize before comparing (./x vs x)",
			findings: map[string]json.RawMessage{
				"be_a": pathChangeJSON(t, "./shared/types.go"),
				"fe_a": pathChangeJSON(t, "shared/types.go"),
			},
			rule: rule,
			want: []string{"shared/types.go"},
		},
		{
			name: "multiple offenders returned sorted and deduped",
			findings: map[string]json.RawMessage{
				"be_a": pathChangeJSON(t, "b.go", "a.go"),
				"be_b": pathChangeJSON(t, "a.go"),
				"fe_a": pathChangeJSON(t, "a.go", "b.go"),
			},
			rule: rule,
			want: []string{"a.go", "b.go"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// root "" leaves every path unresolved, so comparison is literal.
			got := checkPathOverlap(tc.findings, tc.rule, "")
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("checkPathOverlap() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestCheckPathOverlap_ResolvesAcrossRepresentations verifies the gate
// canonicalizes paths against the worktree so a bare basename and its unique
// full-path form collapse to the same file — the duplicate-ownership block
// must fire even when BE and FE express the same file differently.
func TestCheckPathOverlap_ResolvesAcrossRepresentations(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "config.go"), []byte("package sub\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	rule := &model.PathOverlap{Left: []string{"be_files"}, Right: []string{"fe_files"}}
	findings := map[string]json.RawMessage{
		"be_files": pathChangeJSON(t, "config.go"),     // bare basename
		"fe_files": pathChangeJSON(t, "sub/config.go"), // full path, same file
	}
	got := checkPathOverlap(findings, rule, root)
	want := []string{"sub/config.go"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("checkPathOverlap() = %v, want %v (basename and full path must resolve to the same file)", got, want)
	}
}

// crossCheckStepJSON mirrors the pilot's step-4 shape: a single required
// finding plus a path_overlap gate over be_*/fe_* file-list keys.
const crossCheckStepJSON = `[
	{"step_id":"cross-check","title":"Cross-check","instruction":"check","required_findings":[{"key":"plan_cross_check","schema":"nonempty_text"}],"rotation_allowed":false,"path_overlap":{"left":["be_files_to_modify"],"right":["fe_files_to_modify"]}}
]`

// TestAdvance_PathOverlapRejectsWithoutMutating verifies an Advance whose
// step declares a path_overlap gate is rejected with reason "path_overlap"
// when both sides claim the same file, that the rejection message lists the
// offending path, that the cursor is left untouched, and that the reason
// counts toward the evidence cap.
func TestAdvance_PathOverlapRejectsWithoutMutating(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-overlap", "wfi-overlap", "", "")
	e := New(pool, clock.NewTest(time.Now()), nil)
	if _, err := e.Snapshot("wfi-overlap", "node-a", stepwiseDef("def-overlap", crossCheckStepJSON)); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	seedSession(t, pool, "sess-overlap", "proj-overlap", "wfi-overlap", "node-a")
	seedFinding(t, pool, "wfi-overlap", "sess-overlap", "plan_cross_check", "reviewed both sides")
	seedFinding(t, pool, "wfi-overlap", "sess-overlap", "be_files_to_modify", []map[string]string{{"path": "shared/types.go", "change": "add field"}})
	seedFinding(t, pool, "wfi-overlap", "sess-overlap", "fe_files_to_modify", []map[string]string{{"path": "shared/types.go", "change": "consume field"}})

	out, err := e.Advance(context.Background(), "wfi-overlap", "node-a", "cross-check", 1, Evidence{SessionID: "sess-overlap", Summary: "s", FindingKeys: []string{"plan_cross_check"}})
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if out.Kind != OutcomeRejected {
		t.Fatalf("Kind = %v, want OutcomeRejected", out.Kind)
	}
	if out.Rejection == nil {
		t.Fatal("Rejection = nil, want non-nil")
	}
	if out.Rejection.Reason != "path_overlap" {
		t.Errorf("Rejection.Reason = %q, want path_overlap", out.Rejection.Reason)
	}
	if !strings.Contains(out.Rejection.Message, "shared/types.go") {
		t.Errorf("Rejection.Message = %q, want it to list the offending path", out.Rejection.Message)
	}
	if !out.Rejection.CountsTowardEvidenceCap() {
		t.Error("CountsTowardEvidenceCap() = false, want true for path_overlap")
	}

	state, err := e.State("wfi-overlap", "node-a")
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if state.Revision != 1 || state.CurrentIndex != 0 || len(state.Completed) != 0 {
		t.Errorf("cursor mutated by path_overlap Advance: revision=%d index=%d completed=%v", state.Revision, state.CurrentIndex, state.Completed)
	}
}

// TestAdvance_SharedFileOnOneSideOnlyAdvances verifies a file listed only
// under the backend key (not duplicated on the frontend side) is not an
// overlap and the step advances cleanly.
func TestAdvance_SharedFileOnOneSideOnlyAdvances(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-nooverlap", "wfi-nooverlap", "", "")
	e := New(pool, clock.NewTest(time.Now()), nil)
	if _, err := e.Snapshot("wfi-nooverlap", "node-a", stepwiseDef("def-nooverlap", crossCheckStepJSON)); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	seedSession(t, pool, "sess-nooverlap", "proj-nooverlap", "wfi-nooverlap", "node-a")
	seedFinding(t, pool, "wfi-nooverlap", "sess-nooverlap", "plan_cross_check", "shared file assigned to backend only")
	seedFinding(t, pool, "wfi-nooverlap", "sess-nooverlap", "be_files_to_modify", []map[string]string{{"path": "shared/types.go", "change": "add field"}})
	// No fe_files_to_modify finding at all — the shared file is BE-exclusive.

	out, err := e.Advance(context.Background(), "wfi-nooverlap", "node-a", "cross-check", 1, Evidence{SessionID: "sess-nooverlap", Summary: "s", FindingKeys: []string{"plan_cross_check"}})
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if out.Kind != OutcomeDone {
		t.Fatalf("Kind = %v, want OutcomeDone (single-step def, no overlap)", out.Kind)
	}
}
