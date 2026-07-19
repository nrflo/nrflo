package spawner

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"be/internal/model"
)

// insertConsoleSession inserts a raw agent_sessions row with the given kind.
// agent_sessions has no FK on project_id/ticket_id, so callers may pass
// arbitrary values without seeding a ticket/workflow.
func insertConsoleSession(t *testing.T, env *spawnerTestEnv, sessionID, kind, ticketID string) {
	t.Helper()
	_, err := env.pool.Exec(`
		INSERT INTO agent_sessions (id, project_id, ticket_id, phase, agent_type, status, kind, created_at, updated_at)
		VALUES (?, ?, ?, 'analyzer', 'test-agent', 'running', ?, datetime('now'), datetime('now'))
	`, sessionID, env.project, ticketID, kind)
	if err != nil {
		t.Fatalf("failed to insert agent session: %v", err)
	}
}

// setWorkingSetTemplate overwrites the seeded `working-set` injectable body.
func setWorkingSetTemplate(t *testing.T, env *spawnerTestEnv, body string) {
	t.Helper()
	_, err := env.pool.Exec(`UPDATE default_templates SET template = ? WHERE id = 'working-set'`, body)
	if err != nil {
		t.Fatalf("failed to update working-set template: %v", err)
	}
}

func TestWorkingSetInjector_ConsoleChat_RendersAndSubstitutes(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	setWorkingSetTemplate(t, env, "session=${SESSION_ID} project=${PROJECT} ticket=${TICKET} prompt=${PROMPT}")
	insertConsoleSession(t, env, "sess-cc-1", model.AgentSessionKindConsoleChat, "TICK-1")

	inj := NewWorkingSetInjector(env.pool)
	got := inj.InjectUserPromptContext(context.Background(), "sess-cc-1", "hello there")

	want := "session=sess-cc-1 project=" + env.project + " ticket=TICK-1 prompt=hello there"
	if got != want {
		t.Errorf("InjectUserPromptContext = %q, want %q", got, want)
	}
}

func TestWorkingSetInjector_KindVariants(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	setWorkingSetTemplate(t, env, "digest for ${SESSION_ID}")

	cases := []struct {
		name      string
		kind      string
		wantEmpty bool
	}{
		{"console_chat renders", model.AgentSessionKindConsoleChat, false},
		{"console (mcp-external) renders", model.AgentSessionKindConsole, false},
		{"workflow_agent is a no-op", model.AgentSessionKindWorkflowAgent, true},
		{"observer is a no-op", model.AgentSessionKindObserver, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sessionID := "sess-kind-" + tc.kind
			insertConsoleSession(t, env, sessionID, tc.kind, "TICK-KIND")

			inj := NewWorkingSetInjector(env.pool)
			got := inj.InjectUserPromptContext(context.Background(), sessionID, "p")

			if tc.wantEmpty {
				if got != "" {
					t.Errorf("kind=%s: InjectUserPromptContext = %q, want empty", tc.kind, got)
				}
			} else if got == "" {
				t.Errorf("kind=%s: InjectUserPromptContext = empty, want non-empty rendering", tc.kind)
			}
		})
	}
}

func TestWorkingSetInjector_UnknownSession_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	setWorkingSetTemplate(t, env, "digest for ${SESSION_ID}")

	inj := NewWorkingSetInjector(env.pool)
	got := inj.InjectUserPromptContext(context.Background(), "sess-does-not-exist", "p")
	if got != "" {
		t.Errorf("InjectUserPromptContext(unknown session) = %q, want empty", got)
	}
}

func TestWorkingSetInjector_EmptyTemplate_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	env := newSpawnerTestEnv(t)
	// Migration 000176 seeds the working-set row with an empty body — leave it as-is.
	insertConsoleSession(t, env, "sess-empty-tpl", model.AgentSessionKindConsoleChat, "TICK-EMPTY")

	inj := NewWorkingSetInjector(env.pool)
	got := inj.InjectUserPromptContext(context.Background(), "sess-empty-tpl", "p")
	if got != "" {
		t.Errorf("InjectUserPromptContext(empty template) = %q, want empty (backward-silent no-op)", got)
	}
}

// TestWorkingSetInjector_TruncatesOnRuneBoundary builds a template whose byte
// 8192 lands inside a multi-byte rune (a 3-byte € straddling the cap) and
// verifies the result is <=8192 bytes, valid UTF-8 (no split rune), and that
// a truncation warning was logged.
func TestWorkingSetInjector_TruncatesOnRuneBoundary(t *testing.T) {
	env := newSpawnerTestEnv(t)
	prefix := strings.Repeat("a", maxInjectedContextBytes-2) // 8190 bytes
	body := prefix + "€" + "END"                             // € is 3 bytes: lands at 8190-8192
	setWorkingSetTemplate(t, env, body)
	insertConsoleSession(t, env, "sess-trunc", model.AgentSessionKindConsoleChat, "TICK-TRUNC")

	buf := captureLog(t)
	inj := NewWorkingSetInjector(env.pool)
	got := inj.InjectUserPromptContext(context.Background(), "sess-trunc", "p")

	if len(got) > maxInjectedContextBytes {
		t.Errorf("truncated length = %d, want <= %d", len(got), maxInjectedContextBytes)
	}
	if !utf8.ValidString(got) {
		t.Error("truncated result is not valid UTF-8 — a rune was split")
	}
	if strings.Contains(got, "END") {
		t.Error("truncated result should not contain content past the 8KB cap")
	}
	logOut := buf.String()
	if !strings.Contains(logOut, "WARN") || !strings.Contains(logOut, "truncated") {
		t.Errorf("expected a truncation warning to be logged, got: %s", logOut)
	}
	if !strings.Contains(logOut, "sess-trunc") {
		t.Errorf("expected truncation log to include session_id, got: %s", logOut)
	}
}

func TestTruncateUTF8_ShortStringUnchanged(t *testing.T) {
	t.Parallel()
	s := "short"
	if got := truncateUTF8(s, 100); got != s {
		t.Errorf("truncateUTF8(short string) = %q, want unchanged %q", got, s)
	}
}

func TestTruncateUTF8_NeverSplitsARune(t *testing.T) {
	t.Parallel()
	s := strings.Repeat("a", 5) + "€€€" // 5 + 9 = 14 bytes
	for n := 0; n <= len(s); n++ {
		got := truncateUTF8(s, n)
		if !utf8.ValidString(got) {
			t.Fatalf("truncateUTF8(%q, %d) = %q, not valid UTF-8", s, n, got)
		}
		if len(got) > n {
			t.Fatalf("truncateUTF8(%q, %d) = %q, longer than cap", s, n, got)
		}
	}
}
