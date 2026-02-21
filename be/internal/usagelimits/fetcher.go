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

	// Wait for prompt ready ("Ctx:" appears in the status line)
	if !sess.waitFor(ctx, []string{"Ctx:"}, 10*time.Second) && ctx.Err() != nil {
		return ToolUsage{Available: false}
	}

	// Type /usage; wait for autocomplete to settle, then press Enter
	sess.send("/usage")
	if !sleepCtx(ctx, 1500*time.Millisecond) {
		return ToolUsage{Available: false}
	}
	sess.send("\r")

	// Wait for usage data to render
	sess.waitFor(ctx, []string{"resets", "Resets"}, 20*time.Second)
	if !sleepCtx(ctx, 2*time.Second) {
		return ToolUsage{Available: false}
	}

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

	// Wait for prompt ready ("context left" appears in status bar)
	if !sess.waitFor(ctx, []string{"context left"}, 10*time.Second) && ctx.Err() != nil {
		return ToolUsage{Available: false}
	}

	// Type /status; wait for autocomplete to settle, then press Enter
	sess.send("/status")
	if !sleepCtx(ctx, 1500*time.Millisecond) {
		return ToolUsage{Available: false}
	}
	sess.send("\r")

	// Wait for limits data to load
	sess.waitFor(ctx, []string{"% left", "% used"}, 12*time.Second)
	if !sleepCtx(ctx, 1*time.Second) {
		return ToolUsage{Available: false}
	}

	// Exit via Ctrl+C — codex panics on /exit due to a Rust wrapping bug
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
