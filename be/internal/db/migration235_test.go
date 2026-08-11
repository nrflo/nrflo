package db

import (
	"strings"
	"testing"
)

// 000235 appends a verification-pass bullet to the t0 templates: extractor
// tiers run cheap models, so audit/verification briefs must be followed by an
// adversarial re-check pass instead of treating one fanout as final.
func TestMigration235_T0TemplatesVerifyPass(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	for _, id := range []string{"tier-t0-decider", "tier-t0-bare"} {
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
		if !strings.Contains(template, "adversarially re-checks each positive claim") {
			t.Errorf("%s template missing verification-pass guidance; got %q", id, template)
		}
		// The bullet must land as its own line, not spliced mid-bullet.
		if !strings.Contains(template, "act only when notified.\n- For audit/verification briefs") {
			t.Errorf("%s verification bullet not on its own line after the delegation bullet", id)
		}
	}
}
