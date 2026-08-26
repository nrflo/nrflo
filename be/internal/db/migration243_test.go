package db

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
)

func TestMigration243_SimplifiesConsoleProfiles(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var count int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM default_templates WHERE id = 'tier-t0-bare'`).Scan(&count); err != nil {
		t.Fatalf("count tier-t0-bare: %v", err)
	}
	if count != 0 {
		t.Errorf("tier-t0-bare rows = %d, want 0", count)
	}

	for _, tc := range []struct {
		id      string
		anchors []string
	}{
		{"tier-t0-decider", []string{"answer directly", "one well-scoped worker", "Every delegation in this chat launches async", "verifier", "AskUserQuestion"}},
		{"tier-t0-hands", []string{"one coherent work slice", "Do not delegate", "return that decision to the decider"}},
	} {
		var template, defaultTemplate, typ string
		var readonly int
		err := pool.QueryRow(
			`SELECT template, default_template, readonly, type FROM default_templates WHERE id = ?`, tc.id,
		).Scan(&template, &defaultTemplate, &readonly, &typ)
		if errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("template %s missing", tc.id)
		}
		if err != nil {
			t.Fatalf("select %s: %v", tc.id, err)
		}
		if template != defaultTemplate || readonly != 1 || typ != "injectable" {
			t.Errorf("%s invariant: readonly=%d type=%q template==default=%v", tc.id, readonly, typ, template == defaultTemplate)
		}
		for _, anchor := range tc.anchors {
			if !strings.Contains(template, anchor) {
				t.Errorf("%s missing %q", tc.id, anchor)
			}
		}
	}
}
