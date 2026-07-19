package spawner

import (
	"context"

	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
)

// maxInjectedContextBytes caps additionalContext injected into a console
// UserPromptSubmit hook response. Truncated on a UTF-8 rune boundary with a
// warning log rather than silently growing the hook payload unbounded.
const maxInjectedContextBytes = 8192

// WorkingSetInjector is the v1 ContextInjector provider: it renders the
// `working-set` injectable (default_templates) into console UserPromptSubmit
// additionalContext. An empty template renders to "" — fully backward-silent
// no-op — so this is safe to wire unconditionally.
type WorkingSetInjector struct {
	pool *db.Pool
}

// NewWorkingSetInjector constructs a WorkingSetInjector over the given pool.
func NewWorkingSetInjector(pool *db.Pool) *WorkingSetInjector {
	return &WorkingSetInjector{pool: pool}
}

// InjectUserPromptContext renders the working-set injectable for the given
// console session. Returns "" for any non-console session kind, an unknown
// session id, or an empty/missing template — never an error, since this
// feeds a best-effort hook response.
func (w *WorkingSetInjector) InjectUserPromptContext(ctx context.Context, sessionID, prompt string) string {
	var kind, projectID, ticketID string
	err := w.pool.QueryRowContext(ctx,
		`SELECT kind, project_id, ticket_id FROM agent_sessions WHERE id = ?`, sessionID,
	).Scan(&kind, &projectID, &ticketID)
	if err != nil {
		return ""
	}
	if kind != model.AgentSessionKindConsole && kind != model.AgentSessionKindConsoleChat {
		return ""
	}

	vars := map[string]string{
		"SESSION_ID": sessionID,
		"PROJECT":    projectID,
		"TICKET":     ticketID,
		"PROMPT":     prompt,
	}
	out := renderInjectable(ctx, w.pool, "working-set", vars)
	if out == "" {
		return ""
	}

	if len(out) > maxInjectedContextBytes {
		truncated := truncateUTF8(out, maxInjectedContextBytes)
		logger.Warn(ctx, "working-set injectable truncated", "session_id", sessionID, "original_bytes", len(out), "truncated_bytes", len(truncated))
		return truncated
	}
	return out
}

// truncateUTF8 cuts s to at most n bytes, backing off to the nearest UTF-8
// rune boundary so a multi-byte character is never split.
func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !isUTF8Boundary(s[n]) {
		n--
	}
	return s[:n]
}

// isUTF8Boundary reports whether b is not a UTF-8 continuation byte (10xxxxxx).
func isUTF8Boundary(b byte) bool {
	return b&0xC0 != 0x80
}
