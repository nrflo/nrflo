package repo

import (
	"encoding/json"
	"sync"
	"testing"

	"be/internal/clock"
)

// TestFindingRepo_Upsert_ConcurrentWriters is a regression test for
// SQLITE_BUSY escaping to callers when parallel workflow runs hammer
// findings writes (read-then-write transactions on the same key). With
// _txlock=immediate writers queue on busy_timeout at Begin instead of
// failing the deferred snapshot upgrade.
func TestFindingRepo_Upsert_ConcurrentWriters(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	r := NewFindingRepo(pool, clock.Real())

	const writers = 8
	const writesPerWriter = 10
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < writesPerWriter; i++ {
				err := r.Upsert("session", "conc-sess", "shared-key", json.RawMessage(`{"v":1}`), Denorm{}, Actor{Source: "test"})
				if err != nil {
					t.Errorf("Upsert: %v", err)
				}
			}
		}()
	}
	wg.Wait()

	// Exactly one row, with every write serialized into write_count: a lost
	// first-insert race would show up as write_count < writers*writesPerWriter.
	var writeCount int
	if err := pool.QueryRow(
		`SELECT write_count FROM findings WHERE scope='session' AND scope_id='conc-sess' AND key='shared-key'`,
	).Scan(&writeCount); err != nil {
		t.Fatalf("read write_count: %v", err)
	}
	if writeCount != writers*writesPerWriter {
		t.Errorf("write_count = %d, want %d (lost updates under concurrency)", writeCount, writers*writesPerWriter)
	}
}
