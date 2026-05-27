package db

import (
	"strings"
	"testing"
)

// Migration 000137 rewrites the readonly ticket-creator default template to use
// the nrflo MCP ticket tools instead of the removed `nrflow` CLI.
func TestMigration137_TicketCreatorUsesMCPTools(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var tmpl, def string
	err = pool.QueryRow(
		`SELECT template, default_template FROM default_templates
		 WHERE id = 'ticket-creator' AND readonly = 1`,
	).Scan(&tmpl, &def)
	if err != nil {
		t.Fatalf("query ticket-creator: %v", err)
	}
	if tmpl != def {
		t.Errorf("template != default_template after 000137")
	}

	for _, want := range []string{"ticket_create", "ticket_add_dependency", "findings_add"} {
		if !strings.Contains(tmpl, want) {
			t.Errorf("ticket-creator template missing MCP tool reference %q", want)
		}
	}
	for _, banned := range []string{"nrflow", "nrflo tickets", "deps add", "findings add "} {
		if strings.Contains(tmpl, banned) {
			t.Errorf("ticket-creator template still references removed CLI: %q", banned)
		}
	}
}
