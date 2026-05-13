package spawner

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// startOpencodeSQLiteTail launches a goroutine that discovers the opencode
// session in its SQLite DB and polls for token usage, pushing context_left
// updates via sink. Returns a cancel func that stops the goroutine.
//
// DB path priority: $OPENCODE_DB → $XDG_DATA_HOME/opencode/opencode.db →
// ~/.local/share/opencode/opencode.db. On DB-not-yet-present (30s deadline)
// or schema mismatch, the goroutine exits cleanly without failing the run.
func startOpencodeSQLiteTail(ctx context.Context, sessionID, workDir string, startedAt time.Time, maxCtx int, sink Sink) context.CancelFunc {
	cctx, cancel := context.WithCancel(ctx)
	go opencodeSQLiteTailRun(cctx, sessionID, workDir, startedAt, maxCtx, sink)
	return cancel
}

func opencodeSQLiteTailRun(ctx context.Context, sessionID, workDir string, startedAt time.Time, maxCtx int, sink Sink) {
	dbPath, err := waitForOpencodeDB(ctx, 30*time.Second)
	if err != nil {
		return
	}
	dsn := fmt.Sprintf("file:%s?mode=ro&_journal=WAL&_busy_timeout=2000", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		log.Printf("opencode sqlite tail: open DB: %v", err)
		return
	}
	defer db.Close()

	resolvedDir, err := filepath.EvalSymlinks(workDir)
	if err != nil {
		resolvedDir = workDir
	}

	opencodeSessID, err := waitForOpencodeSession(ctx, db, resolvedDir, startedAt, 30*time.Second)
	if err != nil {
		return
	}

	var lastPct int
	for {
		used, err := queryOpencodeTokensUsed(db, opencodeSessID)
		if err != nil {
			if isOpencodeSchemaMismatch(err) {
				log.Printf("opencode sqlite tail: schema mismatch: %v", err)
				return
			}
			// transient read error — keep polling
		} else if used > 0 {
			pct := ComputeContextLeftPct(used, maxCtx)
			if pct != lastPct {
				_, _, _, _ = sink.UpdateContextLeft(sessionID, pct)
				lastPct = pct
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// waitForOpencodeDB polls for the opencode SQLite DB file every 250ms until
// it exists or the deadline/ctx fires. Logs once when waiting starts.
func waitForOpencodeDB(ctx context.Context, deadline time.Duration) (string, error) {
	candidate := opencodeDBCandidate()
	end := time.Now().Add(deadline)
	logged := false
	for {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		if !logged {
			log.Printf("opencode sqlite tail: waiting for DB at %s", candidate)
			logged = true
		}
		if time.Now().After(end) {
			return "", fmt.Errorf("opencode sqlite tail: DB not found at %s within %s", candidate, deadline)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// opencodeDBCandidate returns the expected opencode SQLite DB path.
func opencodeDBCandidate() string {
	if v := os.Getenv("OPENCODE_DB"); v != "" {
		return v
	}
	xdg := os.Getenv("XDG_DATA_HOME")
	if xdg == "" {
		home, _ := os.UserHomeDir()
		xdg = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(xdg, "opencode", "opencode.db")
}

// waitForOpencodeSession polls the sessions table every 250ms until a row
// matching directory + created_at >= startedAt appears, or deadline/ctx fires.
func waitForOpencodeSession(ctx context.Context, db *sql.DB, resolvedDir string, startedAt time.Time, deadline time.Duration) (string, error) {
	end := time.Now().Add(deadline)
	startedAtMS := startedAt.UnixMilli()
	for {
		row := db.QueryRowContext(ctx,
			`SELECT id FROM session WHERE directory = ? AND created_at >= ? ORDER BY created_at DESC LIMIT 1`,
			resolvedDir, startedAtMS)
		var id string
		if err := row.Scan(&id); err == nil {
			return id, nil
		} else if isOpencodeSchemaMismatch(err) {
			return "", fmt.Errorf("opencode sqlite tail: session query: %w", err)
		}
		if time.Now().After(end) {
			return "", fmt.Errorf("opencode sqlite tail: session for dir=%s not found within %s", resolvedDir, deadline)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// queryOpencodeTokensUsed reads the latest assistant message's total token
// usage for the given opencode session. Tokens are stored as a JSON blob;
// we sum input + output + reasoning + cache.read.
func queryOpencodeTokensUsed(db *sql.DB, opencodeSessID string) (int, error) {
	row := db.QueryRow(
		`SELECT COALESCE(json_extract(tokens, '$.input'), 0) +
		        COALESCE(json_extract(tokens, '$.output'), 0) +
		        COALESCE(json_extract(tokens, '$.reasoning'), 0) +
		        COALESCE(json_extract(tokens, '$.cache.read'), 0)
		 FROM message
		 WHERE session_id = ? AND role = 'assistant'
		 ORDER BY rowid DESC
		 LIMIT 1`,
		opencodeSessID)
	var used int
	if err := row.Scan(&used); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return used, nil
}

// isOpencodeSchemaMismatch returns true when the error indicates a missing
// table or column — a schema the tailer cannot handle. On mismatch the tailer
// logs once and exits cleanly rather than crashing the run.
func isOpencodeSchemaMismatch(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no such table") || strings.Contains(msg, "no such column")
}
