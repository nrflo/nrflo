package spawner

import (
	"context"
	"database/sql"
	"encoding/json"
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
// updates via sink. The returned cleanup func performs a final grace-probe
// (opencode flushes the filled assistant message lazily, sometimes after
// the agent already exited) before stopping the goroutine.
//
// DB path priority: $OPENCODE_DB → $XDG_DATA_HOME/opencode/opencode.db →
// ~/.local/share/opencode/opencode.db. On DB-not-yet-present (30s deadline)
// or schema mismatch, the goroutine exits cleanly without failing the run.
func startOpencodeSQLiteTail(ctx context.Context, sessionID, workDir string, startedAt time.Time, maxCtx int, sink Sink) context.CancelFunc {
	cctx, cancel := context.WithCancel(ctx)
	go opencodeSQLiteTailRun(cctx, sessionID, workDir, startedAt, maxCtx, sink)
	return func() {
		finalOpencodeTokensProbe(sessionID, workDir, startedAt, maxCtx, sink)
		cancel()
	}
}

// finalOpencodeTokensProbe runs at agent-exit time. Opens its own
// short-lived read-only handle and polls up to 1.5 s for the latest
// non-zero-usage assistant message, then flushes that into context_left
// via the sink. Idempotent w.r.t. the running tailer goroutine.
//
// Why this exists: opencode commits the assistant placeholder (tokens=0)
// up front and the filled row (real tokens) at end-of-turn. For very
// short workflows (single `nrflo agent finished` call), the filled row
// can land AFTER the agent process exits and the tailer goroutine has
// already been canceled. Without this final probe, `context_left`
// stays NULL on short opencode sessions.
func finalOpencodeTokensProbe(sessionID, workDir string, startedAt time.Time, maxCtx int, sink Sink) {
	dbPath, err := waitForOpencodeDB(context.Background(), 250*time.Millisecond)
	if err != nil {
		return
	}
	dsn := fmt.Sprintf("file:%s?mode=ro&_journal=WAL&_busy_timeout=2000", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return
	}
	defer db.Close()

	resolvedDir, err := filepath.EvalSymlinks(workDir)
	if err != nil {
		resolvedDir = workDir
	}

	probeCtx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	opencodeSessID, err := waitForOpencodeSession(probeCtx, db, resolvedDir, startedAt, 1500*time.Millisecond)
	if err != nil || opencodeSessID == "" {
		return
	}

	// Parts are emitted from the live tailer goroutine on a 250ms cadence;
	// we deliberately don't re-flush them here to avoid duplicating rows
	// the tailer already wrote. The token probe below handles the lazy
	// final-token flush — that's a single dedup'd field, not append-only.

	deadline := time.Now().Add(1500 * time.Millisecond)
	for {
		used, qerr := queryOpencodeTokensUsed(db, opencodeSessID)
		if qerr == nil && used > 0 {
			pct := ComputeContextLeftPct(used, maxCtx)
			_, _, _, _ = sink.UpdateContextLeft(sessionID, pct)
			return
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
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
	var partsCursor int64
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

		if next, perr := flushOpencodeParts(db, opencodeSessID, partsCursor, sessionID, sink); perr == nil {
			partsCursor = next
		} else if isOpencodeSchemaMismatch(perr) {
			log.Printf("opencode sqlite tail: parts schema mismatch: %v", perr)
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// flushOpencodeParts reads every `part` row created after `cursor` for the
// given opencode session, emits each as an agent_messages row via the sink
// (with category derived from the part JSON), and returns the new cursor.
//
// Mapping (part.data JSON `.type` field):
//   - "text"             → category="text",  content = $.text
//   - "tool"             → category="tool",  content = "<tool>: <input snippet>",
//                          payload = full part JSON (for the agent-saver to
//                          summarize tool use + outputs on context save)
//   - "reasoning"        → category="thinking", content = $.text
//   - "step-start" / "step-finish" / unknown → skipped (internal events)
//
// On each non-skipped emission the sink's BumpLastMessage is also called so
// stall detection stays accurate for opencode/cli_interactive (which has no
// hook channel of its own).
func flushOpencodeParts(db *sql.DB, opencodeSessID string, cursor int64, nrfloSessID string, sink Sink) (int64, error) {
	rows, err := db.Query(
		`SELECT p.time_created, p.data
		   FROM part p
		  WHERE p.session_id = ? AND p.time_created > ?
		  ORDER BY p.time_created ASC`,
		opencodeSessID, cursor)
	if err != nil {
		return cursor, err
	}
	defer rows.Close()

	next := cursor
	for rows.Next() {
		var ts int64
		var raw string
		if err := rows.Scan(&ts, &raw); err != nil {
			return cursor, err
		}
		if ts > next {
			next = ts
		}
		content, category, payload, ok := classifyOpencodePart(raw)
		if !ok {
			continue
		}
		_, _, _, _ = sink.RecordHookMessage(nrfloSessID, content, category, payload)
		sink.BumpLastMessage(nrfloSessID)
		sink.SetLastMessage(nrfloSessID, content)
	}
	if err := rows.Err(); err != nil {
		return cursor, err
	}
	return next, nil
}

// classifyOpencodePart parses one part.data JSON blob and returns
// (content, category, payload, ok). ok=false means skip this part.
//
// Truncates `content` to a sane length (1 KB) so the live log stays
// readable; `payload` preserves the full JSON for downstream consumers.
func classifyOpencodePart(raw string) (content, category, payload string, ok bool) {
	var probe struct {
		Type  string `json:"type"`
		Text  string `json:"text"`
		Tool  string `json:"tool"`
		State struct {
			Input map[string]any `json:"input"`
		} `json:"state"`
	}
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return "", "", "", false
	}
	switch probe.Type {
	case "text":
		if probe.Text == "" {
			return "", "", "", false
		}
		return truncate(probe.Text, 1024), "text", raw, true
	case "reasoning":
		if probe.Text == "" {
			return "", "", "", false
		}
		return truncate(probe.Text, 1024), "thinking", raw, true
	case "tool":
		head := probe.Tool
		if head == "" {
			head = "tool"
		}
		// Best-effort one-line summary of the input ("command" for bash,
		// otherwise the first scalar field). Detailed view stays in payload.
		summary := summarizeOpencodeToolInput(probe.State.Input)
		if summary != "" {
			head = head + ": " + summary
		}
		return truncate(head, 1024), "tool", raw, true
	default:
		return "", "", "", false
	}
}

func summarizeOpencodeToolInput(input map[string]any) string {
	if input == nil {
		return ""
	}
	for _, key := range []string{"command", "query", "path", "url", "pattern"} {
		if v, ok := input[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
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
// belonging to a project whose `worktree` matches the spawner's cmd.Dir
// (and time_created >= startedAt) appears, or deadline/ctx fires.
// `session.directory` is the resolved git-root path opencode discovers,
// which can differ from the spawn cwd; `project.worktree` is the raw
// cmd.Dir, so it matches reliably.
func waitForOpencodeSession(ctx context.Context, db *sql.DB, resolvedDir string, startedAt time.Time, deadline time.Duration) (string, error) {
	end := time.Now().Add(deadline)
	startedAtMS := startedAt.UnixMilli()
	for {
		row := db.QueryRowContext(ctx,
			`SELECT s.id
			   FROM session s
			   JOIN project p ON p.id = s.project_id
			  WHERE p.worktree = ? AND s.time_created >= ?
			  ORDER BY s.time_created DESC
			  LIMIT 1`,
			resolvedDir, startedAtMS)
		var id string
		if err := row.Scan(&id); err == nil {
			return id, nil
		} else if isOpencodeSchemaMismatch(err) {
			return "", fmt.Errorf("opencode sqlite tail: session query: %w", err)
		}
		if time.Now().After(end) {
			return "", fmt.Errorf("opencode sqlite tail: session for worktree=%s not found within %s", resolvedDir, deadline)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// queryOpencodeTokensUsed reads the most-recently-written assistant message
// with non-zero usage for the given opencode session. Each message stores
// its payload as a JSON blob in `message.data`; tokens live at
// `$.tokens.{input,output,reasoning,cache.read}` and the role at `$.role`.
//
// Opencode emits an empty assistant placeholder at the start of each turn
// and fills it in as the turn progresses; depending on when we sample we
// may see the placeholder *after* the filled row in the order they hit
// disk. Filtering on non-zero input tokens (or cache.read, which is set
// up-front for cache-hit turns) guarantees we read a real usage record.
func queryOpencodeTokensUsed(db *sql.DB, opencodeSessID string) (int, error) {
	row := db.QueryRow(
		`SELECT COALESCE(json_extract(data, '$.tokens.input'), 0) +
		        COALESCE(json_extract(data, '$.tokens.output'), 0) +
		        COALESCE(json_extract(data, '$.tokens.reasoning'), 0) +
		        COALESCE(json_extract(data, '$.tokens.cache.read'), 0) AS used
		   FROM message
		  WHERE session_id = ?
		    AND json_extract(data, '$.role') = 'assistant'
		    AND (COALESCE(json_extract(data, '$.tokens.input'), 0)
		       + COALESCE(json_extract(data, '$.tokens.cache.read'), 0)) > 0
		  ORDER BY time_created DESC
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
