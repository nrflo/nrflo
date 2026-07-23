package db

import (
	"strings"
	"testing"
)

// 000203 seeds a readonly "stepwise-guidance" injectable: the stepwise
// operating rules + complete_step contract appended to a
// prompt_mode='stepwise' agent def's rendered prompt.

// TestMigration203_StepwiseGuidanceSeeded verifies the readonly injectable
// row: type, readonly, template==default_template (migration058 invariant),
// and the contract anchors the spawner's appendStepwiseBlock/tests key off of.
func TestMigration203_StepwiseGuidanceSeeded(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var template, defaultTemplate, typ string
	var readonly int
	err = pool.QueryRow(
		`SELECT template, default_template, readonly, type FROM default_templates WHERE id = 'stepwise-guidance'`,
	).Scan(&template, &defaultTemplate, &readonly, &typ)
	if err != nil {
		t.Fatalf("SELECT default_templates id=stepwise-guidance: %v", err)
	}
	if readonly != 1 {
		t.Errorf("readonly = %d, want 1", readonly)
	}
	if typ != "injectable" {
		t.Errorf("type = %q, want %q", typ, "injectable")
	}
	if template != defaultTemplate {
		t.Errorf("template != default_template (readonly invariant violated):\ntemplate=%q\ndefault_template=%q", template, defaultTemplate)
	}
	for _, anchor := range []string{
		"${STEP_INDEX}", "${STEP_TOTAL}", "${STEP_TITLE}", "${STEP_ID}", "${STEP_REVISION}",
		"complete_step", "findings_add", "revision",
	} {
		if !strings.Contains(template, anchor) {
			t.Errorf("template missing anchor %q; got %q", anchor, template)
		}
	}
}

// TestMigration203_ReadonlyInvariantHoldsRepoWide re-verifies migration058's
// acceptance criterion after 000203's insert: no readonly row has
// template != default_template.
func TestMigration203_ReadonlyInvariantHoldsRepoWide(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var mismatched int
	err = pool.QueryRow(
		"SELECT COUNT(*) FROM default_templates WHERE readonly = 1 AND template != default_template",
	).Scan(&mismatched)
	if err != nil {
		t.Fatalf("count mismatched rows: %v", err)
	}
	if mismatched != 0 {
		t.Errorf("readonly rows with template != default_template = %d, want 0", mismatched)
	}
}
