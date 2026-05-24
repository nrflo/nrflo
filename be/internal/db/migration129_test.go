package db

import (
	"strings"
	"testing"
)

// TestMigration129_SystemPromptOverrideSeeded verifies that migration 000129
// re-seeds the readonly "system-prompt" injectable in place, replacing the
// near-empty v0.6.1 placeholder with the composed autonomous-agent baseline.
// Both template and default_template are set to the identical override text.
//
// Assertions use stable substring markers rather than the full body so that
// admin-neutral wording edits to the baseline don't make the test brittle.
func TestMigration129_SystemPromptOverrideSeeded(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var template, defaultTemplate string
	err = pool.QueryRow(
		`SELECT template, default_template FROM default_templates WHERE id = 'system-prompt'`,
	).Scan(&template, &defaultTemplate)
	if err != nil {
		t.Fatalf("query system-prompt template: %v", err)
	}

	// Stable markers present in the composed override body.
	markers := []string{
		"autonomous software-engineering agent",
		"OWASP",
	}
	for _, col := range []struct {
		name string
		val  string
	}{
		{"template", template},
		{"default_template", defaultTemplate},
	} {
		for _, want := range markers {
			if !strings.Contains(col.val, want) {
				t.Errorf("%s does not contain marker %q", col.name, want)
			}
		}
	}

	// The re-seed sets both columns to identical text (restore parity).
	if template != defaultTemplate {
		t.Errorf("template and default_template differ; want identical re-seeded text")
	}

	// The seeded body must carry no injectable-expansion tokens: expandInjectable
	// substitutes a fixed ${VAR} set then strips remaining ${...}, and #{...} is
	// left untouched, so any such literal would survive into the rendered prompt.
	for _, tok := range []string{"${", "#{"} {
		if strings.Contains(template, tok) {
			t.Errorf("template contains expansion token %q; seeded text must be literal", tok)
		}
	}
}
