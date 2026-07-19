package spawner

import (
	"testing"
	"time"

	"be/internal/model"
)

// TestShouldRotate_TableDriven exercises every safety rail in shouldRotate in
// isolation: each case flips exactly one gate away from an otherwise-firing
// baseline decision.
func TestShouldRotate_TableDriven(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	baseline := func() rotateDecision {
		return rotateDecision{
			Now:                  now,
			CurrentTokens:        300000,
			ThresholdTokens:      250000,
			CurrentTurn:          10,
			Idle:                 true,
			LastPlanItemInFlight: false,
			LastBoundaryTurn:     8,
			BoundaryWindowTurns:  10,
			LastRestartAt:        time.Time{},
			MinInterval:          10 * time.Minute,
			RestartsDone:         0,
			MaxPerSession:        0,
		}
	}

	tests := []struct {
		name       string
		mutate     func(d rotateDecision) rotateDecision
		wantFire   bool
		wantTokens int
	}{
		{
			name:       "baseline fires",
			mutate:     func(d rotateDecision) rotateDecision { return d },
			wantFire:   true,
			wantTokens: 300000,
		},
		{
			name:     "threshold disabled (<=0)",
			mutate:   func(d rotateDecision) rotateDecision { d.ThresholdTokens = 0; return d },
			wantFire: false,
		},
		{
			name:     "threshold negative disabled",
			mutate:   func(d rotateDecision) rotateDecision { d.ThresholdTokens = -1; return d },
			wantFire: false,
		},
		{
			name:     "under threshold",
			mutate:   func(d rotateDecision) rotateDecision { d.CurrentTokens = 249999; return d },
			wantFire: false,
		},
		{
			name:     "exactly at threshold does not fire",
			mutate:   func(d rotateDecision) rotateDecision { d.CurrentTokens = 250000; return d },
			wantFire: false,
		},
		{
			name:     "not idle defers",
			mutate:   func(d rotateDecision) rotateDecision { d.Idle = false; return d },
			wantFire: false,
		},
		{
			name:     "last plan item in flight skips",
			mutate:   func(d rotateDecision) rotateDecision { d.LastPlanItemInFlight = true; return d },
			wantFire: false,
		},
		{
			name: "boundary window exceeded (stale finding) skips",
			mutate: func(d rotateDecision) rotateDecision {
				d.CurrentTurn = 25 // 25-8=17 > window(10)
				return d
			},
			wantFire: false,
		},
		{
			name: "boundary window disabled (<=0) never gates",
			mutate: func(d rotateDecision) rotateDecision {
				d.CurrentTurn = 1000
				d.BoundaryWindowTurns = 0
				return d
			},
			wantFire:   true,
			wantTokens: 300000,
		},
		{
			name: "min interval not yet elapsed blocks",
			mutate: func(d rotateDecision) rotateDecision {
				d.LastRestartAt = now.Add(-5 * time.Minute)
				return d
			},
			wantFire: false,
		},
		{
			name: "min interval elapsed allows",
			mutate: func(d rotateDecision) rotateDecision {
				d.LastRestartAt = now.Add(-11 * time.Minute)
				return d
			},
			wantFire:   true,
			wantTokens: 300000,
		},
		{
			name: "max per session reached blocks",
			mutate: func(d rotateDecision) rotateDecision {
				d.MaxPerSession = 2
				d.RestartsDone = 2
				return d
			},
			wantFire: false,
		},
		{
			name: "max per session unlimited (<=0) never gates",
			mutate: func(d rotateDecision) rotateDecision {
				d.MaxPerSession = 0
				d.RestartsDone = 1000
				return d
			},
			wantFire:   true,
			wantTokens: 300000,
		},
		{
			name: "max per session under cap allows",
			mutate: func(d rotateDecision) rotateDecision {
				d.MaxPerSession = 2
				d.RestartsDone = 1
				return d
			},
			wantFire:   true,
			wantTokens: 300000,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := tc.mutate(baseline())
			fire, tokensBefore := shouldRotate(d)
			if fire != tc.wantFire {
				t.Errorf("shouldRotate() fire = %v, want %v (decision=%+v)", fire, tc.wantFire, d)
			}
			if fire && tokensBefore != tc.wantTokens {
				t.Errorf("shouldRotate() tokensBefore = %d, want %d", tokensBefore, tc.wantTokens)
			}
			if !fire && tokensBefore != 0 {
				t.Errorf("shouldRotate() tokensBefore = %d, want 0 when not firing", tokensBefore)
			}
		})
	}
}

func TestResolveProactiveRestartThreshold_NilDefFallsThroughToDefault(t *testing.T) {
	t.Parallel()
	got := resolveProactiveRestartThreshold(nil, 250000)
	if got != 250000 {
		t.Errorf("resolveProactiveRestartThreshold(nil, 250000) = %d, want 250000", got)
	}
}

func TestResolveProactiveRestartThreshold_NilColumnFallsThroughToDefault(t *testing.T) {
	t.Parallel()
	def := &model.AgentDefinition{ProactiveRestartThresholdTokens: nil}
	got := resolveProactiveRestartThreshold(def, 250000)
	if got != 250000 {
		t.Errorf("resolveProactiveRestartThreshold(nil column, 250000) = %d, want 250000", got)
	}
}

func TestResolveProactiveRestartThreshold_ZeroOverrideDisables(t *testing.T) {
	t.Parallel()
	zero := 0
	def := &model.AgentDefinition{ProactiveRestartThresholdTokens: &zero}
	got := resolveProactiveRestartThreshold(def, 250000)
	if got != 0 {
		t.Errorf("resolveProactiveRestartThreshold(0 override, 250000) = %d, want 0 (explicit disable wins)", got)
	}
}

func TestResolveProactiveRestartThreshold_PositiveOverrideWins(t *testing.T) {
	t.Parallel()
	custom := 90000
	def := &model.AgentDefinition{ProactiveRestartThresholdTokens: &custom}
	got := resolveProactiveRestartThreshold(def, 250000)
	if got != 90000 {
		t.Errorf("resolveProactiveRestartThreshold(90000 override, 250000) = %d, want 90000", got)
	}
}
