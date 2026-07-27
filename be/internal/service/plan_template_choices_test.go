package service

import (
	"encoding/json"
	"testing"

	"be/internal/clock"
)

// TestPlanTemplateChoices_MirrorsEnabledLibraryWithTiers covers the payload
// get_subworkflow hands a caller that wants to hand-author a manifest: the ids
// must be exactly the install-usable library (so an id copied from here always
// passes ValidatePlanManifest) and every entry must carry a resolved tier, the
// only signal for the dynwf_max_premium_workers cap.
func TestPlanTemplateChoices_MirrorsEnabledLibraryWithTiers(t *testing.T) {
	stubLookPath(t, func(name string) (string, error) { return "/usr/bin/" + name, nil })
	pool := seedDynamicWorkflowDB(t, "template_choices.db")

	all, err := AllowedTemplates(pool, GlobalProjectID, DynamicWorkflow)
	if err != nil {
		t.Fatalf("AllowedTemplates: %v", err)
	}
	want := EnabledTemplates(pool, clock.Real(), all)

	got, err := PlanTemplateChoices(pool, clock.Real(), GlobalProjectID, DynamicWorkflow)
	if err != nil {
		t.Fatalf("PlanTemplateChoices: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("choices = %d, want %d (the EnabledTemplates library)", len(got), len(want))
	}
	valid := map[string]bool{"cheap": true, "mid": true, "premium": true}
	for i, c := range got {
		if c.ID != want[i].ID {
			t.Errorf("choices[%d].ID = %q, want %q", i, c.ID, want[i].ID)
		}
		if !valid[c.Tier] {
			t.Errorf("choices[%d] (%s).Tier = %q, want cheap|mid|premium", i, c.ID, c.Tier)
		}
	}
}

// TestPlanTemplateChoicesJSON_DegradesToNil keeps the poll path best-effort: an
// unreadable library must not fail a get_subworkflow that still has an
// actionable manifest and revision to return.
func TestPlanTemplateChoicesJSON_DegradesToNil(t *testing.T) {
	stubLookPath(t, func(name string) (string, error) { return "/usr/bin/" + name, nil })
	pool := seedDynamicWorkflowDB(t, "template_choices_nil.db")

	if raw := PlanTemplateChoicesJSON(pool, clock.Real(), GlobalProjectID, "no-such-workflow"); raw != nil {
		t.Errorf("PlanTemplateChoicesJSON for an unknown workflow = %s, want nil", raw)
	}

	raw := PlanTemplateChoicesJSON(pool, clock.Real(), GlobalProjectID, DynamicWorkflow)
	var choices []PlanTemplateChoice
	if err := json.Unmarshal(raw, &choices); err != nil {
		t.Fatalf("unmarshal choices: %v (raw=%s)", err, raw)
	}
	if len(choices) == 0 {
		t.Fatal("choices = 0, want the seeded dynamic template library")
	}
}
