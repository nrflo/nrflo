package service

import (
	"os/exec"
	"sync"
)

// lookPath is exec.LookPath by default; tests stub it so CLIAvailable never
// shells out (precedent: python_script_validate.go, venv/manager.go).
var lookPath = exec.LookPath

var cliAvailabilityCache sync.Map // cliType (string) -> bool

// CLIAvailable reports whether the named CLI binary is on PATH, memoized per
// cliType for the process lifetime. This is the only way to hide a
// read_only model row's templates on an install that lacks the binary —
// read-only model rows (e.g. every seeded OpenAI row) can never be disabled
// via the `enabled` flag (see model.go), so a codex-less Docker
// image would otherwise keep offering codex fanout templates to the planner.
func CLIAvailable(cliType string) bool {
	if v, ok := cliAvailabilityCache.Load(cliType); ok {
		return v.(bool)
	}
	_, err := lookPath(cliType)
	available := err == nil
	cliAvailabilityCache.Store(cliType, available)
	return available
}
