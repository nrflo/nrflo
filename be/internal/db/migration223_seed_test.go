package db

import (
	"strings"
	"testing"
)

// 000223 rewrites the readonly delegation-guidance injectable: bare-polling
// instruction replaced with the wait_sec contract, stale "T2" naming dropped.

func TestMigration223_GuidanceTeachesWaitSec(t *testing.T) {
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
	for _, anchor := range []string{"wait_sec", "blocks inline", "never call `get_delegation` repeatedly"} {
		if !strings.Contains(template, anchor) {
			t.Errorf("template missing anchor %q; got %q", anchor, template)
		}
	}
	for _, gone := range []string{"T2 `extractor`", "poll `get_delegation` for the result"} {
		if strings.Contains(template, gone) {
			t.Errorf("template still contains replaced phrase %q; got %q", gone, template)
		}
	}
}
