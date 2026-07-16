package db

import (
	"testing"
)

// TestMigration145_AnthropicContext1M verifies the 4.6+ Anthropic model
// rows are corrected to a 1M context window, while Haiku 4.5 stays at 200k.
func TestMigration145_AnthropicContext1M(t *testing.T) {
	pool, err := newMigratedTestPool(t)
	if err != nil {
		t.Fatalf("newMigratedTestPool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	want := map[string]int{
		"opus-4-8":  1000000,
		"opus-4-7":  1000000,
		"opus-4-6":  1000000,
		"sonnet-5":  1000000,
		"haiku-4-5": 200000,
	}
	for id, exp := range want {
		var got int
		if err := pool.QueryRow(
			`SELECT api_context FROM models WHERE id = ? AND provider = 'anthropic'`, id,
		).Scan(&got); err != nil {
			t.Fatalf("query models %q: %v", id, err)
		}
		if got != exp {
			t.Errorf("models[%q].api_context = %d, want %d", id, got, exp)
		}
	}
}
