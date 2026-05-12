package limitsrefresh

import (
	"context"
	"math/rand"
	"os/exec"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/logger"
	"be/internal/service"
)

const (
	tickInterval    = time.Hour
	freshnessWindow = 30 * time.Minute
	ctxTimeout      = 60 * time.Second
	startupJitter   = 30 * time.Second
	model           = "haiku"
	promptText      = `Reply with the single word "exit" and nothing else.`
)

// Runner is a background goroutine that calls the claude CLI hourly to refresh rate limit data.
type Runner struct {
	pool     *db.Pool
	clock    clock.Clock
	settings *service.GlobalSettingsService
	limits   *service.ClaudeLimitsService
	runFunc  func(ctx context.Context) error
}

// NewRunner constructs a Runner with the real exec-based runFunc.
func NewRunner(pool *db.Pool, clk clock.Clock) *Runner {
	r := &Runner{
		pool:     pool,
		clock:    clk,
		settings: service.NewGlobalSettingsService(pool, clk),
		limits:   service.NewClaudeLimitsService(pool, clk),
	}
	r.runFunc = r.execClaude
	return r
}

// Start spawns the background ticker goroutine.
func (r *Runner) Start(ctx context.Context) {
	go func() {
		jitter := time.Duration(rand.Int63n(int64(startupJitter)))
		select {
		case <-time.After(jitter):
		case <-ctx.Done():
			return
		}
		ticker := time.NewTicker(tickInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				r.tick(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (r *Runner) tick(ctx context.Context) {
	val, err := r.settings.Get("sync_claude_limits")
	if err != nil {
		logger.Info(ctx, "limitsrefresh: failed to read setting", "err", err)
		return
	}
	if val != "true" {
		return
	}

	limits, err := r.limits.Get()
	if err != nil {
		logger.Info(ctx, "limitsrefresh: failed to read limits", "err", err)
		return
	}
	if limits.UpdatedAt != "" {
		if updatedAt, parseErr := time.Parse(time.RFC3339, limits.UpdatedAt); parseErr == nil {
			if r.clock.Now().UTC().Sub(updatedAt) < freshnessWindow {
				logger.Info(ctx, "limitsrefresh: limits fresh, skipping", "updated_at", limits.UpdatedAt)
				return
			}
		}
	}

	runCtx, cancel := context.WithTimeout(ctx, ctxTimeout)
	defer cancel()
	if err := r.runFunc(runCtx); err != nil {
		logger.Info(ctx, "limitsrefresh: run failed", "err", err)
		return
	}
	logger.Info(ctx, "limitsrefresh: run completed")
}

func (r *Runner) execClaude(ctx context.Context) error {
	claudeBin, err := exec.LookPath("claude")
	if err != nil {
		logger.Info(ctx, "limitsrefresh: claude binary not found, skipping")
		return nil
	}
	return exec.CommandContext(ctx, claudeBin, "--model", model, "-p", promptText).Run()
}
