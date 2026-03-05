package orchestrator

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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
