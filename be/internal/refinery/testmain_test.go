package refinery

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"be/internal/db"
)

// foldConsoleOnce drives Manager.foldConsole with a fresh, throwaway
// consoleSession — the test-seam replacement for the deleted Manager.fold —
// so tests that only exercise the event-line-driven console fold (no
// agent_messages delta) don't need to track a consoleSession across calls.
func foldConsoleOnce(ctx context.Context, mgr *Manager, sessionID, projectID string, events []string) {
	mgr.foldConsole(ctx, &consoleSession{}, sessionID, projectID, events)
}

// refineryTemplateDBPath holds the path to a pre-migrated DB created once by
// TestMain. Every test copies this file instead of running all migrations
// per test (be/CLAUDE.md: DB tests never migrate per-test).
var refineryTemplateDBPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "refinery-template-*")
	if err != nil {
		panic("create template dir: " + err.Error())
	}
	refineryTemplateDBPath = filepath.Join(dir, "template.db")
	d, err := db.OpenPath(refineryTemplateDBPath)
	if err != nil {
		panic("create template DB: " + err.Error())
	}
	d.Close()

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// newTestPool copies the pre-migrated template and opens a pool without
// running migrations.
func newTestPool(t *testing.T) *db.Pool {
	t.Helper()
	dest := filepath.Join(t.TempDir(), "test.db")
	if err := copyDBFile(refineryTemplateDBPath, dest); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.NewPoolPathExisting(dest, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open test pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func copyDBFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// seedConsoleChatSession inserts the minimal projects + agent_sessions rows a
// refinery_digests row's FK (console_session_id -> agent_sessions.id) needs,
// mirroring console.ChatService.create's console_chat row shape.
// seedConsoleChatSession seeds a chat with context_left=50 — below the
// console fold-start threshold (default 75), so the fold gate is open and
// debounce/trigger tests exercise actual folds.
func seedConsoleChatSession(t *testing.T, pool *db.Pool, sessionID, projectID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		projectID, projectID, now, now,
	); err != nil {
		t.Fatalf("seed project %s: %v", projectID, err)
	}
	if _, err := pool.Exec(
		`INSERT INTO agent_sessions (id, project_id, ticket_id, phase, node_id, agent_type, status, kind, context_left, created_at, updated_at)
		 VALUES (?, ?, '', 'console_chat', 'console_chat', 'console_chat', 'user_interactive', 'console_chat', 50, ?, ?)`,
		sessionID, projectID, now, now,
	); err != nil {
		t.Fatalf("seed console_chat session %s: %v", sessionID, err)
	}
}

// waitForCondition polls check until it returns true or timeout elapses,
// failing the test on timeout. Used for the sidecar's async goroutine-driven
// fold; yields via runtime.Gosched (never time.Sleep, per Rule 4).
func waitForCondition(t *testing.T, timeout time.Duration, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		runtime.Gosched()
	}
	t.Fatal("condition not met within timeout")
}

// settle yields to other goroutines for d without ever failing the test —
// used to give a sidecar goroutine a chance to (incorrectly) act before
// asserting a negative ("this must NOT have happened yet").
func settle(d time.Duration) {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		runtime.Gosched()
	}
}
