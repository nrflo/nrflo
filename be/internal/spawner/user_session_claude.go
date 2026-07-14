package spawner

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"time"

	"be/internal/pty"
)

// PrepareUserSession builds the argv for a human-driven Claude PTY session
// (orchestrator interactive/plan pre-step, take-control resume). This is a
// HUMAN session sharing the managed spawn's boundary
// (--dangerously-skip-permissions in interactive mode, plan mode's
// --permission-mode plan) — see the managed-session flags in
// BuildInteractiveCommand. opts.PromptFile/SystemPromptOverrideFile are
// written by the caller (orchestrator); this adapter needs no per-session
// profile dir, so cleanup is a noop.
func (a *ClaudeAdapter) PrepareUserSession(opts UserSessionOptions) (pty.Launch, func(), error) {
	args := []string{
		"--session-id", opts.SessionID,
		"--model", opts.Model,
	}
	if opts.ReasoningEffort != "" {
		args = append(args, "--effort", opts.ReasoningEffort)
	}
	if opts.FallbackModels != "" {
		args = append(args, "--fallback-model", opts.FallbackModels)
	}
	if opts.SystemPromptOverrideFile != "" {
		args = append(args, "--system-prompt-file", opts.SystemPromptOverrideFile)
	}
	args = append(args, "--append-system-prompt-file", opts.PromptFile)
	if opts.SettingsJSON != "" {
		args = append(args, "--settings", opts.SettingsJSON)
	}
	if opts.PlanMode {
		// --permission-mode plan handles permissions on its own. Do NOT add
		// --dangerously-skip-permissions — it overrides plan mode.
		args = append(args, "--permission-mode", "plan", "--disallowed-tools", "ExitPlanMode")
	} else {
		args = append(args, "--dangerously-skip-permissions")
	}
	return pty.Launch{Command: "claude", Args: args, Dir: opts.WorkDir}, func() {}, nil
}

// PlanPromptSuffix returns "" — Claude's plan is read from its native
// ~/.claude/plans store, not a file the prompt has to name.
func (a *ClaudeAdapter) PlanPromptSuffix(_ PlanCaptureOptions) string { return "" }

// ReadPlan scans ~/.claude/plans/ for the plan used in opts.SessionID.
func (a *ClaudeAdapter) ReadPlan(opts PlanCaptureOptions) string {
	return readPlanFile(opts.SessionID, opts.WorkDir)
}

// readPlanFile scans ~/.claude/plans/ for recently-modified .md files, then
// greps the Claude session JSONL log for plan filenames to find which plan
// was used in the given session. Returns the matching plan content, or empty
// string if no match.
func readPlanFile(sessionID, projectRoot string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	plansDir := filepath.Join(homeDir, ".claude", "plans")
	entries, err := os.ReadDir(plansDir)
	if err != nil {
		return ""
	}

	// Collect .md files modified in the last 2 days
	cutoff := time.Now().Add(-48 * time.Hour)
	var recentPlans []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(cutoff) {
			recentPlans = append(recentPlans, entry.Name())
		}
	}

	if len(recentPlans) == 0 {
		return ""
	}

	// Build session log path: ~/.claude/projects/<encoded-project-root>/<sessionID>.jsonl
	// Encoding: replace / with - and prepend -
	encodedRoot := "-" + strings.ReplaceAll(strings.TrimPrefix(projectRoot, "/"), "/", "-")
	sessionLogPath := filepath.Join(homeDir, ".claude", "projects", encodedRoot, sessionID+".jsonl")

	logFile, err := os.Open(sessionLogPath)
	if err != nil {
		return ""
	}
	defer logFile.Close()

	// Grep each plan filename in the session log, track the last match
	var lastMatch string
	scanner := bufio.NewScanner(logFile)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer for long JSONL lines
	for scanner.Scan() {
		line := scanner.Text()
		for _, planName := range recentPlans {
			if strings.Contains(line, planName) {
				lastMatch = planName
			}
		}
	}

	if lastMatch == "" {
		return ""
	}

	// Read and return the matching plan file content
	content, err := os.ReadFile(filepath.Join(plansDir, lastMatch))
	if err != nil {
		return ""
	}
	return string(content)
}
