package db

import (
	"strings"
	"testing"
)

// 000227 appends a console-background-task bullet to the readonly
// delegation-guidance injectable; all prior bullets (000225-era) must survive
// verbatim and the readonly (template == default_template) invariant holds.

func TestMigration227_GuidanceTeachesConsoleBackgroundTask(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var template, defaultTemplate string
	err = pool.QueryRow(
		`SELECT template, default_template FROM default_templates WHERE id = 'delegation-guidance'`,
	).Scan(&template, &defaultTemplate)
	if err != nil {
		t.Fatalf("SELECT delegation-guidance: %v", err)
	}
	if template != defaultTemplate {
		t.Errorf("template != default_template (readonly invariant violated)")
	}
	for _, anchor := range []string{
		"background-task",
		"does not consume the delegation",
		"call `get_delegation` once more",
		// 000225-era bullets must still be present.
		"wait_sec",
		"blocks inline",
		"never call `get_delegation` repeatedly",
		"merge_delegation",
	} {
		if !strings.Contains(template, anchor) {
			t.Errorf("template missing anchor %q; got %q", anchor, template)
		}
	}
	if got := strings.Count(template, "## Delegation"); got != 1 {
		t.Errorf("template contains %d `## Delegation` headings, want exactly 1; got %q", got, template)
	}
}
