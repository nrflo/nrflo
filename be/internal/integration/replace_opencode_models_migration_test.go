package integration

import "testing"

// TestMigration051OldOpencodeModelsRemoved verifies that migration 000051 deletes
// the two legacy opencode models (opencode_gpt_normal, opencode_gpt_high).
func TestMigration051OldOpencodeModelsRemoved(t *testing.T) {
	env := NewTestEnv(t)

	for _, id := range []string{"opencode_gpt_normal", "opencode_gpt_high"} {
		var count int
		if err := env.Pool.QueryRow(
			`SELECT COUNT(*) FROM cli_models WHERE id = ?`, id).Scan(&count); err != nil {
			t.Fatalf("query cli_models for %q: %v", id, err)
		}
		if count != 0 {
			t.Errorf("model %q should not exist after migration 000051, found %d row(s)", id, count)
		}
	}
}

// TestMigration051NewOpencodeModelsSeeded verifies that migration 000051 inserts
// the three replacement opencode models with correct cli_type, mapped_model, and read_only.
func TestMigration051NewOpencodeModelsSeeded(t *testing.T) {
	env := NewTestEnv(t)

	cases := []struct {
		id          string
		mappedModel string
		reasoning   string
	}{
		{"opencode_minimax_m25_free", "opencode/minimax-m2.5-free", ""},
		{"opencode_qwen36_plus_free", "opencode/qwen3.6-plus-free", ""},
		{"opencode_gpt54", "openai/gpt-5.4", "high"},
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			var cliType, mappedModel, reasoning string
			var readOnly int
			err := env.Pool.QueryRow(
				`SELECT cli_type, mapped_model, reasoning_effort, read_only FROM cli_models WHERE id = ?`,
				tc.id).Scan(&cliType, &mappedModel, &reasoning, &readOnly)
			if err != nil {
				t.Fatalf("SELECT cli_models WHERE id=%q: %v", tc.id, err)
			}
			if cliType != "opencode" {
				t.Errorf("cli_type = %q, want %q", cliType, "opencode")
			}
			if mappedModel != tc.mappedModel {
				t.Errorf("mapped_model = %q, want %q", mappedModel, tc.mappedModel)
			}
			if reasoning != tc.reasoning {
				t.Errorf("reasoning_effort = %q, want %q", reasoning, tc.reasoning)
			}
			if readOnly != 1 {
				t.Errorf("read_only = %d, want 1", readOnly)
			}
		})
	}
}

// TestMigration051TotalReadonlyModelCount verifies the read-only seeded model
// count after migrations 000043 + 000051 + 000057 (4 versioned Opus rows
// replace the 2 unversioned opus/opus_1m rows, net +2 → 13).
func TestMigration051TotalReadonlyModelCount(t *testing.T) {
	env := NewTestEnv(t)

	var count int
	if err := env.Pool.QueryRow(
		`SELECT COUNT(*) FROM cli_models WHERE read_only = 1`).Scan(&count); err != nil {
		t.Fatalf("count readonly cli_models: %v", err)
	}
	if count != 13 {
		t.Errorf("readonly model count = %d, want 13", count)
	}
}

// TestMigration051OpencodeModelsContextLength verifies that the new opencode models
// have the expected context_length of 200000.
func TestMigration051OpencodeModelsContextLength(t *testing.T) {
	env := NewTestEnv(t)

	for _, id := range []string{"opencode_minimax_m25_free", "opencode_qwen36_plus_free", "opencode_gpt54"} {
		var ctxLen int
		if err := env.Pool.QueryRow(
			`SELECT context_length FROM cli_models WHERE id = ?`, id).Scan(&ctxLen); err != nil {
			t.Fatalf("SELECT context_length for %q: %v", id, err)
		}
		if ctxLen != 200000 {
			t.Errorf("model %q: context_length = %d, want 200000", id, ctxLen)
		}
	}
}

// TestMigration051EnabledByDefault verifies that the new opencode models are enabled.
func TestMigration051EnabledByDefault(t *testing.T) {
	env := NewTestEnv(t)

	for _, id := range []string{"opencode_minimax_m25_free", "opencode_qwen36_plus_free", "opencode_gpt54"} {
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
