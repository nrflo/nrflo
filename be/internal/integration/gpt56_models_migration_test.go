package integration

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// TestMigration156SeedsGPT56Models verifies migration 000156 seeds the three
// GPT-5.6 codex models (sol/terra/luna) with correct mapped model, effort,
// and the 372k context window from the codex 0.144 model catalog.
func TestMigration156SeedsGPT56Models(t *testing.T) {
	env := NewTestEnv(t)

	cases := []struct {
		id          string
		displayName string
		mappedModel string
		effort      string
	}{
		{"codex_gpt56_sol_normal", "Codex GPT-5.6 Sol (Normal)", "gpt-5.6-sol", "medium"},
		{"codex_gpt56_sol_high", "Codex GPT-5.6 Sol (High)", "gpt-5.6-sol", "high"},
		{"codex_gpt56_terra_normal", "Codex GPT-5.6 Terra (Normal)", "gpt-5.6-terra", "medium"},
		{"codex_gpt56_terra_high", "Codex GPT-5.6 Terra (High)", "gpt-5.6-terra", "high"},
		{"codex_gpt56_luna_low", "Codex GPT-5.6 Luna (Low)", "gpt-5.6-luna", "low"},
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			var (
				cliType, displayName, mappedModel, reasoning string
				contextLen, readOnly, enabled                int
			)
			err := env.Pool.QueryRow(
				`SELECT cli_type, display_name, mapped_model, reasoning_effort, context_length, read_only, enabled
				   FROM cli_models WHERE id = ?`, tc.id).Scan(
				&cliType, &displayName, &mappedModel, &reasoning, &contextLen, &readOnly, &enabled)
			if err != nil {
				t.Fatalf("SELECT cli_models WHERE id=%q: %v", tc.id, err)
			}
			if cliType != "codex" {
				t.Errorf("cli_type = %q, want %q", cliType, "codex")
			}
			if displayName != tc.displayName {
				t.Errorf("display_name = %q, want %q", displayName, tc.displayName)
			}
			if mappedModel != tc.mappedModel {
				t.Errorf("mapped_model = %q, want %q", mappedModel, tc.mappedModel)
			}
			if reasoning != tc.effort {
				t.Errorf("reasoning_effort = %q, want %q", reasoning, tc.effort)
			}
			if contextLen != 372000 {
				t.Errorf("context_length = %d, want 372000", contextLen)
			}
			if readOnly != 1 {
				t.Errorf("read_only = %d, want 1", readOnly)
			}
			if enabled != 1 {
				t.Errorf("enabled = %d, want 1", enabled)
			}
		})
	}
}

// TestMigration156SeedsGPT56APIModels verifies the three openai api_models
// rows for GPT-5.6 Sol (low/medium/high) with the 372k context window.
func TestMigration156SeedsGPT56APIModels(t *testing.T) {
	env := NewTestEnv(t)

	for _, tc := range []struct{ id, effort string }{
		{"gpt56_sol_low", "low"},
		{"gpt56_sol_medium", "medium"},
		{"gpt56_sol_high", "high"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			var (
				provider, mappedModel, reasoning string
				contextLen, readOnly, enabled    int
			)
			err := env.Pool.QueryRow(
				`SELECT provider, mapped_model, reasoning_effort, context_length, read_only, enabled
				   FROM api_models WHERE id = ?`, tc.id).Scan(
				&provider, &mappedModel, &reasoning, &contextLen, &readOnly, &enabled)
			if err != nil {
				t.Fatalf("SELECT api_models WHERE id=%q: %v", tc.id, err)
			}
			if provider != "openai" || mappedModel != "gpt-5.6-sol" || reasoning != tc.effort {
				t.Errorf("row = %s/%s/%s, want openai/gpt-5.6-sol/%s", provider, mappedModel, reasoning, tc.effort)
			}
			if contextLen != 372000 || readOnly != 1 || enabled != 1 {
				t.Errorf("context/read_only/enabled = %d/%d/%d, want 372000/1/1", contextLen, readOnly, enabled)
			}
		})
	}
}

