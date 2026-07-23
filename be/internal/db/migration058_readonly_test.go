package db

import (
	"strings"
	"testing"
)

// Split from migration058_test.go to stay under the 300-line file cap.

// TestMigration058_ReadonlyFlagPreserved verifies migration 000058 did NOT
// change the readonly flag (or any other metadata) on the six rows.
func TestMigration058_ReadonlyFlagPreserved(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	for id := range expectedAgentLen {
		t.Run(id, func(t *testing.T) {
			var readonly int
			var typ string
			err := pool.QueryRow(
				`SELECT readonly, type FROM default_templates WHERE id = ?`, id,
			).Scan(&readonly, &typ)
			if err != nil {
				t.Fatalf("query %s: %v", id, err)
			}
			if readonly != 1 {
				t.Errorf("%s: readonly = %d, want 1", id, readonly)
			}
			if typ != "agent" {
				t.Errorf("%s: type = %q, want %q", id, typ, "agent")
			}
		})
	}
}

// TestMigration058_RestoreWouldReturnNewBaseline verifies that the Restore
// endpoint behaviour (UPDATE template = default_template) is idempotent once
// migration 000058 has run, because template already equals default_template.
// This mirrors the acceptance criterion "POST .../restore returns the new
// template text".
func TestMigration058_RestoreWouldReturnNewBaseline(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	for id := range expectedAgentLen {
		t.Run(id, func(t *testing.T) {
			// Simulate a user customisation that the Restore button must undo.
			if _, err := pool.Exec(
				`UPDATE default_templates SET template = 'USER EDIT' WHERE id = ?`, id,
			); err != nil {
				t.Fatalf("simulate user edit %s: %v", id, err)
			}

			// Emulate Restore endpoint logic: template := default_template.
			if _, err := pool.Exec(
				`UPDATE default_templates SET template = default_template WHERE id = ? AND readonly = 1`, id,
			); err != nil {
				t.Fatalf("restore %s: %v", id, err)
			}

			var tmpl string
			if err := pool.QueryRow(
				`SELECT template FROM default_templates WHERE id = ?`, id,
			).Scan(&tmpl); err != nil {
				t.Fatalf("read restored %s: %v", id, err)
			}
			if !noRoleHeaderAgents[id] && !strings.Contains(tmpl, "## Role") {
				t.Errorf("%s: restored template does not contain %q (restore did not return new baseline)", id, "## Role")
			}
			if tmpl == "USER EDIT" {
				t.Errorf("%s: restore did not overwrite user edit", id)
			}
			for _, legacy := range legacyHeaderFragments {
				if strings.Contains(tmpl, legacy) {
					t.Errorf("%s: restored template still contains legacy fragment %q", id, legacy)
				}
			}
		})
	}
}
