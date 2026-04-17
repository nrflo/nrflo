package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// Migration 000058 re-baselines the six readonly agent default templates.
// These tests verify that after all migrations run, every readonly agent row
// has template == default_template, both columns reflect the new canonical
// baseline, and the injectable rows were left untouched.

// expectedAgentLen holds the canonical byte length of each new readonly
// agent template as documented in the ticket. The test allows a +/- 2 byte
// tolerance to absorb trailing-newline / line-ending differences introduced
// by SQL literal formatting.
var expectedAgentLen = map[string]int{
	"setup-analyzer": 712,
	"test-writer":    1428,
	"implementor":    1253,
	"qa-verifier":    1899,
	"doc-updater":    1717,
	"ticket-creator": 1762,
}

// expectedInjectableLen pins the byte lengths of injectable rows that MUST
// NOT be touched by migration 000058.
var expectedInjectableLen = map[string]int{
	"callback":          130,
	"low-context":       133,
	"user-instructions": 43,
}

// legacyHeaderFragments must not appear in the refreshed baselines — they are
// remnants of the pre-000058 "agent header" boilerplate that was removed.
var legacyHeaderFragments = []string{
	"## Agent: ${AGENT}",
	"## Ticket: ${TICKET_ID}",
	"## Parent Session:",
	"## Child Session:",
}

// ticketScopedAgents reference ${TICKET_TITLE}; ticket-creator does NOT.
var ticketScopedAgents = []string{
	"setup-analyzer",
	"test-writer",
	"implementor",
	"qa-verifier",
	"doc-updater",
}

func withinTolerance(got, want int) bool {
	delta := got - want
	if delta < 0 {
		delta = -delta
	}
	return delta <= 2
}

// TestMigration058_TemplatesResynced verifies that after all migrations run,
// no readonly row has template != default_template. This is the acceptance
// criterion "SELECT COUNT(*) ... WHERE readonly = 1 AND template != default_template → 0".
func TestMigration058_TemplatesResynced(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
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

// TestMigration058_SixAgentBaselinesUpdated verifies each of the six readonly
// agent templates has the expected length (±2 bytes), template==default_template,
// contains "## Role", and contains no legacy header fragments.
func TestMigration058_SixAgentBaselinesUpdated(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	for id, wantLen := range expectedAgentLen {
		t.Run(id, func(t *testing.T) {
			var tmpl, def string
			var tmplLen int
			err := pool.QueryRow(
				`SELECT template, default_template, length(template)
				 FROM default_templates WHERE id = ? AND readonly = 1`,
				id,
			).Scan(&tmpl, &def, &tmplLen)
			if err != nil {
				t.Fatalf("query %s: %v", id, err)
			}

			if tmpl != def {
				t.Errorf("%s: template != default_template (lens %d vs %d)", id, len(tmpl), len(def))
			}
			if !withinTolerance(tmplLen, wantLen) {
				t.Errorf("%s: length(template) = %d, want %d ±2", id, tmplLen, wantLen)
			}
			if !strings.Contains(tmpl, "## Role") {
				t.Errorf("%s: template does not contain %q", id, "## Role")
			}
			for _, legacy := range legacyHeaderFragments {
				if strings.Contains(tmpl, legacy) {
					t.Errorf("%s: template still contains legacy fragment %q", id, legacy)
				}
			}
		})
	}
}

// TestMigration058_TicketScopedAgentsHaveTicketTitle verifies the five
// ticket-scoped agents reference ${TICKET_TITLE}; ticket-creator does NOT
// (it is project-scoped).
func TestMigration058_TicketScopedAgentsHaveTicketTitle(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	for _, id := range ticketScopedAgents {
		t.Run(id, func(t *testing.T) {
			var tmpl string
			if err := pool.QueryRow(
				`SELECT template FROM default_templates WHERE id = ? AND readonly = 1`, id,
			).Scan(&tmpl); err != nil {
				t.Fatalf("query %s: %v", id, err)
			}
			if !strings.Contains(tmpl, "${TICKET_TITLE}") {
				t.Errorf("%s: template does not contain %q", id, "${TICKET_TITLE}")
			}
		})
	}

	// ticket-creator is project-scoped.
	var tmpl string
	if err := pool.QueryRow(
		`SELECT template FROM default_templates WHERE id = 'ticket-creator' AND readonly = 1`,
	).Scan(&tmpl); err != nil {
		t.Fatalf("query ticket-creator: %v", err)
	}
	if strings.Contains(tmpl, "${TICKET_TITLE}") {
		t.Errorf("ticket-creator template must NOT contain %q", "${TICKET_TITLE}")
	}
}

// TestMigration058_InjectablesUntouched verifies the injectable rows that
// migration 000058 explicitly does NOT touch keep their original lengths.
// Also confirms the 'continuation' injectable row (removed by 000056) remains absent.
func TestMigration058_InjectablesUntouched(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	for id, wantLen := range expectedInjectableLen {
		t.Run(id, func(t *testing.T) {
			var tmplLen int
			err := pool.QueryRow(
				`SELECT length(template) FROM default_templates WHERE id = ?`, id,
			).Scan(&tmplLen)
			if err != nil {
				t.Fatalf("query %s: %v", id, err)
			}
			if tmplLen != wantLen {
				t.Errorf("%s: length(template) = %d, want %d", id, tmplLen, wantLen)
			}
		})
	}

	// continuation injectable was dropped by 000056 and must NOT be re-seeded
	// by 000058.
	var count int
	if err := pool.QueryRow(
		"SELECT COUNT(*) FROM default_templates WHERE id = 'continuation'",
	).Scan(&count); err != nil {
		t.Fatalf("count continuation: %v", err)
	}
	if count != 0 {
		t.Errorf("continuation row count = %d, want 0 (dropped by 000056)", count)
	}
}

// TestMigration058_UpdatedAtBumped verifies the six readonly agent rows have
// updated_at set to the migration timestamp 2026-04-17T00:00:00Z.
func TestMigration058_UpdatedAtBumped(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	const want = "2026-04-17T00:00:00Z"
	for id := range expectedAgentLen {
		t.Run(id, func(t *testing.T) {
			var updatedAt string
			err := pool.QueryRow(
				`SELECT updated_at FROM default_templates WHERE id = ? AND readonly = 1`, id,
			).Scan(&updatedAt)
			if err != nil {
				t.Fatalf("query %s: %v", id, err)
			}
			if updatedAt != want {
				t.Errorf("%s: updated_at = %q, want %q", id, updatedAt, want)
			}
		})
	}
}

// TestMigration058_ReadonlyFlagPreserved verifies migration 000058 did NOT
// change the readonly flag (or any other metadata) on the six rows.
func TestMigration058_ReadonlyFlagPreserved(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
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
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
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
			if !strings.Contains(tmpl, "## Role") {
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
