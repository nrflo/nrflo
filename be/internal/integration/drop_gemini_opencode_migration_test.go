package integration

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// TestMigration127AgentDefsReassignedFromGeminiOpenCode seeds agent_definitions
// that reference gemini_ and opencode_ model IDs at migration 126, then applies
// migration 127 and verifies the model columns are rewritten to 'sonnet'/empty.
func TestMigration127AgentDefsReassignedFromGeminiOpenCode(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migrate127.db")
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	m := buildMigrator(t, sqlDB)
	migrateTo(t, m, 126)

	now := "2026-05-01T00:00:00Z"
	if _, err := sqlDB.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"proj-127", "p127", "/tmp", now, now); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := sqlDB.Exec(
		`INSERT INTO workflows (id, project_id, description, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"wf-127", "proj-127", "d", now, now); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}

	agents := []struct {
		id, model, lcModel string
	}{
		{"gemini-main", "gemini_pro", ""},
		{"gemini-lc", "sonnet", "gemini_flash"},
		{"opencode-main", "opencode_gpt_4_5", ""},
		{"opencode-lc", "sonnet", "opencode_mini"},
		{"gemini-both", "gemini_flash_lite", "opencode_mini"},
		{"untouched", "sonnet", "haiku"},
	}
	for _, a := range agents {
		if _, err := sqlDB.Exec(
			`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, low_consumption_model, layer, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.id, "proj-127", "wf-127", a.model, 60, "p", a.lcModel, 0, now, now); err != nil {
			t.Fatalf("insert agent_def %q: %v", a.id, err)
		}
	}

	migrateTo(t, m, 127)

	wantModel := map[string]string{
		"gemini-main":   "sonnet",
		"gemini-lc":     "sonnet",
		"opencode-main": "sonnet",
		"opencode-lc":   "sonnet",
		"gemini-both":   "sonnet",
		"untouched":     "sonnet",
	}
	wantLC := map[string]string{
		"gemini-main":   "",
		"gemini-lc":     "",
		"opencode-main": "",
		"opencode-lc":   "",
		"gemini-both":   "",
		"untouched":     "haiku",
	}

	for id, wantM := range wantModel {
		var gotModel, gotLC string
		if err := sqlDB.QueryRow(
			`SELECT model, low_consumption_model FROM agent_definitions WHERE id = ? AND project_id = ? AND workflow_id = ?`,
			id, "proj-127", "wf-127").Scan(&gotModel, &gotLC); err != nil {
			t.Fatalf("scan agent_def %q: %v", id, err)
		}
		if gotModel != wantM {
			t.Errorf("agent_def %q: model = %q, want %q", id, gotModel, wantM)
		}
		if wantL := wantLC[id]; gotLC != wantL {
			t.Errorf("agent_def %q: low_consumption_model = %q, want %q", id, gotLC, wantL)
		}
	}
}

// TestMigration127SystemAgentDefsReassigned seeds system_agent_definitions with
// gemini/opencode models at migration 126 and verifies they become 'sonnet' after 127.
func TestMigration127SystemAgentDefsReassigned(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migrate127sys.db")
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	m := buildMigrator(t, sqlDB)
	migrateTo(t, m, 126)

	now := "2026-05-01T00:00:00Z"
	sysAgents := []struct{ id, role, model string }{
		{"sys-g1", "gemini-agent", "gemini_pro"},
		{"sys-o1", "opencode-agent", "opencode_gpt_4_5"},
		{"sys-s1", "sonnet-agent", "sonnet"},
	}
	for _, s := range sysAgents {
		if _, err := sqlDB.Exec(
			`INSERT INTO system_agent_definitions (id, model, timeout, prompt, role, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			s.id, s.model, 60, "p", s.role, now, now); err != nil {
			t.Fatalf("insert system_agent_def %q: %v", s.id, err)
		}
	}

	migrateTo(t, m, 127)

	wantModel := map[string]string{
		"sys-g1": "sonnet",
		"sys-o1": "sonnet",
		"sys-s1": "sonnet",
	}
	for id, want := range wantModel {
		var got string
		if err := sqlDB.QueryRow(
			`SELECT model FROM system_agent_definitions WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatalf("scan system_agent_def %q: %v", id, err)
		}
		if got != want {
			t.Errorf("system_agent_def %q: model = %q, want %q", id, got, want)
		}
	}
}

// TestMigration127GeminiOpenCodeRowsDeletedFromCLIModels verifies that any
// gemini/opencode cli_models rows seeded before migration 127 are removed.
func TestMigration127GeminiOpenCodeRowsDeletedFromCLIModels(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migrate127cli.db")
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	m := buildMigrator(t, sqlDB)
	// Run to 126 — at this point gemini rows exist (seeded by migration 106).
	migrateTo(t, m, 126)

	// Confirm gemini rows exist before 127.
	var beforeCount int
	if err := sqlDB.QueryRow(
		`SELECT COUNT(*) FROM cli_models WHERE cli_type IN ('gemini', 'opencode')`,
	).Scan(&beforeCount); err != nil {
		t.Fatalf("count before migration 127: %v", err)
	}
	if beforeCount == 0 {
		t.Skip("no gemini/opencode cli_models rows present at migration 126; skipping deletion check")
	}

	migrateTo(t, m, 127)

	var afterCount int
	if err := sqlDB.QueryRow(
		`SELECT COUNT(*) FROM cli_models WHERE cli_type IN ('gemini', 'opencode')`,
	).Scan(&afterCount); err != nil {
		t.Fatalf("count after migration 127: %v", err)
	}
	if afterCount != 0 {
		t.Errorf("cli_models gemini/opencode count = %d after migration 127, want 0", afterCount)
	}
}
