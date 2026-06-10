package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// TestBuildDSN_TxLockImmediate proves _txlock=immediate is honored by the
// driver: Begin() must take the write lock up front, so a second connection
// with busy_timeout(0) fails its own Begin() immediately instead of both
// transactions starting deferred and racing at first write.
func TestBuildDSN_TxLockImmediate(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "txlock.db")

	d1, err := sql.Open("sqlite", buildDSN(path))
	if err != nil {
		t.Fatalf("open d1: %v", err)
	}
	defer d1.Close()

	tx1, err := d1.Begin()
	if err != nil {
		t.Fatalf("begin tx1: %v", err)
	}
	defer tx1.Rollback() //nolint:errcheck

	// Second handle: immediate txlock but zero busy_timeout so the failure is
	// instant rather than waiting out the 10s production timeout.
	d2, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout%280%29&_txlock=immediate")
	if err != nil {
		t.Fatalf("open d2: %v", err)
	}
	defer d2.Close()

	tx2, err := d2.Begin()
	if err == nil {
		tx2.Rollback() //nolint:errcheck
		t.Fatal("second Begin() succeeded — _txlock=immediate is not in effect (deferred Begin takes no lock)")
	}
	if !IsBusy(err) {
		t.Fatalf("second Begin() error = %v, want a busy/locked error", err)
	}
}
