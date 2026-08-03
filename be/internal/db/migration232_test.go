package db

import (
	"strings"
	"testing"
)

// 000232 appends workflow_wait guidance to the tier-t0-decider and
// tier-t0-bare injectables, matching the tool's addition to both console
// catalogues (console/profiles.go).
func TestMigration232_T0TemplatesMentionWorkflowWait(t *testing.T) {
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
		if !strings.Contains(template, "workflow_wait") {
			t.Errorf("%s template missing workflow_wait guidance; got %q", id, template)
		}
	}
}
