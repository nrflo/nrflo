package integration

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// TestMigration138MigratesOpus47To48 exercises migration 000138: agent and
// system-agent rows pinned to opus_4_7/opus_4_7_1m (model + low_consumption_model)
// are rewritten to opus_4_8/opus_4_8_1m, while the 4.7 built-in model rows survive
// (only references migrate, unlike 000057 which deleted the legacy rows).
func TestMigration138MigratesOpus47To48(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migrate138.db")
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	m := buildMigrator(t, sqlDB)
	migrateTo(t, m, 137)

	now := "2026-05-01T00:00:00Z"
	if _, err := sqlDB.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"proj-138", "p138", "/tmp", now, now); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := sqlDB.Exec(
		`INSERT INTO workflows (id, project_id, description, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"wf-138", "proj-138", "d", now, now); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}

	agents := []struct {
		id, model, lcModel string
	}{
		{"builder", "opus_4_7", ""},
		{"planner", "opus_4_7_1m", ""},
		{"writer", "sonnet", "opus_4_7"},
		{"analyst", "haiku", "opus_4_7_1m"},
		{"untouched", "opus_4_6", "sonnet"},
	}
	for _, a := range agents {
		if _, err := sqlDB.Exec(
			`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, low_consumption_model, layer, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.id, "proj-138", "wf-138", a.model, 60, "p", a.lcModel, 0, now, now); err != nil {
			t.Fatalf("insert agent_def %q: %v", a.id, err)
		}
	}

	sysAgents := []struct{ id, model string }{
		{"sys-1", "opus_4_7"},
		{"sys-2", "opus_4_7_1m"},
		{"sys-3", "sonnet"},
	}
	for _, s := range sysAgents {
		if _, err := sqlDB.Exec(
			`INSERT INTO system_agent_definitions (id, role, model, timeout, prompt, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			s.id, s.id, s.model, 60, "p", now, now); err != nil {
			t.Fatalf("insert system_agent_def %q: %v", s.id, err)
		}
	}

	migrateTo(t, m, 138)

	wantModel := map[string]string{
		"builder":   "opus_4_8",
		"planner":   "opus_4_8_1m",
		"writer":    "sonnet",
		"analyst":   "haiku",
		"untouched": "opus_4_6",
	}
	for id, want := range wantModel {
		var got string
		if err := sqlDB.QueryRow(
			`SELECT model FROM agent_definitions WHERE id = ? AND project_id = ? AND workflow_id = ?`,
			id, "proj-138", "wf-138").Scan(&got); err != nil {
			t.Fatalf("scan agent_def %q: %v", id, err)
		}
		if got != want {
			t.Errorf("agent_def %q: model = %q, want %q", id, got, want)
		}
	}

	wantLC := map[string]string{
		"builder":   "",
		"planner":   "",
		"writer":    "opus_4_8",
		"analyst":   "opus_4_8_1m",
		"untouched": "sonnet",
	}
	for id, want := range wantLC {
		var got string
		if err := sqlDB.QueryRow(
			`SELECT low_consumption_model FROM agent_definitions WHERE id = ? AND project_id = ? AND workflow_id = ?`,
			id, "proj-138", "wf-138").Scan(&got); err != nil {
			t.Fatalf("scan agent_def lc %q: %v", id, err)
		}
		if got != want {
			t.Errorf("agent_def %q: low_consumption_model = %q, want %q", id, got, want)
		}
	}

	wantSys := map[string]string{
		"sys-1": "opus_4_8",
		"sys-2": "opus_4_8_1m",
		"sys-3": "sonnet",
	}
	for id, want := range wantSys {
		var got string
		if err := sqlDB.QueryRow(
			`SELECT model FROM system_agent_definitions WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatalf("scan system_agent_def %q: %v", id, err)
		}
		if got != want {
			t.Errorf("system_agent_def %q: model = %q, want %q", id, got, want)
		}
	}

	// New 4.8 built-ins seeded with correct mapping + context in both registries.
	assertModelRow(t, sqlDB, "cli_models", "opus_4_8", "claude-opus-4-8", 200000)
	assertModelRow(t, sqlDB, "cli_models", "opus_4_8_1m", "claude-opus-4-8[1m]", 1000000)
	assertModelRow(t, sqlDB, "api_models", "opus_4_8", "claude-opus-4-8", 200000)
	assertModelRow(t, sqlDB, "api_models", "opus_4_8_1m", "claude-opus-4-8[1m]", 1000000)

	// 4.7 rows must NOT be deleted — they remain selectable built-ins.
	for _, tbl := range []string{"cli_models", "api_models"} {
		for _, id := range []string{"opus_4_7", "opus_4_7_1m"} {
			var count int
			if err := sqlDB.QueryRow(
				`SELECT COUNT(*) FROM `+tbl+` WHERE id = ?`, id).Scan(&count); err != nil {
				t.Fatalf("count %s %q: %v", tbl, id, err)
			}
			if count != 1 {
				t.Errorf("%s %q: count = %d, want 1 (4.7 rows must survive)", tbl, id, count)
			}
		}
	}
}

// assertModelRow asserts a single read-only model row exists with the expected
// mapped_model and context_length.
func assertModelRow(t *testing.T, sqlDB *sql.DB, table, id, wantMapped string, wantCtx int) {
	t.Helper()
	var mapped string
	var ctx int
	var readOnly int
	if err := sqlDB.QueryRow(
		`SELECT mapped_model, context_length, read_only FROM `+table+` WHERE id = ?`, id).
		Scan(&mapped, &ctx, &readOnly); err != nil {
		t.Fatalf("scan %s %q: %v", table, id, err)
	}
	if mapped != wantMapped {
		t.Errorf("%s %q: mapped_model = %q, want %q", table, id, mapped, wantMapped)
	}
	if ctx != wantCtx {
		t.Errorf("%s %q: context_length = %d, want %d", table, id, ctx, wantCtx)
	}
	if readOnly != 1 {
		t.Errorf("%s %q: read_only = %d, want 1", table, id, readOnly)
	}
}
