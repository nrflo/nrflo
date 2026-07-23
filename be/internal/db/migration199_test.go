package db

import (
	"strings"
	"testing"
)

// TestMigration199_SeedsCrashResumeInjectable verifies migration 000199 seeds
// the readonly `crash-resume` injectable rendered as the first turn's input
// when a codex app-server thread/resume hand-off succeeds
// (spawner/codex_appserver_resume.go startOrResumeThread).
func TestMigration199_SeedsCrashResumeInjectable(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var id, name, tmplType, template, defaultTemplate string
	var readonly int
	err = pool.QueryRow(
		`SELECT id, name, type, template, readonly, COALESCE(default_template, '') FROM default_templates WHERE id = 'crash-resume'`,
	).Scan(&id, &name, &tmplType, &template, &readonly, &defaultTemplate)
	if err != nil {
		t.Fatalf("query crash-resume: %v", err)
	}

	if id != "crash-resume" {
		t.Errorf("id = %q, want 'crash-resume'", id)
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

	for _, want := range []string{"${RESTART_REASON}", "Completion Contract", "agent_finished", "agent_fail", "MCP tools"} {
		if !strings.Contains(template, want) {
			t.Errorf("crash-resume template missing %q", want)
		}
	}
}
