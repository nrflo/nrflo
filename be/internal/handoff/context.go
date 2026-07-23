package handoff

import (
	"context"
	"database/sql"

	"be/internal/db"
	"be/internal/logger"
)

// maxChainSessions caps how many sessions of a kill->relaunch chain
// (walked backwards via ancestor_session_id) contribute message-derived
// candidates to the handoff. Findings and the slot digest already span the
// whole chain; only message-derived refs are session-scoped.
const maxChainSessions = 3

// handoffContext is the resolved identity + working-tree location a Compose
// call needs before it can read findings, extract references, and resolve
// them against the filesystem.
type handoffContext struct {
	sessionID          string
	projectID          string
	ticketID           string
	workflowInstanceID string
	nodeID             string
	taskAnchor         string
	repoRoot           string
}

// resolveContext joins agent_sessions -> workflow_instances -> projects for
// a session's identity and working-tree root. repoRoot precedence mirrors
// orchestrator/consult.go:64-70: workflow_instances.worktree_path when
// non-empty, else projects.root_path. A resolution failure (unknown
// session, missing rows) returns ok=false and logs — callers degrade to an
// empty Verified State rather than propagating the error.
func resolveContext(ctx context.Context, pool *db.Pool, sessionID string) (handoffContext, bool) {
	var hc handoffContext
	hc.sessionID = sessionID

	var prompt, worktreePath, rootPath sql.NullString
	err := pool.QueryRow(`
		SELECT s.project_id, s.workflow_instance_id, s.node_id, s.ticket_id, s.prompt,
		       wi.worktree_path, p.root_path
		FROM agent_sessions s
		JOIN workflow_instances wi ON wi.id = s.workflow_instance_id
		JOIN projects p ON p.id = s.project_id
		WHERE s.id = ?`, sessionID,
	).Scan(&hc.projectID, &hc.workflowInstanceID, &hc.nodeID, &hc.ticketID, &prompt,
		&worktreePath, &rootPath)
	if err != nil {
		logger.Warn(ctx, "handoff: resolve context failed", "session_id", sessionID, "error", err)
		return hc, false
	}

	if prompt.Valid {
		hc.taskAnchor = prompt.String
	}
	if worktreePath.Valid && worktreePath.String != "" {
		hc.repoRoot = worktreePath.String
	} else if rootPath.Valid {
		hc.repoRoot = rootPath.String
	}
	return hc, true
}

// chainSessionIDs walks ancestor_session_id backwards from sessionID,
// newest first, capped at maxChainSessions — a kill->relaunch chain shares
// one Verified State so message-derived refs from earlier sessions in the
// chain still surface after a relaunch.
func chainSessionIDs(ctx context.Context, pool *db.Pool, sessionID string) []string {
	ids := []string{sessionID}
	cur := sessionID
	for len(ids) < maxChainSessions {
		var ancestor sql.NullString
		err := pool.QueryRow(`SELECT ancestor_session_id FROM agent_sessions WHERE id = ?`, cur).Scan(&ancestor)
		if err != nil {
			logger.Warn(ctx, "handoff: walk ancestor chain failed", "session_id", cur, "error", err)
			return ids
		}
		if !ancestor.Valid || ancestor.String == "" {
			return ids
		}
		ids = append(ids, ancestor.String)
		cur = ancestor.String
	}
	return ids
}
