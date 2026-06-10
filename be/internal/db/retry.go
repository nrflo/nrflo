package db

import (
	"strings"
	"time"
)

// IsBusy reports whether err is a SQLite contention error worth retrying.
// Covers both SQLITE_BUSY (code 5) and SQLITE_BUSY_SNAPSHOT (517), which the
// modernc.org/sqlite driver surfaces as 'database is locked' strings.
func IsBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "database is locked") || strings.Contains(msg, "SQLITE_BUSY")
}

// busySleep is the inter-attempt sleep, swappable in tests.
var busySleep = time.Sleep

// WithBusyRetry runs fn in a retry loop. On SQLite contention errors fn is
// re-invoked with a small linear backoff up to maxAttempts. fn must be
// self-contained: each attempt opens, and commits or rolls back, its own
// transaction. With _txlock=immediate in the DSN every attempt already waits
// up to busy_timeout for the write lock, so this is a backstop for the rare
// residue (e.g. checkpoint contention), not the primary defense.
func WithBusyRetry(fn func() error) error {
	const maxAttempts = 5
	var err error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		err = fn()
		if !IsBusy(err) {
			return err
		}
		busySleep(time.Duration(10*(attempt+1)) * time.Millisecond)
	}
	return err
}
