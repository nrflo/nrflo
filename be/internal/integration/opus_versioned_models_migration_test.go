package integration

import "testing"

// TestMigration057OldOpusModelsRemoved verifies that migration 000057 deletes
// the two legacy unversioned opus rows (opus, opus_1m) from cli_models.
func TestMigration057OldOpusModelsRemoved(t *testing.T) {
	env := NewTestEnv(t)

	for _, id := range []string{"opus", "opus_1m"} {
		var count int
		if err := env.Pool.QueryRow(
			`SELECT COUNT(*) FROM cli_models WHERE id = ?`, id).Scan(&count); err != nil {
			t.Fatalf("query cli_models for %q: %v", id, err)
		}
		if count != 0 {
			t.Errorf("model %q should not exist after migration 000057, found %d row(s)", id, count)
		}
	}
}

// TestMigration057NewOpusModelsSeeded verifies that migration 000057 inserts
// the four versioned Opus models with correct cli_type, mapped_model, context_length.
func TestMigration057NewOpusModelsSeeded(t *testing.T) {
	env := NewTestEnv(t)

	cases := []struct {
		id            string
		displayName   string
		mappedModel   string
		contextLength int
	}{
		{"opus_4_6", "Opus 4.6", "claude-opus-4-6", 200000},
		{"opus_4_6_1m", "Opus 4.6 (1M)", "claude-opus-4-6[1m]", 1000000},
		{"opus_4_7", "Opus 4.7", "claude-opus-4-7", 200000},
		{"opus_4_7_1m", "Opus 4.7 (1M)", "claude-opus-4-7[1m]", 1000000},
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
			if cliType != "claude" {
				t.Errorf("cli_type = %q, want %q", cliType, "claude")
			}
			if displayName != tc.displayName {
				t.Errorf("display_name = %q, want %q", displayName, tc.displayName)
			}
			if mappedModel != tc.mappedModel {
				t.Errorf("mapped_model = %q, want %q", mappedModel, tc.mappedModel)
			}
			if reasoning != "" {
				t.Errorf("reasoning_effort = %q, want empty", reasoning)
			}
			if contextLen != tc.contextLength {
				t.Errorf("context_length = %d, want %d", contextLen, tc.contextLength)
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

// TestMigration057ClaudeCLIModelsExactSet verifies the final post-migration
// list of Claude CLI models is exactly the expected 6 (haiku, sonnet, and
// the four versioned opus rows) — no bare opus/opus_1m leaking through.
func TestMigration057ClaudeCLIModelsExactSet(t *testing.T) {
	env := NewTestEnv(t)

	rows, err := env.Pool.Query(
		`SELECT id FROM cli_models WHERE cli_type = 'claude' ORDER BY id`)
	if err != nil {
		t.Fatalf("query claude cli_models: %v", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan: %v", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}

	want := []string{"haiku", "opus_4_6", "opus_4_6_1m", "opus_4_7", "opus_4_7_1m", "sonnet"}
	if len(ids) != len(want) {
		t.Fatalf("claude cli_models count = %d (%v), want %d (%v)", len(ids), ids, len(want), want)
	}
	for i, id := range ids {
		if id != want[i] {
			t.Errorf("claude cli_models[%d] = %q, want %q (full list: %v)", i, id, want[i], ids)
		}
	}
}

// TestMigration057NoAgentDefsReferenceOldOpus verifies no agent_definitions or
// system_agent_definitions rows reference the removed opus/opus_1m IDs after
// migration, either in the model or low_consumption_model columns.
func TestMigration057NoAgentDefsReferenceOldOpus(t *testing.T) {
	env := NewTestEnv(t)

	queries := []struct {
		name  string
		query string
	}{
		{
			name: "agent_definitions.model",
			query: `SELECT COUNT(*) FROM agent_definitions
				WHERE model IN ('opus', 'opus_1m')`,
		},
		{
			name: "agent_definitions.low_consumption_model",
			query: `SELECT COUNT(*) FROM agent_definitions
				WHERE low_consumption_model IN ('opus', 'opus_1m')`,
		},
		{
			name: "system_agent_definitions.model",
			query: `SELECT COUNT(*) FROM system_agent_definitions
				WHERE model IN ('opus', 'opus_1m')`,
		},
	}

	for _, q := range queries {
		t.Run(q.name, func(t *testing.T) {
			var count int
			if err := env.Pool.QueryRow(q.query).Scan(&count); err != nil {
				t.Fatalf("%s: %v", q.name, err)
			}
			if count != 0 {
				t.Errorf("%s: %d row(s) still reference removed opus/opus_1m, want 0", q.name, count)
			}
		})
	}
}

// TestMigration057VersionedOpusEnabled verifies that the four versioned Opus
// models are enabled (not disabled) by default after migration.
func TestMigration057VersionedOpusEnabled(t *testing.T) {
	env := NewTestEnv(t)

	for _, id := range []string{"opus_4_6", "opus_4_6_1m", "opus_4_7", "opus_4_7_1m"} {
		var enabled int
		if err := env.Pool.QueryRow(
			`SELECT enabled FROM cli_models WHERE id = ?`, id).Scan(&enabled); err != nil {
			t.Fatalf("SELECT enabled for %q: %v", id, err)
		}
		if enabled != 1 {
			t.Errorf("model %q: enabled = %d, want 1", id, enabled)
		}
	}
}
