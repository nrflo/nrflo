package cli

import (
	"os/exec"
	"strings"
)

// gitTicketHint returns the current git branch name in the working directory,
// used as the console session's candidate "current ticket" (nrflo names a
// ticket's branch/worktree after the ticket id, so the branch IS the ticket id
// by convention). It is a package var so tests can stub it without a real repo.
//
// Returns "" when not in a git repo or on a detached HEAD ("HEAD") — the server
// validates the hint against real tickets and drops an unknown id silently, so
// a non-ticket branch (main, a feature branch, a typo) simply yields no current
// ticket rather than an error.
var gitTicketHint = func() string {
	out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return ""
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" || branch == "HEAD" {
		return ""
	}
	return branch
}
