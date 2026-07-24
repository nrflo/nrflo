package repo

import (
	"database/sql"
	"testing"
)

// TestUpdateTimeBuckets_PersistsTimeBucketsJSON verifies UpdateTimeBuckets
// writes the nullable column via a targeted UPDATE (mirrors UpdateCost).
func TestUpdateTimeBuckets_PersistsTimeBucketsJSON(t *testing.T) {
	t.Parallel()
	database, r, wfiID := setupTokenTestDB(t)
	defer database.Close()

	insertSessionWithToken(t, database, "sess-timing", wfiID, "tok-timing", "running")

	bucketsJSON := `{"thinking_sec":1.5,"tool_arg_sec":2,"text_sec":0.5,"tool_wait_sec":3}`
	if err := r.UpdateTimeBuckets("sess-timing", bucketsJSON); err != nil {
		t.Fatalf("UpdateTimeBuckets: %v", err)
	}

	var got sql.NullString
	if err := database.QueryRow(`SELECT time_buckets_json FROM agent_sessions WHERE id = ?`, "sess-timing").
		Scan(&got); err != nil {
		t.Fatalf("query: %v", err)
	}
	if !got.Valid || got.String != bucketsJSON {
		t.Errorf("time_buckets_json = %+v, want %q", got, bucketsJSON)
	}
}

// TestUpdateTimeBuckets_UnknownSession_ReturnsNilError verifies
// UpdateTimeBuckets is a silent no-op on 0 rows affected (a session that
// ended before its first flush) rather than erroring.
func TestUpdateTimeBuckets_UnknownSession_ReturnsNilError(t *testing.T) {
	t.Parallel()
	database, r, _ := setupTokenTestDB(t)
	defer database.Close()

	if err := r.UpdateTimeBuckets("does-not-exist", `{}`); err != nil {
		t.Errorf("UpdateTimeBuckets on unknown session = %v, want nil error", err)
	}
}

// TestUpdateTimeBuckets_OverwritesPreviousSnapshot verifies a second call
// replaces (not merges) the previous time_buckets_json value.
func TestUpdateTimeBuckets_OverwritesPreviousSnapshot(t *testing.T) {
	t.Parallel()
	database, r, wfiID := setupTokenTestDB(t)
	defer database.Close()

	insertSessionWithToken(t, database, "sess-timing-2", wfiID, "tok-timing-2", "running")

	if err := r.UpdateTimeBuckets("sess-timing-2", `{"thinking_sec":1}`); err != nil {
		t.Fatalf("first UpdateTimeBuckets: %v", err)
	}
	if err := r.UpdateTimeBuckets("sess-timing-2", `{"thinking_sec":9}`); err != nil {
		t.Fatalf("second UpdateTimeBuckets: %v", err)
	}

	var got string
	if err := database.QueryRow(`SELECT time_buckets_json FROM agent_sessions WHERE id = ?`, "sess-timing-2").
		Scan(&got); err != nil {
		t.Fatalf("query: %v", err)
	}
	if got != `{"thinking_sec":9}` {
		t.Errorf("time_buckets_json = %q, want overwritten value", got)
	}
}
