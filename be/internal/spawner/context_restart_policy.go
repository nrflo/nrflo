package spawner

import "time"

// rotateDecision bundles every signal shouldRotate needs. Split into its own
// struct (mirrors context_watcher_policy.go's selectEviction inputs) so the
// policy is a pure function over explicit values instead of process-global
// state — trivially unit-testable with a fixed clock.
type rotateDecision struct {
	Now                  time.Time
	CurrentTokens        int
	ThresholdTokens      int // <=0 disables the policy entirely
	CurrentTurn          int
	Idle                 bool
	LastPlanItemInFlight bool
	LastBoundaryTurn     int
	BoundaryWindowTurns  int // <=0 disables the boundary-recency gate
	LastRestartAt        time.Time
	MinInterval          time.Duration
	RestartsDone         int
	MaxPerSession        int // <=0 means unlimited (still logged/counted)
}

// shouldRotate applies the proactive-restart safety rails: over threshold,
// idle (never mid-tool-chain), a task boundary was stamped recently enough,
// the minimum inter-restart interval has elapsed, the per-session cap (if
// any) isn't spent, and the last plan item isn't currently in flight. Shared
// by the autonomous monitor (spawner.ProactiveRestartDecision) and console
// chat rotation, which always satisfies the boundary-recency gate trivially
// (CurrentTurn==LastBoundaryTurn==0 — the call site itself IS the boundary).
func shouldRotate(d rotateDecision) (fire bool, tokensBefore int) {
	if d.ThresholdTokens <= 0 {
		return false, 0
	}
	if d.CurrentTokens <= d.ThresholdTokens {
		return false, 0
	}
	if !d.Idle {
		return false, 0
	}
	if d.LastPlanItemInFlight {
		return false, 0
	}
	if d.BoundaryWindowTurns > 0 && d.CurrentTurn-d.LastBoundaryTurn > d.BoundaryWindowTurns {
		return false, 0
	}
	if !d.LastRestartAt.IsZero() && d.Now.Sub(d.LastRestartAt) < d.MinInterval {
		return false, 0
	}
	if d.MaxPerSession > 0 && d.RestartsDone >= d.MaxPerSession {
		return false, 0
	}
	return true, d.CurrentTokens
}
