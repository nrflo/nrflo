package console

import (
	"errors"
	"testing"
)

func TestProfileByName_Empty_ReturnsZeroValueProfile(t *testing.T) {
	t.Parallel()
	p, err := ProfileByName("")
	if err != nil {
		t.Fatalf("ProfileByName(\"\"): %v", err)
	}
	if p.Name != "" || p.DefaultEngine != "" || p.ContextBudgetTokens != 0 || p.RefineryDefault || p.NativeToolPolicy != "" || p.Catalogue != nil {
		t.Errorf("ProfileByName(\"\") = %+v, want zero-value Profile", p)
	}
}

func TestProfileByName_Unknown_ReturnsErrUnknownProfile(t *testing.T) {
	t.Parallel()
	_, err := ProfileByName("no-such-profile")
	if !errors.Is(err, ErrUnknownProfile) {
		t.Errorf("ProfileByName(unknown) error = %v, want ErrUnknownProfile", err)
	}
}

func TestProfileByName_T0BareRemoved(t *testing.T) {
	t.Parallel()
	_, err := ProfileByName("t0-bare")
	if !errors.Is(err, ErrUnknownProfile) {
		t.Errorf("ProfileByName(t0-bare) error = %v, want ErrUnknownProfile", err)
	}
}

// TestProfileByName_T0Decider_Defaults locks in every defaulted field the
// ticket specifies: claude/opus-5/high, 30k budget, refinery on,
// tier-t0-decider template, native policy none, and the restricted catalogue.
func TestProfileByName_T0Decider_Defaults(t *testing.T) {
	t.Parallel()
	p, err := ProfileByName("t0-decider")
	if err != nil {
		t.Fatalf("ProfileByName(t0-decider): %v", err)
	}
	if p.DefaultEngine != "claude" {
		t.Errorf("DefaultEngine = %q, want claude", p.DefaultEngine)
	}
	if p.DefaultModelID != "opus-5" {
		t.Errorf("DefaultModelID = %q, want opus-5", p.DefaultModelID)
	}
	if p.DefaultEffort != "high" {
		t.Errorf("DefaultEffort = %q, want high", p.DefaultEffort)
	}
	if p.ContextBudgetTokens != 30000 {
		t.Errorf("ContextBudgetTokens = %d, want 30000", p.ContextBudgetTokens)
	}
	if !p.RefineryDefault {
		t.Error("RefineryDefault = false, want true")
	}
	if p.SystemTemplateID != "tier-t0-decider" {
		t.Errorf("SystemTemplateID = %q, want tier-t0-decider", p.SystemTemplateID)
	}
	if p.NativeToolPolicy != NativeToolPolicyNone {
		t.Errorf("NativeToolPolicy = %q, want %q", p.NativeToolPolicy, NativeToolPolicyNone)
	}
	if !p.SiblingFlows {
		t.Error("SiblingFlows = false, want true")
	}
	if len(p.Catalogue) == 0 {
		t.Fatal("Catalogue is empty, want the restricted T0 allowlist")
	}
	for _, banned := range []string{"read_file", "edit_file", "write_file", "bash", "glob", "grep", "web_fetch", "web_search"} {
		for _, name := range p.Catalogue {
			if name == banned {
				t.Errorf("t0-decider Catalogue contains banned tool %q", banned)
			}
		}
	}
	for _, want := range []string{"delegate", "get_delegation", "dynamic_workflow", "consult"} {
		found := false
		for _, name := range p.Catalogue {
			if name == want {
				found = true
			}
		}
		if !found {
			t.Errorf("t0-decider Catalogue missing expected tool %q", want)
		}
	}
}

// TestProfileByName_T0Decider_NotA1MModel guards against accidentally
// wiring a `[1m]`-suffixed row (a distinct, separately-priced context
// variant) as the T0 decider default.
func TestProfileByName_T0Decider_NotA1MModel(t *testing.T) {
	t.Parallel()
	p, err := ProfileByName("t0-decider")
	if err != nil {
		t.Fatalf("ProfileByName(t0-decider): %v", err)
	}
	if p.DefaultModelID == "opus-5-1m" || p.DefaultModelID != "opus-5" {
		t.Errorf("DefaultModelID = %q, want the non-1m opus-5 row", p.DefaultModelID)
	}
}

// TestProfileByName_T0Hands_Defaults verifies the full-tools companion
// profile: sonnet-5/medium, full tools (nil catalogue), 80k budget, refinery on
// (per-profile, not the global refinery_enabled flag).
func TestProfileByName_T0Hands_Defaults(t *testing.T) {
	t.Parallel()
	p, err := ProfileByName("t0-hands")
	if err != nil {
		t.Fatalf("ProfileByName(t0-hands): %v", err)
	}
	if p.DefaultEngine != "claude" {
		t.Errorf("DefaultEngine = %q, want claude", p.DefaultEngine)
	}
	if p.DefaultModelID != "sonnet-5" {
		t.Errorf("DefaultModelID = %q, want sonnet-5", p.DefaultModelID)
	}
	if p.DefaultEffort != "medium" {
		t.Errorf("DefaultEffort = %q, want medium", p.DefaultEffort)
	}
	if p.ContextBudgetTokens != 80000 {
		t.Errorf("ContextBudgetTokens = %d, want 80000", p.ContextBudgetTokens)
	}
	if !p.RefineryDefault {
		t.Error("RefineryDefault = false, want true (per-profile refinery for t0-hands)")
	}
	if p.NativeToolPolicy != NativeToolPolicyFull {
		t.Errorf("NativeToolPolicy = %q, want %q", p.NativeToolPolicy, NativeToolPolicyFull)
	}
	if p.SystemTemplateID != "tier-t0-hands" {
		t.Errorf("SystemTemplateID = %q, want tier-t0-hands", p.SystemTemplateID)
	}
	if !p.SiblingFlows {
		t.Error("SiblingFlows = false, want true")
	}
	if p.Catalogue != nil {
		t.Errorf("Catalogue = %v, want nil (full console tool set)", p.Catalogue)
	}
}

func TestListProfiles_SortedByName(t *testing.T) {
	t.Parallel()
	profiles := ListProfiles()
	if len(profiles) != 2 {
		t.Fatalf("ListProfiles() = %d entries, want 2 (t0-decider, t0-hands)", len(profiles))
	}
	for i := 1; i < len(profiles); i++ {
		if profiles[i-1].Name >= profiles[i].Name {
			t.Errorf("profiles not sorted: %q >= %q at index %d", profiles[i-1].Name, profiles[i].Name, i)
		}
	}
	names := map[string]bool{}
	for _, p := range profiles {
		names[p.Name] = true
	}
	if !names["t0-decider"] || !names["t0-hands"] {
		t.Errorf("ListProfiles() = %+v, want t0-decider and t0-hands", names)
	}
}

// TestListProfiles_SiblingFlows verifies both built-in profiles set
// SiblingFlows true — each is a valid SwitchModel/OpenHandsSibling origin.
func TestListProfiles_SiblingFlows(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"t0-decider", "t0-hands"} {
		p, err := ProfileByName(name)
		if err != nil {
			t.Fatalf("ProfileByName(%q): %v", name, err)
		}
		if !p.SiblingFlows {
			t.Errorf("%s.SiblingFlows = false, want true", name)
		}
	}
}
