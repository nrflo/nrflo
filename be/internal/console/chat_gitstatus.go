package console

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const gitStatusTimeout = 2 * time.Second

// runGitCommand runs `git <args...>` in dir with a short timeout and dir set
// explicitly (never inheriting the server's cwd) — a package var so tests can
// stub it without a real repo, per the cli.gitTicketHint idiom
// (be/internal/cli/console_git.go).
var runGitCommand = func(dir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitStatusTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	return cmd.Output()
}

// gitWorkdirStatus reports dir's branch (or short SHA on detached HEAD) plus
// added/deleted line counts for uncommitted changes against HEAD (staged +
// unstaged, tracked files only). ok=false — dir isn't a git repo, git isn't
// available, or any command errors/times out — means the caller must omit
// the entire status-bar segment rather than render a placeholder.
func gitWorkdirStatus(dir string) (branch string, added, deleted int, ok bool) {
	if dir == "" {
		return "", 0, 0, false
	}
	branchOut, err := runGitCommand(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", 0, 0, false
	}
	branch = strings.TrimSpace(string(branchOut))
	if branch == "" {
		return "", 0, 0, false
	}
	if branch == "HEAD" {
		shaOut, err := runGitCommand(dir, "rev-parse", "--short", "HEAD")
		if err != nil {
			return "", 0, 0, false
		}
		branch = strings.TrimSpace(string(shaOut))
		if branch == "" {
			return "", 0, 0, false
		}
	}

	diffOut, err := runGitCommand(dir, "diff", "--numstat", "HEAD")
	if err != nil {
		return "", 0, 0, false
	}
	added, deleted, ok = parseNumstat(string(diffOut))
	if !ok {
		return "", 0, 0, false
	}
	return branch, added, deleted, true
}

// parseNumstat sums `git diff --numstat` rows ("<added>\t<deleted>\t<path>",
// binary files as "-\t-\t<path>"). A malformed row — not exactly 3
// tab-separated fields, or a non-"-" count that isn't an integer — fails the
// whole parse so the caller omits the segment instead of showing a partial
// count.
func parseNumstat(output string) (added, deleted int, ok bool) {
	output = strings.TrimSpace(output)
	if output == "" {
		return 0, 0, true
	}
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			return 0, 0, false
		}
		a, aok := numstatField(fields[0])
		d, dok := numstatField(fields[1])
		if !aok || !dok {
			return 0, 0, false
		}
		added += a
		deleted += d
	}
	return added, deleted, true
}

// numstatField parses one numstat count field: "-" (binary file) is 0/ok,
// anything else must be a non-negative integer.
func numstatField(field string) (int, bool) {
	if field == "-" {
		return 0, true
	}
	n, err := strconv.Atoi(field)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}
