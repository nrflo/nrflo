package spawner

// resumeHandoff is opaque native-session state a backend hands to its
// successor across a relaunch (e.g. the codex app-server's per-session
// CODEX_HOME dir + thread id). Relaunch sites move it via transferResume
// without inspecting its contents; discard() releases whatever resource it
// owns (best-effort, safe to call multiple times is NOT guaranteed — callers
// must nil the field after calling it, which discardResume/transferResume do).
type resumeHandoff interface {
	discard()
}

// costBaselineCapture is an optional resumeHandoff sub-interface for
// backends whose native session reports cumulative (not per-turn delta)
// usage, so the dying session's raw reported cumulative must ride along on
// the handoff to seed the resumed session's reported high water
// (SeedSessionCostReported) — captured via SessionCostReported, not
// SessionCost, so a second consecutive resume of the same thread cannot
// re-bill the first segment. Asserted at the call site in transferResume; a
// handoff without it is simply moved without a captured baseline.
type costBaselineCapture interface {
	captureCostBaseline(snap CostSnapshot)
}

// interactiveHandoff is an optional ExecutionBackend sub-interface for
// backends that can hand extra InteractiveSpawnOptions fields (e.g. the live
// CODEX_HOME dir) to a take-control PTY resume launch. Asserted at the call
// site exactly like the established PostStarter pattern
// (cli_adapter.go:200) — never added to the base ExecutionBackend interface,
// never a name-check on the backend.
type interactiveHandoff interface {
	TakeControlExtras() (InteractiveExtras, bool)
}

// discardResume releases proc's resume handoff, if any, and nils the field.
// Nil-safe on both the proc and the handoff. Call from every terminal path
// that does not transfer the handoff onward (finalizePhase,
// cancelRunningProcs, a non-transferring relaunchWithBookkeeping) so a
// resume-capable backend's temp CODEX_HOME dir is never leaked.
func (p *processInfo) discardResume() {
	if p == nil || p.resumeHandoff == nil {
		return
	}
	p.resumeHandoff.discard()
	p.resumeHandoff = nil
}

// transferResume moves oldProc's resume handoff to newProc when oldProc opted
// into a native resume (resumeOnRelaunch), otherwise discards it. Called from
// relaunchForContinuation after the rest of the continuation-tracking fields
// are carried over.
func transferResume(oldProc, newProc *processInfo) {
	if oldProc == nil || newProc == nil {
		return
	}
	if oldProc.resumeOnRelaunch && oldProc.resumeHandoff != nil {
		if capturer, ok := oldProc.resumeHandoff.(costBaselineCapture); ok {
			if snap, ok := SessionCostReported(oldProc.sessionID); ok {
				capturer.captureCostBaseline(snap)
			}
		}
		newProc.resumeHandoff = oldProc.resumeHandoff
		oldProc.resumeHandoff = nil
		return
	}
	oldProc.discardResume()
}
