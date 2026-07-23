package stepengine

import "testing"

// TestShouldRotate_TableDriven exercises every guard in ShouldRotate in
// isolation: each case flips exactly one gate away from an otherwise-firing
// baseline decision. Mirrors spawner/context_restart_policy_test.go's style.
func TestShouldRotate_TableDriven(t *testing.T) {
	t.Parallel()

	baseline := func() RotateInput {
		return RotateInput{
			ContextTokens:   300000,
			ThresholdTokens: 250000,
			RotationAllowed: true,
			FinalStep:       false,
		}
	}

	tests := []struct {
		name     string
		mutate   func(RotateInput) RotateInput
		wantFire bool
	}{
		{
			name:     "baseline fires",
			mutate:   func(d RotateInput) RotateInput { return d },
			wantFire: true,
		},
		{
			name:     "threshold zero disables",
			mutate:   func(d RotateInput) RotateInput { d.ThresholdTokens = 0; return d },
			wantFire: false,
		},
		{
			name:     "threshold negative disables",
			mutate:   func(d RotateInput) RotateInput { d.ThresholdTokens = -1; return d },
			wantFire: false,
		},
		{
			name:     "under threshold",
			mutate:   func(d RotateInput) RotateInput { d.ContextTokens = 249999; return d },
			wantFire: false,
		},
		{
			name:     "exactly at threshold does not fire (strictly greater than)",
			mutate:   func(d RotateInput) RotateInput { d.ContextTokens = 250000; return d },
			wantFire: false,
		},
		{
			name:     "one over threshold fires",
			mutate:   func(d RotateInput) RotateInput { d.ContextTokens = 250001; return d },
			wantFire: true,
		},
		{
			name:     "rotation not allowed blocks",
			mutate:   func(d RotateInput) RotateInput { d.RotationAllowed = false; return d },
			wantFire: false,
		},
		{
			name:     "final step never rotates even over threshold",
			mutate:   func(d RotateInput) RotateInput { d.FinalStep = true; return d },
			wantFire: false,
		},
		{
			name: "final step + rotation disallowed both block",
			mutate: func(d RotateInput) RotateInput {
				d.FinalStep = true
				d.RotationAllowed = false
				return d
			},
			wantFire: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			in := tc.mutate(baseline())
			if got := ShouldRotate(in); got != tc.wantFire {
				t.Errorf("ShouldRotate(%+v) = %v, want %v", in, got, tc.wantFire)
			}
		})
	}
}
