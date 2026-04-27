package db

import (
	"path/filepath"
	"strings"
	"testing"

	"be/internal/db/migrations"
)

// no-op:no-op was the escape-hatch finding the obsolete instant-stall
// detection consumed. After the feature removal, the seeded
// system_agent_definitions prompts must no longer instruct agents to write
// it. Migration 000061 strips the literal block from any already-seeded row.
const noOpSignalLiteral = "no-op:no-op"

// loadMigration061 reads the actual 000061 up migration SQL from the embed FS
// so tests exercise the real REPLACE statements (not a hand-copied duplicate).
func loadMigration061(t *testing.T) string {
	t.Helper()
	data, err := migrations.FS.ReadFile("000061_drop_no_op_signal.up.sql")
	if err != nil {
		t.Fatalf("read embedded migration: %v", err)
	}
	return string(data)
}

// loadSystemAgentPrompt returns the prompt text for the given system agent
// definition row, or fails the test if the row is missing.
func loadSystemAgentPrompt(t *testing.T, pool *Pool, id string) string {
	t.Helper()
	var prompt string
	err := pool.QueryRow(
		"SELECT prompt FROM system_agent_definitions WHERE id = ?", id,
	).Scan(&prompt)
	if err != nil {
		t.Fatalf("load %q prompt: %v", id, err)
	}
	return prompt
}

// TestMigration061_FreshMigrationLeavesNoNoOpSignal verifies that after all
// migrations run on a fresh DB, neither seeded system agent's prompt mentions
// the dead no-op:no-op finding. Confirms migrations 000039/000052 were edited
// to no longer seed the line; 000061's REPLACE is a no-op for fresh DBs.
func TestMigration061_FreshMigrationLeavesNoNoOpSignal(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	for _, id := range []string{"conflict-resolver", "context-saver"} {
		prompt := loadSystemAgentPrompt(t, pool, id)
		if strings.Contains(prompt, noOpSignalLiteral) {
			t.Errorf("system agent %q prompt still contains %q after fresh migrations", id, noOpSignalLiteral)
		}
	}
}

// TestMigration061_ConflictResolverPromptIntactAfterMigrations sanity-checks
// that the strip did not corrupt the conflict-resolver prompt: core
// instructions (template variables, agent fail call, merge command) survive.
// Migration 000059 has already rewritten "nrflow" → "nrflo", so the post-
// migration text uses the renamed binary.
func TestMigration061_ConflictResolverPromptIntactAfterMigrations(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	prompt := loadSystemAgentPrompt(t, pool, "conflict-resolver")
	for _, want := range []string{
		"Merge Conflict Resolver",
		"${BRANCH_NAME}",
		"${DEFAULT_BRANCH}",
		"${MERGE_ERROR}",
		"nrflo agent fail",
		"git merge",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("conflict-resolver prompt missing fragment %q", want)
		}
	}
}

// TestMigration061_ContextSaverPromptIntactAfterMigrations confirms the
// context-saver prompt retains the singular to_resume command and lost the
// two-command block.
func TestMigration061_ContextSaverPromptIntactAfterMigrations(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	prompt := loadSystemAgentPrompt(t, pool, "context-saver")
	if !strings.Contains(prompt, "Then run this command:") {
		t.Errorf("context-saver missing 'Then run this command:' header")
	}
	if strings.Contains(prompt, "Then run these two commands in order:") {
		t.Errorf("context-saver still contains the deleted two-command block")
	}
	if !strings.Contains(prompt, "NRF_SESSION_ID=${TARGET_SESSION_ID} nrflo findings add to_resume") {
		t.Errorf("context-saver missing the to_resume command (post-rename)")
	}
}

