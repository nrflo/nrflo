package db

import (
	"strings"
	"testing"
)

// 000234 scopes 000233's blanket wait-for-notification guidance to async
// launches: extractor delegations return inline (delegate's default
// wait_sec 120), so the templates must not tell the model to end its turn
// and wait after one.
func TestMigration234_T0TemplatesExtractorInline(t *testing.T) {
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
		// 000239 widens the sentence to "Extractor and verifier delegations
		// return ..." — anchor on the stable tail.
		if !strings.Contains(template, "delegations return their findings inline") {
			t.Errorf("%s template missing extractor-inline guidance; got %q", id, template)
		}
		if strings.Contains(template, "After launching a delegation or a run (delegate/dynamic_workflow/workflow_run)") {
			t.Errorf("%s template still carries 000233's blanket delegation guidance; got %q", id, template)
		}
	}
}
