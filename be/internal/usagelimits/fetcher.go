package usagelimits

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"time"

	"be/internal/logger"
)

const scriptTimeout = 65 * time.Second

// scriptResult matches the JSON output structure of scripts/usage-limits.sh.
type scriptResult struct {
	Claude scriptTool `json:"claude"`
	Codex  scriptTool `json:"codex"`
}

type scriptTool struct {
	Available bool         `json:"available"`
	Session   *UsageMetric `json:"session"`
	Weekly    *UsageMetric `json:"weekly"`
	Error     string       `json:"error,omitempty"`
}

// FetchAll runs scripts/usage-limits.sh and returns parsed usage data.
func FetchAll(scriptPath string) *UsageLimits {
	ctx := context.Background()
	result := &UsageLimits{FetchedAt: time.Now()}

	if _, err := os.Stat(scriptPath); err != nil {
		logger.Info(ctx, "usage-limits: script not found", "path", scriptPath)
		result.Claude = ToolUsage{Available: false, Error: "script not found"}
		result.Codex = ToolUsage{Available: false, Error: "script not found"}
		return result
	}

	logger.Info(ctx, "usage-limits: running script", "path", scriptPath)

	cmdCtx, cancel := context.WithTimeout(ctx, scriptTimeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, scriptPath)
	cmd.Env = filteredEnv()

	out, err := cmd.Output()
	if err != nil {
		logger.Info(ctx, "usage-limits: script failed", "error", err, "output", string(out))
		result.Claude = ToolUsage{Available: false, Error: "script error: " + err.Error()}
		result.Codex = ToolUsage{Available: false, Error: "script error: " + err.Error()}
		return result
	}

	var parsed scriptResult
	if err := json.Unmarshal(out, &parsed); err != nil {
		logger.Info(ctx, "usage-limits: parse failed", "error", err, "output", string(out))
		result.Claude = ToolUsage{Available: false, Error: "parse error: " + err.Error()}
		result.Codex = ToolUsage{Available: false, Error: "parse error: " + err.Error()}
		return result
	}

	result.Claude = ToolUsage{
		Available: parsed.Claude.Available,
		Session:   parsed.Claude.Session,
		Weekly:    parsed.Claude.Weekly,
		Error:     parsed.Claude.Error,
	}
	result.Codex = ToolUsage{
		Available: parsed.Codex.Available,
		Session:   parsed.Codex.Session,
		Weekly:    parsed.Codex.Weekly,
		Error:     parsed.Codex.Error,
	}

	logger.Info(ctx, "usage-limits: FetchAll done",
		"claude_available", result.Claude.Available,
		"claude_error", result.Claude.Error,
		"codex_available", result.Codex.Available,
		"codex_error", result.Codex.Error,
	)
	return result
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
