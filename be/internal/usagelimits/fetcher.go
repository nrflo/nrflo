package usagelimits

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	creackpty "github.com/creack/pty"

	"be/internal/logger"
)

const (
	fetchTimeout = 15 * time.Second
	readBufSize  = 4096
	// Delay before sending command to let the CLI initialize
	initDelay = 2 * time.Second
)

// FetchAll fetches usage data from both Claude and Codex concurrently.
func FetchAll() *UsageLimits {
	result := &UsageLimits{
		FetchedAt: time.Now(),
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		result.Claude = fetchClaude()
	}()

	go func() {
		defer wg.Done()
		result.Codex = fetchCodex()
	}()

	wg.Wait()
	return result
}

func fetchClaude() ToolUsage {
	if _, err := exec.LookPath("claude"); err != nil {
		return ToolUsage{Available: false}
	}

	raw, err := spawnAndScrape("claude", nil, "/usage\n", "/exit\n", "resets")
	if err != nil {
		logger.Info(context.Background(), "usage-limits: claude scrape failed", "error", err)
		return ToolUsage{Available: true, Error: "scrape failed: " + err.Error()}
	}

	cleaned := stripANSI(raw)
	return *parseClaude(cleaned)
}

func fetchCodex() ToolUsage {
	if _, err := exec.LookPath("codex"); err != nil {
		return ToolUsage{Available: false}
	}

	raw, err := spawnAndScrape("codex", nil, "/status\n", "/exit\n", "% left", "% used")
	if err != nil {
		logger.Info(context.Background(), "usage-limits: codex scrape failed", "error", err)
		return ToolUsage{Available: true, Error: "scrape failed: " + err.Error()}
	}

	cleaned := stripANSI(raw)
	return *parseCodex(cleaned)
}

// spawnAndScrape spawns a CLI in a PTY, sends a command, reads until
// one of the stopPatterns is found or timeout, then sends exitCmd and kills.
func spawnAndScrape(cli string, extraEnv []string, cmd, exitCmd string, stopPatterns ...string) ([]byte, error) {
	execCmd := exec.Command(cli)

	// Filter CLAUDECODE from env (avoids nested-mode behavior)
	execCmd.Env = filteredEnv(extraEnv)

	ptmx, err := creackpty.Start(execCmd)
	if err != nil {
		return nil, err
	}
	defer ptmx.Close()

	// Ensure process is killed on exit
	defer func() {
		if execCmd.Process != nil {
			_ = execCmd.Process.Signal(syscall.SIGTERM)
			// Give it a moment then force kill
			go func() {
				time.Sleep(2 * time.Second)
				if execCmd.Process != nil {
					_ = execCmd.Process.Kill()
				}
			}()
		}
	}()

	// Wait for CLI to initialize
	time.Sleep(initDelay)

	// Send command
	_, _ = ptmx.Write([]byte(cmd))

	// Read output with timeout
	var buf bytes.Buffer
	deadline := time.Now().Add(fetchTimeout)
	readBuf := make([]byte, readBufSize)

	for time.Now().Before(deadline) {
		_ = ptmx.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, err := ptmx.Read(readBuf)
		if n > 0 {
			buf.Write(readBuf[:n])

			// Check for stop patterns
			content := buf.Bytes()
			for _, pattern := range stopPatterns {
				if bytes.Contains(bytes.ToLower(content), bytes.ToLower([]byte(pattern))) {
					// Give a moment for remaining output to arrive
					time.Sleep(1 * time.Second)
					// Read any remaining
					_ = ptmx.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
					if extra, err := ptmx.Read(readBuf); extra > 0 && err == nil {
						buf.Write(readBuf[:extra])
					}
					goto done
				}
			}
		}
		if err != nil && !os.IsTimeout(err) {
			break
		}
	}

done:
	// Send exit command
	_, _ = ptmx.Write([]byte(exitCmd))

	return buf.Bytes(), nil
}

// filteredEnv returns os.Environ() with CLAUDECODE removed.
func filteredEnv(extra []string) []string {
	env := os.Environ()
	filtered := make([]string, 0, len(env)+len(extra))
	for _, e := range env {
		if len(e) >= 10 && e[:10] == "CLAUDECODE" {
			continue
		}
		filtered = append(filtered, e)
	}
	filtered = append(filtered, extra...)
	return filtered
}
