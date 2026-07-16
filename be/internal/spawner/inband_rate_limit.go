package spawner

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"be/internal/logger"
)

// inBandRateLimitIdleGrace is how long an interactive agent must be silent
// before we read its transcript to look for a swallowed API error. The grace
// only bounds how often the transcript is read — the swallowed-error case stays
// silent indefinitely, so a short window is enough to catch it promptly.
const inBandRateLimitIdleGrace = 30 * time.Second

// nonAlnum matches every character Claude Code replaces with '-' when it
// encodes a working directory into its ~/.claude/projects/<dir> transcript
// folder name. Verified against 461 live transcript dirs: the rule is exactly
// "replace each non-alphanumeric byte with a dash" (so '/', '.', '_', '-' and
// spaces all collapse to '-').
var nonAlnum = regexp.MustCompile(`[^a-zA-Z0-9]`)

// handleInBandRateLimit detects an interactive Claude turn that ended on a
// server-side API error (e.g. HTTP 529 "Overloaded") which the CLI surfaced as
// an assistant message and then idled on — without exiting and without firing a
// Stop/StopFailure hook. None of the usual recovery paths see it: the process
// never exits (so ClassifyExit never runs), no completion tool is called, and
// the idle nudge is futile because the turn already terminally failed inside the
// CLI. Left alone the agent burns the whole nudge budget and then auto-fails.
//
// When the latest assistant message matches the adapter's rate-limit patterns,
// it routes the agent through the same kill + backoff + relaunch as a
// rate-limit exit (handleRateLimitRetry). The killed process is left with
// finalStatus=CONTINUE, so the monitor's existing continuation branch relaunches
// it on the next poll — now with the model's --fallback-model chain in play.
//
// Returns true when it handled (killed) the process. Claude-only in practice:
// only the cli_interactive backend reaches here and it is always Claude (codex
// runs on the app-server backend).
func (s *Spawner) handleInBandRateLimit(ctx context.Context, proc *processInfo, req SpawnRequest) bool {
	if proc.adapter == nil || proc.lowContextSaving || proc.finalStatus != "" {
		return false
	}
	if !proc.rateLimitConfig.Enabled || proc.rateLimitTotalWait >= proc.rateLimitConfig.MaxWait {
		return false
	}

	proc.messagesMutex.Lock()
	idle := s.config.Clock.Now().Sub(proc.lastMessageTime)
	proc.messagesMutex.Unlock()
	if idle < inBandRateLimitIdleGrace {
		return false
	}

	text := lastAssistantText(proc.env, proc.workDir, proc.sessionID)
	if text == "" {
		return false
	}

	class, matched := proc.adapter.ClassifyExit(text, "", 1,
		proc.rateLimitConfig.LimitPatterns, proc.rateLimitConfig.ErrorPatterns)
	if class != RetryClassRateLimit {
		return false
	}

	logger.Warn(ctx, "in-band rate-limit detected: CLI surfaced a server-side API error without exiting; relaunching with backoff",
		"session_id", proc.sessionID, "agent_type", proc.agentType,
		"matched", matched, "idle", idle.Round(time.Second))

	// Mirrors the exit-path rate-limit handling: kill, persist rate_limit_until,
	// set finalStatus=CONTINUE, then back off before the monitor relaunches.
	s.handleRateLimitRetry(ctx, proc, req, matched)
	s.waitForRateLimitRetry(ctx, proc, req)
	return true
}

// lastAssistantText returns the text of the last assistant message in the
// agent's Claude transcript, or "" when the transcript is missing/unreadable or
// has no assistant message yet. Only the tail of the file is read so this stays
// cheap even for long sessions.
func lastAssistantText(env []string, workDir, sessionID string) string {
	path := claudeTranscriptPath(env, workDir, sessionID)
	if path == "" {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return ""
	}
	const tailBytes = 64 * 1024
	var start int64
	if fi.Size() > tailBytes {
		start = fi.Size() - tailBytes
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return ""
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	if start > 0 && len(lines) > 0 {
		lines = lines[1:] // drop the partial line the tail seek lands inside
	}

	last := ""
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry struct {
			Type    string `json:"type"`
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal([]byte(line), &entry) != nil || entry.Type != "assistant" {
			continue
		}
		if txt := assistantBlocksText(entry.Message.Content); txt != "" {
			last = txt
		}
	}
	return last
}

// assistantBlocksText extracts the concatenated text of an assistant message's
// content, which is either a JSON array of typed blocks or a plain string.
func assistantBlocksText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) == nil {
		var sb strings.Builder
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				if sb.Len() > 0 {
					sb.WriteByte('\n')
				}
				sb.WriteString(b.Text)
			}
		}
		return sb.String()
	}
	var str string
	if json.Unmarshal(raw, &str) == nil {
		return str
	}
	return ""
}

// claudeTranscriptPath reconstructs the path to a Claude session transcript:
// <config-dir>/projects/<encoded-workdir>/<session-id>.jsonl. The config dir is
// CLAUDE_CONFIG_DIR from the agent env, else <HOME>/.claude. Returns "" if the
// inputs needed to build the path are missing. proc-free so console engines
// (no processInfo) can reuse it.
func claudeTranscriptPath(env []string, workDir, sessionID string) string {
	if workDir == "" || sessionID == "" {
		return ""
	}
	base := envValue(env, "CLAUDE_CONFIG_DIR")
	if base == "" {
		home := envValue(env, "HOME")
		if home == "" {
			home, _ = os.UserHomeDir()
		}
		if home == "" {
			return ""
		}
		base = filepath.Join(home, ".claude")
	}
	// claude derives the project dir from its resolved cwd (getcwd follows
	// symlinks — /var/folders → /private/var/folders on macOS), so resolve
	// before encoding or the path misses for any symlinked workdir.
	if resolved, err := filepath.EvalSymlinks(workDir); err == nil {
		workDir = resolved
	}
	encoded := nonAlnum.ReplaceAllString(workDir, "-")
	return filepath.Join(base, "projects", encoded, sessionID+".jsonl")
}

// envValue returns the value of key in a KEY=VALUE env slice ("" if absent).
func envValue(env []string, key string) string {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return e[len(prefix):]
		}
	}
	return ""
}
