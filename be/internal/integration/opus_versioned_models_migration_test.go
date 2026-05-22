package integration

import "testing"

// TestMigration057NewOpusModelsSeeded verifies that migration 000057 inserts
// the four versioned Opus models with correct cli_type, display_name,
// mapped_model, context_length, read_only, and enabled flags. (The removal of
// the legacy opus/opus_1m rows is covered by TestMigration057MigratesAgentDefModelColumns.)
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
