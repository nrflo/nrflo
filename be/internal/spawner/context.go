package spawner

import (
	"encoding/json"
	"os"

	"be/internal/repo"
)

// contextFileEntry represents one entry in /tmp/usable_context.json
type contextFileEntry struct {
	PctUsed *float64 `json:"pct_used"`
}

// readContextFile reads /tmp/usable_context.json and returns parsed data.
// Returns nil on any error (file not found, parse error, etc).
func readContextFile() map[string]contextFileEntry {
	data, err := os.ReadFile("/tmp/usable_context.json")
	if err != nil {
		return nil
	}
	var result map[string]contextFileEntry
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result
}

// updateContextLeft updates the context_left field on a process from context file data
func updateContextLeft(proc *processInfo, contextData map[string]contextFileEntry) {
	if contextData == nil {
		return
	}
	entry, ok := contextData[proc.sessionID]
	if !ok || entry.PctUsed == nil {
		return
	}
	remaining := 100 - int(*entry.PctUsed)
	if remaining != proc.contextLeft {
		proc.contextLeft = remaining
		proc.contextLeftDirty = true
	}
}

// saveContextLeft saves context_left to the database if dirty
func (s *Spawner) saveContextLeft(proc *processInfo) {
	if !proc.contextLeftDirty {
		return
	}
	pool := s.pool()
	if pool == nil {
		return
	}

	sessionRepo := repo.NewAgentSessionRepo(pool)
	sessionRepo.UpdateContextLeft(proc.sessionID, proc.contextLeft)
	proc.contextLeftDirty = false
}