// TestMigration156MigratesAgentDefModelColumns exercises the UPDATE statements
// in migration 000156 by seeding agent_definitions rows under migration 155
// (referencing the gpt-5.3/5.4/5.5 codex ids), then running 156 and verifying
// model + low_consumption_model were rewritten to the GPT-5.6 ids.
func TestMigration156MigratesAgentDefModelColumns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migrate156.db")
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	m := buildMigrator(t, sqlDB)
	migrateTo(t, m, 155)

	now := "2026-07-13T00:00:00Z"
	if _, err := sqlDB.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"proj-156", "p156", "/tmp", now, now); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := sqlDB.Exec(
		`INSERT INTO workflows (id, project_id, description, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"wf-156", "proj-156", "d", now, now); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}

	agents := []struct {
		id, model, lcModel string
	}{
		{"legacy53", "codex_gpt_normal", ""},
		{"legacy53h", "codex_gpt_high", ""},
		{"gen54", "codex_gpt54_normal", "codex_gpt54_mini_low"},
		{"gen55", "codex_gpt55_high", "codex_gpt55_normal"},
		{"gen55mini", "codex_gpt55_mini_low", ""},
		{"untouched", "sonnet", "haiku"},
	}
	for _, a := range agents {
		if _, err := sqlDB.Exec(
			`INSERT INTO agent_definitions (id, project_id, workflow_id, model, timeout, prompt, low_consumption_model, layer, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.id, "proj-156", "wf-156", a.model, 60, "p", a.lcModel, 0, now, now); err != nil {
			t.Fatalf("insert agent_def %q: %v", a.id, err)
		}
	}

	// role+execution_mode carries a UNIQUE constraint — give each row its own role.
	sysAgents := []struct{ id, role, model string }{
		{"sys-53", "role-53", "codex_gpt_high"},
		{"sys-55", "role-55", "codex_gpt55_normal"},
		{"sys-keep", "role-keep", "sonnet"},
	}
	for _, s := range sysAgents {
		if _, err := sqlDB.Exec(
			`INSERT INTO system_agent_definitions (id, role, model, timeout, prompt, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			s.id, s.role, s.model, 60, "p", now, now); err != nil {
			t.Fatalf("insert system_agent_def %q: %v", s.id, err)
		}
	}

	migrateTo(t, m, 156)

	wantModel := map[string]string{
		"legacy53":  "codex_gpt56_sol_normal",
		"legacy53h": "codex_gpt56_sol_high",
		"gen54":     "codex_gpt56_sol_normal",
		"gen55":     "codex_gpt56_sol_high",
		"gen55mini": "codex_gpt56_luna_low",
		"untouched": "sonnet",
	}
	for id, want := range wantModel {
		var got string
		if err := sqlDB.QueryRow(
			`SELECT model FROM agent_definitions WHERE id = ? AND project_id = ? AND workflow_id = ?`,
			id, "proj-156", "wf-156").Scan(&got); err != nil {
			t.Fatalf("scan agent_def %q: %v", id, err)
		}
		if got != want {
			t.Errorf("agent_def %q: model = %q, want %q", id, got, want)
		}
	}

	wantLC := map[string]string{
		"gen54":     "codex_gpt56_luna_low",
		"gen55":     "codex_gpt56_sol_normal",
		"untouched": "haiku",
	}
	for id, want := range wantLC {
		var got string
		if err := sqlDB.QueryRow(
			`SELECT low_consumption_model FROM agent_definitions WHERE id = ? AND project_id = ? AND workflow_id = ?`,
			id, "proj-156", "wf-156").Scan(&got); err != nil {
			t.Fatalf("scan agent_def lc %q: %v", id, err)
		}
		if got != want {
			t.Errorf("agent_def %q: low_consumption_model = %q, want %q", id, got, want)
		}
	}

	wantSys := map[string]string{
		"sys-53":   "codex_gpt56_sol_high",
		"sys-55":   "codex_gpt56_sol_normal",
		"sys-keep": "sonnet",
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

	// The pre-5.6 cli_models rows stay as selectable built-ins (000144 precedent).
	for _, id := range []string{"codex_gpt_normal", "codex_gpt54_normal", "codex_gpt55_normal"} {
		var count int
		if err := sqlDB.QueryRow(
			`SELECT COUNT(*) FROM cli_models WHERE id = ?`, id).Scan(&count); err != nil {
			t.Fatalf("count cli_models %q: %v", id, err)
		}
		if count != 1 {
			t.Errorf("cli_models %q: count = %d, want 1 (kept as built-in)", id, count)
		}
	}
}
