package usagelimits

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"be/internal/logger"
)

// FetchAll scrapes usage limits from Claude and Codex CLIs concurrently via PTY.
// The context can be cancelled to abort in-flight PTY sessions (e.g., on server shutdown).
func FetchAll(ctx context.Context) *UsageLimits {
	result := &UsageLimits{FetchedAt: time.Now()}
	env := filteredEnv()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); result.Claude = fetchClaude(ctx, env) }()
	go func() { defer wg.Done(); result.Codex = fetchCodex(ctx, env) }()
	wg.Wait()

	logger.Info(ctx, "usage-limits: FetchAll done",
		"claude_available", result.Claude.Available,
		"claude_error", result.Claude.Error,
		"codex_available", result.Codex.Available,
		"codex_error", result.Codex.Error,
	)
	return result
}

func fetchClaude(ctx context.Context, env []string) ToolUsage {
	if _, err := exec.LookPath("claude"); err != nil {
		return ToolUsage{Available: false}
	}
	logger.Info(ctx, "usage-limits: scraping claude")

	sess, err := startPTY("claude", env)
	if err != nil {
		return ToolUsage{Available: true, Error: "spawn error: " + err.Error()}
	}
	defer sess.close()

	// Wait for prompt ready ("Ctx:" appears in the status line); 25s for cold starts.
	if !sess.waitFor(ctx, []string{"Ctx:"}, 25*time.Second) && ctx.Err() != nil {
		return ToolUsage{Available: false}
	}
	// Ensure screen is stable before typing the command.
	sess.waitIdle(ctx, 400*time.Millisecond, 3*time.Second)

	// Type /usage; wait for autocomplete to settle, then press Enter.
	sess.send("/usage")
	sess.waitIdle(ctx, 300*time.Millisecond, 2*time.Second)
	sess.send("\r")

	// Wait for usage data to render, then a stable frame.
	sess.waitFor(ctx, []string{"resets", "Resets"}, 20*time.Second)
	sess.waitIdle(ctx, 500*time.Millisecond, 3*time.Second)

	sess.send("/exit\r")
	sleepCtx(ctx, 500*time.Millisecond) //nolint:errcheck

	session, weekly := parseClaude(sess.output())
	if session == nil && weekly == nil {
		return ToolUsage{Available: true, Error: "failed to parse /usage output"}
	}
	return ToolUsage{Available: true, Session: session, Weekly: weekly}
}

func fetchCodex(ctx context.Context, env []string) ToolUsage {
	if _, err := exec.LookPath("codex"); err != nil {
		return ToolUsage{Available: false}
	}
	logger.Info(ctx, "usage-limits: scraping codex")

	sess, err := startPTY("codex", env)
	if err != nil {
		return ToolUsage{Available: true, Error: "spawn error: " + err.Error()}
	}
	defer sess.close()

	// Wait for prompt ready: "context left" is typical but allow fallback to any
	// stable idle frame (Codex may vary wording between versions).
	// 25s gives room for cold start, auth restore, and slow networks.
	if !sess.waitFor(ctx, []string{"context left", "Context left"}, 25*time.Second) {
		if ctx.Err() != nil {
			return ToolUsage{Available: false}
		}
		// Prompt signal not seen — wait for a stable frame anyway, then try.
		sess.waitIdle(ctx, 600*time.Millisecond, 5*time.Second)
	} else {
		// Prompt seen — wait for screen to stop repainting before typing.
		sess.waitIdle(ctx, 400*time.Millisecond, 3*time.Second)
	}

	// Type /status; wait for autocomplete to settle, then press Enter.
	sess.send("/status")
	sess.waitIdle(ctx, 300*time.Millisecond, 2*time.Second)
	sess.send("\r")

	// Wait for limits data, then a stable frame to ensure full render.
	sess.waitFor(ctx, []string{"% left", "% used", "% remaining"}, 15*time.Second)
	sess.waitIdle(ctx, 500*time.Millisecond, 3*time.Second)

	// Exit via Ctrl+C — codex panics on /exit due to a Rust wrapping bug.
	sess.send("\x03")
	sleepCtx(ctx, 500*time.Millisecond) //nolint:errcheck

	session, weekly := parseCodex(sess.output())
	if session == nil && weekly == nil {
		return ToolUsage{Available: true, Error: "failed to parse /status output"}
	}
	return ToolUsage{Available: true, Session: session, Weekly: weekly}
}

// sleepCtx sleeps for d or returns false early if ctx is cancelled.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	select {
	case <-time.After(d):
		return true
	case <-ctx.Done():
		return false
	}
}

// filteredEnv returns os.Environ() with CLAUDECODE removed.
func filteredEnv() []string {
	env := os.Environ()
	filtered := make([]string, 0, len(env))
	for _, e := range env {
		if strings.HasPrefix(e, "CLAUDECODE") {
			continue
		}
		filtered = append(filtered, e)
	}
	return filtered
}
