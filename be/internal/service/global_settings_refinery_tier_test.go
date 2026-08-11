package service

import "testing"

// TestRefineryFoldStartPctForModel_Precedence exercises the resolution
// chain: explicit tier key > explicit per-model key > built-in cheap-off
// default > generic autonomous/console key.
func TestRefineryFoldStartPctForModel_Precedence(t *testing.T) {
	t.Parallel()
	svc := setupGlobalSettingsTestEnv(t)

	// Premium model, nothing set: generic defaults (autonomous 60 / console 75).
	if got, err := svc.GetRefineryFoldStartPctForModel("opus-5", false); err != nil || got != DefaultRefineryFoldStartContextPct {
		t.Errorf("premium autonomous default = %d (%v), want %d", got, err, DefaultRefineryFoldStartContextPct)
	}
	if got, err := svc.GetRefineryFoldStartPctForModel("opus-5", true); err != nil || got != DefaultRefineryConsoleFoldStartContextPct {
		t.Errorf("premium console default = %d (%v), want %d", got, err, DefaultRefineryConsoleFoldStartContextPct)
	}

	// Cheap model, nothing set: folding disabled by the built-in default.
	if got, err := svc.GetRefineryFoldStartPctForModel("haiku", false); err != nil || got != 0 {
		t.Errorf("cheap default = %d (%v), want 0 (folding disabled)", got, err)
	}

	// Per-model key re-enables one cheap model.
	if err := svc.Set(RefineryFoldStartPctModelKeyPrefix+"haiku", "45"); err != nil {
		t.Fatalf("Set model key: %v", err)
	}
	if got, _ := svc.GetRefineryFoldStartPctForModel("haiku", false); got != 45 {
		t.Errorf("cheap with model key = %d, want 45", got)
	}

	// Tier key prevails over the per-model key.
	if err := svc.Set(RefineryFoldStartPctCheapKey, "30"); err != nil {
		t.Fatalf("Set tier key: %v", err)
	}
	if got, _ := svc.GetRefineryFoldStartPctForModel("haiku", false); got != 30 {
		t.Errorf("cheap with tier+model keys = %d, want 30 (tier prevails)", got)
	}

	// Tier key 0 disables even with a per-model key present.
	if err := svc.Set(RefineryFoldStartPctCheapKey, "0"); err != nil {
		t.Fatalf("Set tier key 0: %v", err)
	}
	if got, _ := svc.GetRefineryFoldStartPctForModel("haiku", false); got != 0 {
		t.Errorf("cheap with tier key 0 = %d, want 0", got)
	}

	// Unknown-registry id classifies by name through the same classifier.
	if got, _ := svc.GetRefineryFoldStartPctForModel("some-haiku-preview", false); got != 0 {
		t.Errorf("raw haiku-named id = %d, want 0 (cheap by name)", got)
	}
	if got, _ := svc.GetRefineryFoldStartPctForModel("", false); got != DefaultRefineryFoldStartContextPct {
		t.Errorf("empty model id = %d, want generic default %d", got, DefaultRefineryFoldStartContextPct)
	}

	// Invalid tier value falls through, not to a default of its own.
	if err := svc.Set(RefineryFoldStartPctPremiumKey, "150"); err != nil {
		t.Fatalf("Set invalid tier value: %v", err)
	}
	if got, _ := svc.GetRefineryFoldStartPctForModel("opus-5", false); got != DefaultRefineryFoldStartContextPct {
		t.Errorf("premium with invalid tier value = %d, want generic default", got)
	}
}
