package spawner

import (
	"os"
	"testing"

	"be/internal/clock"
)

// fakeResumeHandoff is a minimal resumeHandoff for exercising discardResume/
// transferResume generically (not codex-specific); it also implements
// costBaselineCapture so transferResume's baseline-capture branch is covered.
type fakeResumeHandoff struct {
	discarded        bool
	capturedBaseline CostSnapshot
	captured         bool
}

func (h *fakeResumeHandoff) discard() { h.discarded = true }
func (h *fakeResumeHandoff) captureCostBaseline(s CostSnapshot) {
	h.capturedBaseline = s
	h.captured = true
}

// TestDiscardResume_NilHandoff_NoOp verifies discardResume on a proc with no
// handoff never panics and leaves the field nil.
func TestDiscardResume_NilHandoff_NoOp(t *testing.T) {
	t.Parallel()
	proc := &processInfo{}
	proc.discardResume() // must not panic
	if proc.resumeHandoff != nil {
		t.Error("resumeHandoff should stay nil")
	}
}

// TestDiscardResume_NilProc_NoOp verifies calling discardResume on a nil
// *processInfo never panics (nil-safe method receiver).
func TestDiscardResume_NilProc_NoOp(t *testing.T) {
	t.Parallel()
	var proc *processInfo
	proc.discardResume() // must not panic
}

// TestDiscardResume_ArmedHandoff_RemovesDirAndNilsField verifies discardResume
// on an armed codexThreadHandoff removes its profile dir and nils the field,
// and that a second call is a safe no-op (idempotent).
func TestDiscardResume_ArmedHandoff_RemovesDirAndNilsField(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	proc := &processInfo{resumeHandoff: &codexThreadHandoff{threadID: "t1", profileDir: dir}}

	proc.discardResume()

	if proc.resumeHandoff != nil {
		t.Error("resumeHandoff not nilled after discardResume")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("profile dir still exists after discardResume: err=%v", err)
	}

	// Idempotent: a second call (now on a nil handoff) must not panic.
	proc.discardResume()
}

// TestDiscardResume_GenericHandoff_CallsDiscard verifies discardResume works
// through the resumeHandoff interface generically, not just for
// codexThreadHandoff.
func TestDiscardResume_GenericHandoff_CallsDiscard(t *testing.T) {
	t.Parallel()
	h := &fakeResumeHandoff{}
	proc := &processInfo{resumeHandoff: h}
	proc.discardResume()
	if !h.discarded {
		t.Error("discard() not called on the handoff")
	}
	if proc.resumeHandoff != nil {
		t.Error("resumeHandoff not nilled")
	}
}

// TestTransferResume_ResumeOnRelaunchTrue_MovesHandoff verifies
// resumeOnRelaunch=true moves the handoff from oldProc to newProc (nilling
// oldProc's field) without discarding its resource (dir survives).
func TestTransferResume_ResumeOnRelaunchTrue_MovesHandoff(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	handoff := &codexThreadHandoff{threadID: "t1", profileDir: dir}
	oldProc := &processInfo{sessionID: "old", resumeHandoff: handoff, resumeOnRelaunch: true}
	newProc := &processInfo{sessionID: "new"}

	transferResume(oldProc, newProc)

	if oldProc.resumeHandoff != nil {
		t.Error("oldProc.resumeHandoff not nilled after transfer")
	}
	if newProc.resumeHandoff != handoff {
		t.Error("newProc.resumeHandoff did not receive the moved handoff")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("profile dir removed despite a transfer (not a discard): %v", err)
	}
}

// TestTransferResume_ResumeOnRelaunchFalse_Discards verifies resumeOnRelaunch
// left false (the default for every relaunch reason except fail-restart)
// discards the handoff instead of moving it: dir removed, both fields nil.
func TestTransferResume_ResumeOnRelaunchFalse_Discards(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	handoff := &codexThreadHandoff{threadID: "t1", profileDir: dir}
	oldProc := &processInfo{sessionID: "old", resumeHandoff: handoff, resumeOnRelaunch: false}
	newProc := &processInfo{sessionID: "new"}

	transferResume(oldProc, newProc)

	if oldProc.resumeHandoff != nil {
		t.Error("oldProc.resumeHandoff not nilled after discard")
	}
	if newProc.resumeHandoff != nil {
		t.Error("newProc.resumeHandoff should stay nil when the handoff is discarded, not transferred")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("profile dir still exists after a discarding transferResume: err=%v", err)
	}
}

// TestTransferResume_NilProcs_NoOp verifies transferResume never panics when
// either proc is nil.
func TestTransferResume_NilProcs_NoOp(t *testing.T) {
	t.Parallel()
	proc := &processInfo{resumeHandoff: &fakeResumeHandoff{}}
	transferResume(nil, proc)
	transferResume(proc, nil)
	if proc.resumeHandoff == nil {
		t.Error("proc.resumeHandoff should be untouched when the other side is nil")
	}
}

// TestTransferResume_NoHandoff_NoOp verifies transferResume on a proc with no
// handoff never panics regardless of resumeOnRelaunch.
func TestTransferResume_NoHandoff_NoOp(t *testing.T) {
	t.Parallel()
	oldProc := &processInfo{resumeOnRelaunch: true}
	newProc := &processInfo{}
	transferResume(oldProc, newProc)
	if newProc.resumeHandoff != nil {
		t.Error("newProc.resumeHandoff should stay nil when oldProc had none")
	}
}

// TestTransferResume_CapturesBaselineOnlyWhenTransferring verifies the
// optional costBaselineCapture sub-interface is invoked (with the dying
// session's raw reported cumulative — SessionCostReported, not the attributed
// snapshot, so a second consecutive resume of the same thread cannot re-bill
// the first segment) only on the transfer path, never on discard. setUsage
// (not addUsage) is used to drive the store, since only setUsage's
// cumulative-report shape advances the reported high water that
// SessionCostReported reads.
func TestTransferResume_CapturesBaselineOnlyWhenTransferring(t *testing.T) {
	t.Cleanup(func() { globalCostStore.drop("old-transfer"); globalCostStore.drop("old-discard") })

	t.Run("transfer captures baseline", func(t *testing.T) {
		globalCostStore.register("old-transfer", "", nil, clock.Real(), nil)
		globalCostStore.setUsage("old-transfer", 111, 22, 0, 0)

		h := &fakeResumeHandoff{}
		oldProc := &processInfo{sessionID: "old-transfer", resumeHandoff: h, resumeOnRelaunch: true}
		newProc := &processInfo{}

		transferResume(oldProc, newProc)

		if !h.captured {
			t.Fatal("captureCostBaseline not called on the transfer path")
		}
		if h.capturedBaseline.InputTokens != 111 || h.capturedBaseline.OutputTokens != 22 {
			t.Errorf("captured baseline = %+v, want in:111 out:22", h.capturedBaseline)
		}
	})

	t.Run("discard never captures a baseline", func(t *testing.T) {
		globalCostStore.register("old-discard", "", nil, clock.Real(), nil)
		globalCostStore.setUsage("old-discard", 5, 5, 0, 0)

		h := &fakeResumeHandoff{}
		oldProc := &processInfo{sessionID: "old-discard", resumeHandoff: h, resumeOnRelaunch: false}
		newProc := &processInfo{}

		transferResume(oldProc, newProc)

		if h.captured {
			t.Error("captureCostBaseline called on a discarding transferResume")
		}
	})
}
