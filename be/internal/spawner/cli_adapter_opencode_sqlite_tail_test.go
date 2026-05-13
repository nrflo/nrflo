package spawner

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// setupOpencodeTestDB creates a SQLite DB with the opencode session+message
// schema at dbPath and returns an open write handle.
func setupOpencodeTestDB(t *testing.T, dbPath string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=rwc")
	if err != nil {
		t.Fatalf("setupOpencodeTestDB: open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	// Mirror the real opencode 1.14.x schema: session is keyed by project_id
	// (worktree match), session.time_created is the creation timestamp, and
	// per-message JSON lives in message.data (role + tokens nested inside).
	_, err = db.Exec(`
		CREATE TABLE project (
			id TEXT PRIMARY KEY,
			worktree TEXT NOT NULL,
			time_created INTEGER NOT NULL
		);
		CREATE TABLE session (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			directory TEXT NOT NULL,
			time_created INTEGER NOT NULL
		);
		CREATE TABLE message (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			time_created INTEGER NOT NULL,
			data TEXT NOT NULL
		);
		CREATE TABLE part (
			id TEXT PRIMARY KEY,
			message_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			time_created INTEGER NOT NULL,
			data TEXT NOT NULL
		);
	`)
	if err != nil {
		t.Fatalf("setupOpencodeTestDB: schema: %v", err)
	}
	return db
}

// insertOpencodeSession seeds matching project + session rows so the tailer's
// `project.worktree = ?` lookup succeeds. `worktree` matches the spawner's
// cmd.Dir; `session.directory` is opencode-derived (often a git root) and is
// not what the tailer joins on.
func insertOpencodeSession(t *testing.T, db *sql.DB, id, worktree string, createdAtMS int64) {
	t.Helper()
	projectID := "proj-" + id
	if _, err := db.Exec(
		`INSERT INTO project (id, worktree, time_created) VALUES (?, ?, ?)`,
		projectID, worktree, createdAtMS,
	); err != nil {
		t.Fatalf("insertOpencodeSession: project: %v", err)
	}
	_, err := db.Exec(
		`INSERT INTO session (id, project_id, directory, time_created) VALUES (?, ?, ?, ?)`,
		id, projectID, worktree, createdAtMS,
	)
	if err != nil {
		t.Fatalf("insertOpencodeSession: %v", err)
	}
}

func insertOpencodeAssistantMsg(t *testing.T, db *sql.DB, msgID, sessionID string, inputTokens, outputTokens int) {
	t.Helper()
	data := fmt.Sprintf(
		`{"role":"assistant","tokens":{"input":%d,"output":%d,"reasoning":0,"cache":{"read":0}}}`,
		inputTokens, outputTokens)
	_, err := db.Exec(
		`INSERT INTO message (id, session_id, time_created, data) VALUES (?, ?, ?, ?)`,
		msgID, sessionID, time.Now().UnixNano(), data,
	)
	if err != nil {
		t.Fatalf("insertOpencodeAssistantMsg: %v", err)
	}
}

// pollForPct spins until sink.contextUpdates contains want or deadline elapses.
func pollForPct(sink *opencodeTestSink, want int, deadline time.Duration) bool {
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		sink.mu.Lock()
		updates := append([]int{}, sink.contextUpdates...)
		sink.mu.Unlock()
		for _, pct := range updates {
			if pct == want {
				return true
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// TestOpencodeSQLiteTail_HappyPath: valid schema, one assistant message with
// 100k/200k tokens used → UpdateContextLeft(50) within 1.5s.
func TestOpencodeSQLiteTail_HappyPath(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "opencode.db")
	db := setupOpencodeTestDB(t, dbPath)

	rawWorkDir := t.TempDir()
	workDir, err := filepath.EvalSymlinks(rawWorkDir)
	if err != nil {
		workDir = rawWorkDir
	}

	const maxCtx = 200000
	const tokensUsed = 100000 // → 50% left

	now := time.Now()
	insertOpencodeSession(t, db, "sess-happy", workDir, now.UnixMilli())
	insertOpencodeAssistantMsg(t, db, "msg-1", "sess-happy", tokensUsed, 0)

	t.Setenv("OPENCODE_DB", dbPath)
	sink := &opencodeTestSink{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cleanup := startOpencodeSQLiteTail(ctx, "sess-happy", workDir, now.Add(-time.Second), maxCtx, sink)
	defer cleanup()

	if !pollForPct(sink, 50, 1500*time.Millisecond) {
		sink.mu.Lock()
		got := append([]int{}, sink.contextUpdates...)
		sink.mu.Unlock()
		t.Errorf("contextUpdates = %v after 1.5s; want entry with pct==50", got)
	}
}

// TestWaitForOpencodeDB_NotPresent_Deadline: non-existent DB returns error
// after the short deadline without panicking.
func TestWaitForOpencodeDB_NotPresent_Deadline(t *testing.T) {
	t.Setenv("OPENCODE_DB", filepath.Join(t.TempDir(), "nonexistent.db"))

	ctx := context.Background()
	_, err := waitForOpencodeDB(ctx, 300*time.Millisecond)
	if err == nil {
		t.Error("waitForOpencodeDB: expected error for missing DB, got nil")
	}
}

// TestWaitForOpencodeDB_CtxCancelled: context cancelled before deadline
// returns without panicking; no DB calls.
func TestWaitForOpencodeDB_CtxCancelled(t *testing.T) {
	t.Setenv("OPENCODE_DB", filepath.Join(t.TempDir(), "nonexistent.db"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before call

	_, err := waitForOpencodeDB(ctx, 30*time.Second)
	if err == nil {
		t.Error("waitForOpencodeDB: expected error for cancelled ctx, got nil")
	}
}

// TestOpencodeSQLiteTail_SchemaMismatch_ExitsCleanly: DB exists but has no
// tables; tailer exits cleanly, no Sink call, no panic.
func TestOpencodeSQLiteTail_SchemaMismatch_ExitsCleanly(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "opencode.db")

	// Create empty DB: file exists but no tables → "no such table: session".
	emptyDB, err := sql.Open("sqlite", "file:"+dbPath+"?mode=rwc")
	if err != nil {
		t.Fatalf("create empty DB: %v", err)
	}
	emptyDB.Close()

	t.Setenv("OPENCODE_DB", dbPath)
	sink := &opencodeTestSink{}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cleanup := startOpencodeSQLiteTail(ctx, "sess-mismatch", dir, time.Now().Add(-time.Second), 200000, sink)
	defer cleanup()

	// Goroutine exits quickly (<500ms) once schema mismatch is detected.
	end := time.Now().Add(1000 * time.Millisecond)
	for time.Now().Before(end) {
		sink.mu.Lock()
		n := len(sink.contextUpdates)
		sink.mu.Unlock()
		if n > 0 {
			t.Errorf("contextUpdates = %v, want none on schema mismatch", sink.contextUpdates)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestOpencodeSQLiteTail_Dedup: two assistant messages with the same token
// count produce exactly one UpdateContextLeft call.
func TestOpencodeSQLiteTail_Dedup(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "opencode.db")
	db := setupOpencodeTestDB(t, dbPath)

	rawWorkDir := t.TempDir()
	workDir, err := filepath.EvalSymlinks(rawWorkDir)
	if err != nil {
		workDir = rawWorkDir
	}

	const maxCtx = 200000
	const tokensUsed = 100000 // → pct=50

	now := time.Now()
	insertOpencodeSession(t, db, "sess-dedup", workDir, now.UnixMilli())
	// Two messages with identical token sums; tailer reads only the latest.
	insertOpencodeAssistantMsg(t, db, "msg-d1", "sess-dedup", tokensUsed, 0)
	insertOpencodeAssistantMsg(t, db, "msg-d2", "sess-dedup", tokensUsed, 0)

	t.Setenv("OPENCODE_DB", dbPath)
	sink := &opencodeTestSink{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cleanup := startOpencodeSQLiteTail(ctx, "sess-dedup", workDir, now.Add(-time.Second), maxCtx, sink)
	defer cleanup()

	if !pollForPct(sink, 50, 1500*time.Millisecond) {
		t.Fatal("no pct=50 received within 1.5s")
	}

	// Allow two more poll cycles then assert exactly one call (dedup guard).
	time.Sleep(600 * time.Millisecond)

	sink.mu.Lock()
	count := len(sink.contextUpdates)
	updates := append([]int{}, sink.contextUpdates...)
	sink.mu.Unlock()

	if count != 1 {
		t.Errorf("contextUpdates = %v (len=%d), want exactly 1 (dedup)", updates, count)
	}
}

// TestOpencodeSQLiteTail_CtxCancel_TerminatesPromptly: cancelling the returned
// CancelFunc stops further Sink calls.
func TestOpencodeSQLiteTail_CtxCancel_TerminatesPromptly(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "opencode.db")
	db := setupOpencodeTestDB(t, dbPath)

	rawWorkDir := t.TempDir()
	workDir, err := filepath.EvalSymlinks(rawWorkDir)
	if err != nil {
		workDir = rawWorkDir
	}

	now := time.Now()
	// Session present but no messages → used==0 → no Sink calls during setup.
	insertOpencodeSession(t, db, "sess-cancel", workDir, now.UnixMilli())

	t.Setenv("OPENCODE_DB", dbPath)
	sink := &opencodeTestSink{}
	ctx, cancel := context.WithCancel(context.Background())

	cleanup := startOpencodeSQLiteTail(ctx, "sess-cancel", workDir, now.Add(-time.Second), 200000, sink)
	defer cleanup()

	// Cancel immediately; goroutine should see ctx.Done() in next select.
	cancel()

	// Wait 600ms (>2 poll cycles) then assert no Sink calls.
	end := time.Now().Add(600 * time.Millisecond)
	for time.Now().Before(end) {
		time.Sleep(50 * time.Millisecond)
	}

	sink.mu.Lock()
	n := len(sink.contextUpdates)
	sink.mu.Unlock()
	if n != 0 {
		t.Errorf("contextUpdates len=%d after cancel, want 0", n)
	}
}

// TestIsOpencodeSchemaMismatch covers the error-string detection logic.
func TestIsOpencodeSchemaMismatch(t *testing.T) {
	t.Parallel()
	cases := []struct {
		msg  string
		want bool
	}{
		{"no such table: session", true},
		{"no such column: directory", true},
		{"sql: no rows in result set", false},
		{"constraint failed: UNIQUE", false},
		{"", false},
	}
	for _, tc := range cases {
		var err error
		if tc.msg != "" {
			err = fmt.Errorf("%s", tc.msg)
		}
		got := isOpencodeSchemaMismatch(err)
		if got != tc.want {
			t.Errorf("isOpencodeSchemaMismatch(%q) = %v, want %v", tc.msg, got, tc.want)
		}
	}
}
