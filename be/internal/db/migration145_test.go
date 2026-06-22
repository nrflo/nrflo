package db

import (
	"testing"
)

// TestMigration145_AnthropicContext1M verifies the 4.6+ Anthropic api_models
// rows are corrected to a 1M context window, while Haiku 4.5 stays at 200k.
func TestMigration145_AnthropicContext1M(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	want := map[string]int{
		"opus_4_8": 1000000,
		"opus_4_7": 1000000,
		"opus_4_6": 1000000,
		"sonnet":   1000000,
		"haiku":    200000,
	}
	for id, exp := range want {
		var got int
		if err := pool.QueryRow(
			`SELECT context_length FROM api_models WHERE id = ? AND provider = 'anthropic'`, id,
		).Scan(&got); err != nil {
			t.Fatalf("query api_models %q: %v", id, err)
		}
		if got != exp {
			t.Errorf("api_models[%q].context_length = %d, want %d", id, got, exp)
		}
	}
}
