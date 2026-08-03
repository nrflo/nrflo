package db

import (
	"strings"
	"testing"
)

// 000232 appended workflow_wait guidance to the tier-t0-decider/tier-t0-bare
// injectables; 000233 replaced it with wait-for-notification guidance (the
// server pushes delegation/sub-workflow completions into the launching
// console chat — console.ChatNotifier). This asserts the post-233 state.
func TestMigration233_T0TemplatesWaitForNotification(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	for _, id := range []string{"tier-t0-decider", "tier-t0-bare"} {
		var template, defaultTemplate string
		err := pool.QueryRow(
			`SELECT template, default_template FROM default_templates WHERE id = ?`, id,
		).Scan(&template, &defaultTemplate)
		if err != nil {
			t.Fatalf("SELECT default_templates id=%q: %v", id, err)
		}
		if template != defaultTemplate {
			t.Errorf("template != default_template (readonly invariant violated) for %q", id)
		}
		if !strings.Contains(template, "sends you a message") {
			t.Errorf("%s template missing notification guidance; got %q", id, template)
		}
		if strings.Contains(template, "block on `workflow_wait`") {
			t.Errorf("%s template still carries 000232's polling guidance; got %q", id, template)
		}
	}
}
