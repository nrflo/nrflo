package db

import (
	"strings"
	"testing"
)

// TestMigration196_RefineryPromptPreservesTaskAnchorWording verifies the
// _refinery prompt was updated in place: it still carries the original
// "Working-Set Refinery" framing and now also instructs the model to treat
// an optional ## Task section as an immutable verbatim anchor.
func TestMigration196_RefineryPromptPreservesTaskAnchorWording(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	var prompt string
	if err := pool.QueryRow(
		`SELECT prompt FROM system_agent_definitions WHERE id = '_refinery'`,
	).Scan(&prompt); err != nil {
		t.Fatalf("SELECT system_agent_definitions id=_refinery: %v", err)
	}

	if !strings.Contains(prompt, "Working-Set Refinery") {
		t.Errorf("_refinery prompt = %q, want it to still contain the original 'Working-Set Refinery' framing", prompt)
	}
	if !strings.Contains(prompt, "## Task") {
		t.Errorf("_refinery prompt = %q, want it to mention the ## Task section", prompt)
	}
	if !strings.Contains(prompt, "immutable") {
		t.Errorf("_refinery prompt = %q, want anchor-preservation wording (immutable)", prompt)
	}
	if !strings.Contains(prompt, "verbatim") {
		t.Errorf("_refinery prompt = %q, want anchor-preservation wording (verbatim)", prompt)
	}
}
