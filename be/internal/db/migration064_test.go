package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestMigration064_SeedsSystemPromptSuffix verifies that migration 000064
// inserts the system-prompt-suffix injectable template with expected values.
func TestMigration064_SeedsSystemPromptSuffix(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var id, name, tmplType, template string
	var readonly int
	err = pool.QueryRow(
		`SELECT id, name, type, template, readonly FROM default_templates WHERE id = 'system-prompt-suffix'`,
	).Scan(&id, &name, &tmplType, &template, &readonly)
	if err != nil {
		t.Fatalf("query system-prompt-suffix: %v", err)
	}

	if id != "system-prompt-suffix" {
		t.Errorf("id = %q, want 'system-prompt-suffix'", id)
	}
	if tmplType != "injectable" {
		t.Errorf("type = %q, want 'injectable'", tmplType)
	}
	if readonly != 1 {
		t.Errorf("readonly = %d, want 1", readonly)
	}
	// After migration 000068, system-prompt-suffix carries the autonomy rules
	// and the completion contract. `agent continue` was removed — it is the
	// spawner's internal low-context protocol, not user-facing.
	for _, want := range []string{"Completion Contract", "Autonomous Run", "nrflo agent finished", "nrflo agent fail"} {
		if !strings.Contains(template, want) {
			t.Errorf("system-prompt-suffix template missing %q", want)
		}
	}
}

// TestMigration064_SeedsFinishReminder verifies that migration 000064
// inserts the finish-reminder injectable template with expected values.
func TestMigration064_SeedsFinishReminder(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var id, name, tmplType, template string
	var readonly int
	err = pool.QueryRow(
		`SELECT id, name, type, template, readonly FROM default_templates WHERE id = 'finish-reminder'`,
	).Scan(&id, &name, &tmplType, &template, &readonly)
	if err != nil {
		t.Fatalf("query finish-reminder: %v", err)
	}

	if id != "finish-reminder" {
		t.Errorf("id = %q, want 'finish-reminder'", id)
	}
	if tmplType != "injectable" {
		t.Errorf("type = %q, want 'injectable'", tmplType)
	}
	if readonly != 1 {
		t.Errorf("readonly = %d, want 1", readonly)
	}
	// After migration 000066, finish-reminder uses `agent finished` for success.
	for _, want := range []string{"Before Finishing", "nrflo agent finished", "nrflo agent fail"} {
		if !strings.Contains(template, want) {
			t.Errorf("finish-reminder template missing %q", want)
		}
	}
}

// TestMigration064_NewInjectableTemplatesHaveDefaultTemplate verifies that
// both new readonly templates have default_template populated (for restore support).
func TestMigration064_NewInjectableTemplatesHaveDefaultTemplate(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	for _, id := range []string{"system-prompt-suffix", "finish-reminder"} {
		var defaultTemplate string
		err := pool.QueryRow(
			`SELECT COALESCE(default_template, '') FROM default_templates WHERE id = ?`, id,
		).Scan(&defaultTemplate)
		if err != nil {
			t.Fatalf("%s: query default_template: %v", id, err)
		}
		if defaultTemplate == "" {
			t.Errorf("%s: default_template is empty; should be set for readonly templates", id)
		}
	}
}

// TestMigration064_ContextSaverSweep verifies that migration 000064 removes
// the "Do NOT call / just exit 0" guidance from the context-saver prompt.
func TestMigration064_ContextSaverSweep(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var prompt string
	err = pool.QueryRow(
		`SELECT prompt FROM system_agent_definitions WHERE id = 'context-saver'`,
	).Scan(&prompt)
	if err != nil {
		t.Fatalf("query context-saver: %v", err)
	}

	// The "just exit 0" fragment removed by migration 000064 must be absent
	if strings.Contains(prompt, "just exit 0 after saving findings") {
		t.Error("context-saver prompt still contains 'just exit 0 after saving findings'; sweep failed")
	}
	// The replacement text must be present
	if !strings.Contains(prompt, "exit 0 or call `nrflo agent continue`") {
		t.Error("context-saver prompt missing replacement 'exit 0 or call `nrflo agent continue`'")
	}
}

// TestMigration064_NewInjectablePromptsContainNoNoOpGuidance verifies that
// neither new injectable template contains silent-exit / no-op style guidance.
func TestMigration064_NewInjectablePromptsContainNoNoOpGuidance(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	rows, err := pool.Query(
		`SELECT id, template FROM default_templates WHERE id IN ('system-prompt-suffix', 'finish-reminder')`,
	)
	if err != nil {
		t.Fatalf("query new templates: %v", err)
	}
	defer rows.Close()

	forbidden := []string{"no-op", "nothing to do", "just exit"}
	for rows.Next() {
		var id, template string
		if err := rows.Scan(&id, &template); err != nil {
			t.Fatalf("scan: %v", err)
		}
		lower := strings.ToLower(template)
		for _, phrase := range forbidden {
			if strings.Contains(lower, phrase) {
				t.Errorf("%s template contains forbidden phrase %q", id, phrase)
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
}

// TestMigration064_ExactlyTwoNewInjectablesSeeded verifies that exactly the
// two expected injectable rows from migration 000064 are present.
func TestMigration064_ExactlyTwoNewInjectablesSeeded(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var count int
	err = pool.QueryRow(
		`SELECT COUNT(*) FROM default_templates WHERE id IN ('system-prompt-suffix', 'finish-reminder')`,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count new injectable rows: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 new injectable rows (system-prompt-suffix + finish-reminder), got %d", count)
	}
}
