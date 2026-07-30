package db

import (
	"strings"
	"testing"
)

// TestMigration219_SeedsRestartFeedbackInjectables verifies migration 000219
// seeds the readonly `validation-failure` and `timeout-restart` injectables
// rendered by the restart-feedback prepend
// (spawner/template_restart_feedback.go), and removes the now-dead
// `continuation` row seeded by 000054.
func TestMigration219_SeedsRestartFeedbackInjectables(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	t.Run("validation-failure", func(t *testing.T) {
		var id, name, tmplType, template, defaultTemplate string
		var readonly int
		err := pool.QueryRow(
			`SELECT id, name, type, template, readonly, COALESCE(default_template, '') FROM default_templates WHERE id = 'validation-failure'`,
		).Scan(&id, &name, &tmplType, &template, &readonly, &defaultTemplate)
		if err != nil {
			t.Fatalf("query validation-failure: %v", err)
		}
		if tmplType != "injectable" {
			t.Errorf("type = %q, want 'injectable'", tmplType)
		}
		if readonly != 1 {
			t.Errorf("readonly = %d, want 1", readonly)
		}
		if template == "" {
			t.Fatal("template is empty")
		}
		if defaultTemplate != template {
			t.Errorf("default_template != template; want them equal for a freshly-seeded readonly row\ntemplate=%q\ndefault_template=%q", template, defaultTemplate)
		}
		for _, want := range []string{"${FAILED_COMMAND}", "${EXIT_CODE}", "${OUTPUT_TAIL}", "${PREVIOUS_DATA}", "Failed Validation"} {
			if !strings.Contains(template, want) {
				t.Errorf("validation-failure template missing %q", want)
			}
		}
	})

	t.Run("timeout-restart", func(t *testing.T) {
		var id, name, tmplType, template, defaultTemplate string
		var readonly int
		err := pool.QueryRow(
			`SELECT id, name, type, template, readonly, COALESCE(default_template, '') FROM default_templates WHERE id = 'timeout-restart'`,
		).Scan(&id, &name, &tmplType, &template, &readonly, &defaultTemplate)
		if err != nil {
			t.Fatalf("query timeout-restart: %v", err)
		}
		if tmplType != "injectable" {
			t.Errorf("type = %q, want 'injectable'", tmplType)
		}
		if readonly != 1 {
			t.Errorf("readonly = %d, want 1", readonly)
		}
		if template == "" {
			t.Fatal("template is empty")
		}
		if defaultTemplate != template {
			t.Errorf("default_template != template; want them equal for a freshly-seeded readonly row\ntemplate=%q\ndefault_template=%q", template, defaultTemplate)
		}
		for _, want := range []string{"${PREVIOUS_DATA}", "Timed Out"} {
			if !strings.Contains(template, want) {
				t.Errorf("timeout-restart template missing %q", want)
			}
		}
	})

	t.Run("continuation row removed", func(t *testing.T) {
		var count int
		if err := pool.QueryRow(`SELECT COUNT(*) FROM default_templates WHERE id = 'continuation'`).Scan(&count); err != nil {
			t.Fatalf("count continuation: %v", err)
		}
		if count != 0 {
			t.Errorf("continuation row count = %d, want 0 (dead injectable, deleted by 000219)", count)
		}
	})
}
