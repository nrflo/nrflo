package db

import (
	"strings"
	"testing"
)

// 000240: console chats launch every delegation tier async by default — the
// t0 templates carry the async contract and the delegation-guidance
// injectable names both surfaces' defaults.
func TestMigration240_ConsoleAsyncDelegateGuidance(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	for _, id := range []string{"tier-t0-decider", "tier-t0-bare"} {
		var tpl, def string
		if err := pool.QueryRow(
			`SELECT template, default_template FROM default_templates WHERE id = ?`, id,
		).Scan(&tpl, &def); err != nil {
			t.Fatalf("SELECT %s: %v", id, err)
		}
		if tpl != def {
			t.Errorf("%s: template != default_template (readonly invariant)", id)
		}
		if !strings.Contains(tpl, "Every delegation in this chat launches async") {
			t.Errorf("%s missing the console-async delegation contract", id)
		}
		if strings.Contains(tpl, "return their findings inline") {
			t.Errorf("%s still carries the inline-return contract", id)
		}
	}

	var guidance string
	if err := pool.QueryRow(
		`SELECT template FROM default_templates WHERE id = 'delegation-guidance'`,
	).Scan(&guidance); err != nil {
		t.Fatalf("SELECT delegation-guidance: %v", err)
	}
	if !strings.Contains(guidance, "in workflow runs it blocks inline by default") ||
		!strings.Contains(guidance, "in interactive console chats it launches async") {
		t.Errorf("delegation-guidance does not name both surfaces' defaults: %q", guidance)
	}
}
