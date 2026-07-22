package service

import (
	"strings"
	"testing"
)

// TestValidateEffortAllowed_None verifies "none" behaves like any other
// enum member for list-membership: passes when listed as supported, and
// yields the "not supported by this model" message (not "invalid
// reasoning_effort") when it isn't — this is the post-change branch the
// planner flagged as a likely regression surface.
func TestValidateEffortAllowed_None(t *testing.T) {
	t.Run("supported when listed", func(t *testing.T) {
		if err := ValidateEffortAllowed("none", []string{"none", "low"}); err != nil {
			t.Errorf("ValidateEffortAllowed(none, [none low]) = %v, want nil", err)
		}
	})

	t.Run("not supported yields model-specific message, not invalid enum", func(t *testing.T) {
		err := ValidateEffortAllowed("none", []string{"low", "medium"})
		if err == nil {
			t.Fatal("ValidateEffortAllowed(none, [low medium]) succeeded, want error")
		}
		if strings.Contains(err.Error(), "invalid reasoning_effort") {
			t.Errorf("error = %v, want NOT to mention invalid reasoning_effort ('none' is a recognized enum value)", err)
		}
		if !strings.Contains(err.Error(), "not supported by this model") {
			t.Errorf("error = %v, want mention of not supported by this model", err)
		}
	})

	t.Run("empty supported list yields no-effort-selection message", func(t *testing.T) {
		err := ValidateEffortAllowed("none", nil)
		if err == nil || !strings.Contains(err.Error(), "does not support effort selection") {
			t.Errorf("error = %v, want mention of does not support effort selection", err)
		}
	})
}

// TestNormalizeSupportedEfforts_None verifies "none" is accepted as a valid
// enum entry and sorts as the weakest level (ahead of "low").
func TestNormalizeSupportedEfforts_None(t *testing.T) {
	out, err := NormalizeSupportedEfforts([]string{"medium", "none", "low", "none"})
	if err != nil {
		t.Fatalf("NormalizeSupportedEfforts: %v", err)
	}
	want := []string{"none", "low", "medium"}
	if len(out) != len(want) {
		t.Fatalf("out = %v, want %v", out, want)
	}
	for i, w := range want {
		if out[i] != w {
			t.Errorf("out[%d] = %q, want %q", i, out[i], w)
		}
	}
}

// TestNormalizeSupportedEfforts_UnknownEntry_Rejected verifies an
// unrecognized entry (not "none" and not a real level) still errors.
func TestNormalizeSupportedEfforts_UnknownEntry_Rejected(t *testing.T) {
	_, err := NormalizeSupportedEfforts([]string{"nope"})
	if err == nil {
		t.Fatal("NormalizeSupportedEfforts([nope]) succeeded, want error")
	}
	if !strings.Contains(err.Error(), "invalid supported_efforts entry") {
		t.Errorf("error = %v, want mention of invalid supported_efforts entry", err)
	}
}
