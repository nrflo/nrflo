package stepengine

import (
	"testing"
	"time"

	"be/internal/clock"
)

func TestRejection_CountsTowardEvidenceCap(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		rej  *Rejection
		want bool
	}{
		{"nil rejection", nil, false},
		{"missing_evidence counts", &Rejection{Reason: "missing_evidence"}, true},
		{"invalid_evidence counts", &Rejection{Reason: "invalid_evidence"}, true},
		{"check_failed counts", &Rejection{Reason: "check_failed"}, true},
		{"stale_revision does not count", &Rejection{Reason: "stale_revision"}, false},
		{"step_mismatch does not count", &Rejection{Reason: "step_mismatch"}, false},
		{"unknown reason does not count", &Rejection{Reason: "something_else"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.rej.CountsTowardEvidenceCap(); got != tc.want {
				t.Errorf("CountsTowardEvidenceCap() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEngine_RecordRejectionAndRejectionCount(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-rejeng", "wfi-rejeng", "", "")
	e := New(pool, clock.NewTest(time.Now()), nil)
	if _, err := e.Snapshot("wfi-rejeng", "node-a", stepwiseDef("def-rejeng", twoStepJSON)); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	seedSession(t, pool, "sess-rejeng", "proj-rejeng", "wfi-rejeng", "node-a")

	count, err := e.RejectionCount("wfi-rejeng", "node-a", "s1")
	if err != nil {
		t.Fatalf("RejectionCount (none recorded yet): %v", err)
	}
	if count != 0 {
		t.Errorf("RejectionCount = %d, want 0 before any RecordRejection", count)
	}

	for i, want := range []int{1, 2} {
		got, err := e.RecordRejection("wfi-rejeng", "node-a", "s1")
		if err != nil {
			t.Fatalf("RecordRejection #%d: %v", i, err)
		}
		if got != want {
			t.Errorf("RecordRejection #%d = %d, want %d", i, got, want)
		}
	}

	count, err = e.RejectionCount("wfi-rejeng", "node-a", "s1")
	if err != nil {
		t.Fatalf("RejectionCount: %v", err)
	}
	if count != 2 {
		t.Errorf("RejectionCount = %d, want 2", count)
	}

	// A step never rejected reports 0, not an error.
	count, err = e.RejectionCount("wfi-rejeng", "node-a", "s2")
	if err != nil {
		t.Fatalf("RejectionCount(s2): %v", err)
	}
	if count != 0 {
		t.Errorf("RejectionCount(s2) = %d, want 0", count)
	}
}

// TestState_DecodesRejectionsMap verifies State() surfaces the rejections
// column decoded, so P6's read model can render it without decoding raw JSON
// itself.
func TestState_DecodesRejectionsMap(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	seedProjectAndWorkflow(t, pool, "proj-rejstate", "wfi-rejstate", "", "")
	e := New(pool, clock.NewTest(time.Now()), nil)
	if _, err := e.Snapshot("wfi-rejstate", "node-a", stepwiseDef("def-rejstate", twoStepJSON)); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	seedSession(t, pool, "sess-rejstate", "proj-rejstate", "wfi-rejstate", "node-a")

	state, err := e.State("wfi-rejstate", "node-a")
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if len(state.Rejections) != 0 {
		t.Errorf("State.Rejections on fresh cursor = %+v, want empty", state.Rejections)
	}

	if _, err := e.RecordRejection("wfi-rejstate", "node-a", "s1"); err != nil {
		t.Fatalf("RecordRejection: %v", err)
	}

	state, err = e.State("wfi-rejstate", "node-a")
	if err != nil {
		t.Fatalf("State after RecordRejection: %v", err)
	}
	if state.Rejections["s1"] != 1 {
		t.Errorf("State.Rejections[s1] = %d, want 1", state.Rejections["s1"])
	}
}
