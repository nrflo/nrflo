package repo

import (
	"testing"
	"time"

	"be/internal/model"
)

func TestCountRunning_EmptyTable(t *testing.T) {
	_, r, _ := setupRunningTestDB(t)

	count, err := r.CountRunning()
	if err != nil {
		t.Fatalf("CountRunning() error: %v", err)
	}
	if count != 0 {
		t.Errorf("CountRunning() on empty table = %d, want 0", count)
	}
}

func TestCountRunning_OnlyCountsRunning(t *testing.T) {
	database, r, wfiID := setupRunningTestDB(t)
	defer database.Close()

	now := time.Now().UTC()
	insertRunningSession(t, database, "sess-running-1", wfiID, model.AgentSessionRunning, now.Add(-2*time.Minute))
	insertRunningSession(t, database, "sess-running-2", wfiID, model.AgentSessionRunning, now.Add(-1*time.Minute))
	insertRunningSession(t, database, "sess-completed", wfiID, model.AgentSessionCompleted, now.Add(-3*time.Minute))
	insertRunningSession(t, database, "sess-failed", wfiID, model.AgentSessionFailed, now.Add(-4*time.Minute))
	insertRunningSession(t, database, "sess-timeout", wfiID, model.AgentSessionTimeout, now.Add(-5*time.Minute))

	count, err := r.CountRunning()
	if err != nil {
		t.Fatalf("CountRunning() error: %v", err)
	}
	if count != 2 {
		t.Errorf("CountRunning() = %d, want 2", count)
	}
}

func TestCountRunning_MultipleRunning(t *testing.T) {
	database, r, wfiID := setupRunningTestDB(t)
	defer database.Close()

	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		insertRunningSession(t, database, newTestID(i), wfiID, model.AgentSessionRunning, now.Add(time.Duration(-i)*time.Minute))
	}

	count, err := r.CountRunning()
	if err != nil {
		t.Fatalf("CountRunning() error: %v", err)
	}
	if count != 5 {
		t.Errorf("CountRunning() = %d, want 5", count)
	}
}

func TestCountRunning_AllNonRunningStatuses(t *testing.T) {
	cases := []struct {
		name   string
		status model.AgentSessionStatus
	}{
		{"completed", model.AgentSessionCompleted},
		{"failed", model.AgentSessionFailed},
		{"timeout", model.AgentSessionTimeout},
		{"continued", model.AgentSessionContinued},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			database, r, wfiID := setupRunningTestDB(t)
			defer database.Close()

			insertRunningSession(t, database, "sess-1", wfiID, tc.status, time.Now().UTC())

			count, err := r.CountRunning()
			if err != nil {
				t.Fatalf("CountRunning() error: %v", err)
			}
			if count != 0 {
				t.Errorf("CountRunning() with status=%s = %d, want 0", tc.status, count)
			}
		})
	}
}

func TestCountRunning_SingleRunning(t *testing.T) {
	database, r, wfiID := setupRunningTestDB(t)
	defer database.Close()

	insertRunningSession(t, database, "sess-only", wfiID, model.AgentSessionRunning, time.Now().UTC())

	count, err := r.CountRunning()
	if err != nil {
		t.Fatalf("CountRunning() error: %v", err)
	}
	if count != 1 {
		t.Errorf("CountRunning() = %d, want 1", count)
	}
}

// newTestID generates a unique session ID for table-driven tests.
func newTestID(i int) string {
	return "sess-count-" + string(rune('a'+i))
}
