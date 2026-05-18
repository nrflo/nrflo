package spawner

import (
	"strconv"
	"strings"
	"time"
)

// rateLimitConfig holds the resolved rate-limit retry configuration for a session.
type rateLimitConfig struct {
	Enabled        bool
	InitialBackoff time.Duration
	MaxWait        time.Duration
	LimitPatterns  []string // user-supplied extra limit patterns (merged with adapter defaults)
	ErrorPatterns  []string // user-supplied extra error patterns (merged with adapter defaults)
}

// loadRateLimitConfig reads rate-limit settings from the config table, preferring
// project-scoped values and falling back to global. Returns sensible defaults when
// nothing is configured.
func (s *Spawner) loadRateLimitConfig(projectID, adapterName string) rateLimitConfig {
	cfg := rateLimitConfig{
		Enabled:        true,
		InitialBackoff: 60 * time.Second,
		MaxWait:        3600 * time.Second,
	}

	pool := s.pool()
	if pool == nil {
		return cfg
	}

	getVal := func(key string) string {
		if projectID != "" {
			if v, _ := pool.GetProjectConfig(projectID, key); v != "" {
				return v
			}
		}
		v, _ := pool.GetConfig(key)
		return v
	}

	if v := getVal("rate_limit_enabled"); v != "" {
		cfg.Enabled = v != "false" && v != "0"
	}
	if v := getVal("rate_limit_initial_backoff_sec"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.InitialBackoff = time.Duration(n) * time.Second
		}
	}
	if v := getVal("rate_limit_max_wait_sec"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxWait = time.Duration(n) * time.Second
		}
	}
	if adapterName != "" {
		if v := getVal(adapterName + "_limit_patterns"); v != "" {
			cfg.LimitPatterns = splitConfigPatterns(v)
		}
		if v := getVal(adapterName + "_error_patterns"); v != "" {
			cfg.ErrorPatterns = splitConfigPatterns(v)
		}
	}
	return cfg
}

// splitConfigPatterns splits a comma-separated pattern string into trimmed non-empty entries.
func splitConfigPatterns(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// computeRateLimitDelay returns the backoff delay for the nth retry (1-based).
// delay = min(InitialBackoff * 2^(n-1), MaxWait).
func computeRateLimitDelay(cfg rateLimitConfig, retryCount int) time.Duration {
	if retryCount <= 0 {
		retryCount = 1
	}
	shift := uint(retryCount - 1)
	var delay time.Duration
	if shift >= 32 {
		delay = cfg.MaxWait
	} else {
		delay = cfg.InitialBackoff << shift
	}
	if delay > cfg.MaxWait {
		delay = cfg.MaxWait
	}
	return delay
}

// appendRecent appends text to the recent-blocks ring buffer (cap 10).
func (p *processInfo) appendRecent(text string) {
	if text == "" {
		return
	}
	p.recentMu.Lock()
	defer p.recentMu.Unlock()
	p.recentBlocks = append(p.recentBlocks, text)
	if len(p.recentBlocks) > 10 {
		p.recentBlocks = p.recentBlocks[len(p.recentBlocks)-10:]
	}
}

// recentTail returns the concatenated recent blocks for pattern matching.
func (p *processInfo) recentTail() string {
	p.recentMu.Lock()
	defer p.recentMu.Unlock()
	return strings.Join(p.recentBlocks, "\n")
}

// appendStderr appends text to the stderr-blocks ring buffer (cap 10).
func (p *processInfo) appendStderr(text string) {
	if text == "" {
		return
	}
	p.recentMu.Lock()
	defer p.recentMu.Unlock()
	p.stderrBlocks = append(p.stderrBlocks, text)
	if len(p.stderrBlocks) > 10 {
		p.stderrBlocks = p.stderrBlocks[len(p.stderrBlocks)-10:]
	}
}

// stderrTail returns the concatenated stderr blocks for pattern matching.
func (p *processInfo) stderrTail() string {
	p.recentMu.Lock()
	defer p.recentMu.Unlock()
	return strings.Join(p.stderrBlocks, "\n")
}
