package spawner

import (
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
)

// freshSlotDigest is the single freshness predicate for the autonomous
// refinery slot digest at (workflowInstanceID, nodeID): the digest is fresh
// when it exists, has non-empty content, and was last folded at or after
// sessionStart (the killed session's start time). Freshness is monotonic —
// UpsertSlot only ever bumps updated_at — so a digest fresh at kill time is
// still fresh when read at relaunch-prompt-assembly time. Returns the digest
// content and true when fresh; otherwise ("", false), signaling callers to
// fall back to the existing agent-save / to_resume path.
//
// Reads via repo.NewRefineryDigestRepo (never imports refinery directly —
// see Import Hygiene in refinery/CLAUDE.md).
func freshSlotDigest(pool *db.Pool, clk clock.Clock, workflowInstanceID, nodeID string, sessionStart time.Time) (string, bool) {
	if pool == nil || workflowInstanceID == "" || nodeID == "" {
		return "", false
	}

	d, err := repo.NewRefineryDigestRepo(pool, clk).GetSlot(workflowInstanceID, nodeID)
	if err != nil || d == nil {
		return "", false
	}
	if d.Content == "" || d.UpdatedAt.Before(sessionStart) {
		return "", false
	}
	return d.Content, true
}
