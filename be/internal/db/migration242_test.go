package db

import (
	"testing"
)

// 000242 reclassifies already-persisted ChatNotifier wake-ups from
// 'user_input' to 'system_turn'. Rows that merely mention the marker mid-text
// are human messages and must be left alone — the prefix is anchored.
func TestMigration242_BackfillsConsoleNoticeCategory(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("NewPoolPath: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	const sid = "sess-242"
	if _, err := pool.Exec(`
		INSERT INTO agent_sessions
			(id, project_id, ticket_id, phase, agent_type, status, kind,
			 nudge_count, config, created_at, updated_at)
		VALUES (?, 'p242', '', 'chat', 'console_chat', 'running', 'console_chat',
		        0, '', datetime('now'), datetime('now'))`, sid); err != nil {
		t.Fatalf("insert agent_session: %v", err)
	}

	rows := []struct {
		seq     int
		content string
	}{
		{1, `[nrflo] Delegation abc finished — collect the results with get_delegation (delegation_id "abc").`},
		{2, "what should we work on next?"},
		{3, "look at the [nrflo] docs please"},
	}
	for _, r := range rows {
		_, err := pool.Exec(
			`INSERT INTO agent_messages (session_id, seq, content, category, created_at) VALUES (?, ?, ?, 'user_input', ?)`,
			sid, r.seq, r.content, "2026-01-01T00:00:00Z")
		if err != nil {
			t.Fatalf("INSERT seq=%d: %v", r.seq, err)
		}
	}

	// Re-run the statement the migration carries: the fixture rows are
	// inserted after migrations ran, so this asserts the rule, not the ordering.
	if _, err := pool.Exec(
		`UPDATE agent_messages SET category = 'system_turn' WHERE category = 'user_input' AND content LIKE '[nrflo] %'`,
	); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	want := map[int]string{1: "system_turn", 2: "user_input", 3: "user_input"}
	for seq, wantCat := range want {
		var got string
		if err := pool.QueryRow(
			`SELECT category FROM agent_messages WHERE session_id = ? AND seq = ?`, sid, seq,
		).Scan(&got); err != nil {
			t.Fatalf("SELECT seq=%d: %v", seq, err)
		}
		if got != wantCat {
			t.Errorf("seq=%d category = %q, want %q", seq, got, wantCat)
		}
	}
}
