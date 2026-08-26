package db

import (
	"strings"
	"testing"
)

// 000241 appends the decision-card + no-bare-labels bullets to the t0
// templates: with AskUserQuestion restored to the "none" native-tool policy,
// an owner decision has a real surface and must stop being buried in prose.
func TestMigration241_T0TemplatesDecisionCard(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	for _, id := range []string{"tier-t0-decider"} {
		var template, defaultTemplate string
		err := pool.QueryRow(
			`SELECT template, default_template FROM default_templates WHERE id = ?`, id,
		).Scan(&template, &defaultTemplate)
		if err != nil {
			t.Fatalf("SELECT default_templates id=%q: %v", id, err)
		}
		if template != defaultTemplate {
			t.Errorf("template != default_template (readonly invariant violated) for %q", id)
		}
		if !strings.Contains(template, "ask with the AskUserQuestion tool") {
			t.Errorf("%s template missing decision-card guidance; got %q", id, template)
		}
		if !strings.Contains(template, "Never put a bare label in front of the owner") {
			t.Errorf("%s template missing no-bare-labels guidance; got %q", id, template)
		}
		// Both bullets must land as their own lines after 000235's bullet.
		if !strings.Contains(template, "low-stakes lookups.\n- When you need a decision from the owner") {
			t.Errorf("%s decision bullet not on its own line after the verification bullet", id)
		}
	}
}