// TestMigration061_NoOpSignalAbsentFromAllSystemPrompts is a sweeping
// regression guard: no row in system_agent_definitions should reference the
// dead no-op:no-op signal after migrations run.
func TestMigration061_NoOpSignalAbsentFromAllSystemPrompts(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	rows, err := pool.Query("SELECT id, prompt FROM system_agent_definitions")
	if err != nil {
		t.Fatalf("query system_agent_definitions: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, prompt string
		if err := rows.Scan(&id, &prompt); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if strings.Contains(prompt, noOpSignalLiteral) {
			t.Errorf("system_agent_definitions[%s].prompt still contains %q", id, noOpSignalLiteral)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
}

// TestMigration061_LeavesUserCustomizedPromptsAlone verifies that re-running
// the migration on prompts that no longer contain the literal block (e.g.,
// user-customized) is a no-op. SQLite REPLACE is a literal substring match,
// so missing patterns must leave content byte-for-byte unchanged.
func TestMigration061_LeavesUserCustomizedPromptsAlone(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	customConflict := "# Custom Resolver\n\nUser rewrote the entire prompt. No literal block here."
	customContext := "# Custom Saver\n\nUser changed everything; just write to_resume directly."

	if _, err := pool.Exec(
		"UPDATE system_agent_definitions SET prompt = ? WHERE id = 'conflict-resolver'",
		customConflict,
	); err != nil {
		t.Fatalf("seed custom conflict-resolver: %v", err)
	}
	if _, err := pool.Exec(
		"UPDATE system_agent_definitions SET prompt = ? WHERE id = 'context-saver'",
		customContext,
	); err != nil {
		t.Fatalf("seed custom context-saver: %v", err)
	}

	if _, err := pool.Exec(loadMigration061(t)); err != nil {
		t.Fatalf("re-apply migration 000061: %v", err)
	}

	if got := loadSystemAgentPrompt(t, pool, "conflict-resolver"); got != customConflict {
		t.Errorf("custom conflict-resolver prompt was modified.\n got: %q\nwant: %q", got, customConflict)
	}
	if got := loadSystemAgentPrompt(t, pool, "context-saver"); got != customContext {
		t.Errorf("custom context-saver prompt was modified.\n got: %q\nwant: %q", got, customContext)
	}
}

// TestMigration061_StripsPostRenameLegacyPrompts verifies that migration
// 000061 strips the no-op:no-op escape-hatch line from prompts that have
// already been rewritten by migration 000059 (nrflow → nrflo). This is the
// real-world ordering — 000059 runs before 000061 — so the migration patterns
// must match the post-rename "nrflo" binary name.
func TestMigration061_StripsPostRenameLegacyPrompts(t *testing.T) {
	pool, err := NewPoolPath(filepath.Join(t.TempDir(), "test.db"), DefaultPoolConfig())
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	// Simulate a DB that was seeded with the legacy prompts and then ran
	// migration 000059 (nrflow → nrflo). The no-op:no-op line is still
	// present, but the binary is spelled "nrflo".
	postRenameConflict := strings.ReplaceAll(legacyConflictResolverPromptPreRename(), "nrflow", "nrflo")
	postRenameContext := strings.ReplaceAll(legacyContextSaverPromptPreRename(), "nrflow", "nrflo")

	if _, err := pool.Exec(
		"UPDATE system_agent_definitions SET prompt = ? WHERE id = 'conflict-resolver'",
		postRenameConflict,
	); err != nil {
		t.Fatalf("seed post-rename conflict-resolver: %v", err)
	}
	if _, err := pool.Exec(
		"UPDATE system_agent_definitions SET prompt = ? WHERE id = 'context-saver'",
		postRenameContext,
	); err != nil {
		t.Fatalf("seed post-rename context-saver: %v", err)
	}

	if _, err := pool.Exec(loadMigration061(t)); err != nil {
		t.Fatalf("apply migration 000061: %v", err)
	}

	for _, id := range []string{"conflict-resolver", "context-saver"} {
		got := loadSystemAgentPrompt(t, pool, id)
		if strings.Contains(got, noOpSignalLiteral) {
			t.Errorf("%s: migration 000061 failed to strip %q from post-rename prompt", id, noOpSignalLiteral)
		}
	}
}

// legacyConflictResolverPromptPreRename returns a faithful reproduction of
// the conflict-resolver prompt as seeded by the original (pre-edit) 000039
// migration: contains the no-op:no-op escape-hatch line and uses the legacy
// "nrflow" binary name.
func legacyConflictResolverPromptPreRename() string {
	return "# Merge Conflict Resolver\n\n" +
		"## Rules\n\n" +
		"- Do NOT modify any code beyond what is necessary to resolve conflicts\n" +
		"- If the conflict is too complex to resolve confidently, call `nrflow agent fail --reason \"description of why\"`\n" +
		"- If there is nothing to do, run `nrflow findings add no-op:no-op` before exiting\n" +
		"\n## Exit\n- Exit 0 on success"
}

// legacyContextSaverPromptPreRename returns a faithful reproduction of the
// context-saver prompt as seeded by the original (pre-edit) 000052 migration.
func legacyContextSaverPromptPreRename() string {
	return "# Context Saver\n\npreamble\n\n" +
		"Then run these two commands in order:\n\n" +
		"```bash\n" +
		"NRF_SESSION_ID=${TARGET_SESSION_ID} nrflow findings add to_resume \"<your concise summary>\"\n" +
		"```\n\n" +
		"```bash\n" +
		"nrflow findings add no-op:no-op\n" +
		"```\n\n## Rules\n- foo"
}
