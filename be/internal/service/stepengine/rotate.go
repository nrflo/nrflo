package stepengine

// RotateInput bundles every signal ShouldRotate needs — mirrors
// spawner/context_restart_policy.go's rotateDecision: a pure input struct
// instead of process-global state, trivially unit-testable.
type RotateInput struct {
	ContextTokens   int
	ThresholdTokens int // <=0 disables the policy entirely
	RotationAllowed bool
	FinalStep       bool
}

// ShouldRotate reports whether a completed step's outcome should be upgraded
// from Next to Rotate: a threshold is configured, usage strictly exceeds it,
// the SNAPSHOT step (not the live definition) allows rotation, and the step
// is not the final one in the sequence.
func ShouldRotate(in RotateInput) bool {
	if in.ThresholdTokens <= 0 {
		return false
	}
	if in.ContextTokens <= in.ThresholdTokens {
		return false
	}
	if !in.RotationAllowed {
		return false
	}
	if in.FinalStep {
		return false
	}
	return true
}
