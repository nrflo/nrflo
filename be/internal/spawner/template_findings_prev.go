package spawner

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/handoff"
	"be/internal/repo"
)

// prevContinuedSession identifies the most recent continued session for an
// agent type/model/phase, resolved once and shared by the previous-data read
// (previousDataFor) and the restart-feedback prepend (restartFeedbackBlock)
// so both act on the same row.
type prevContinuedSession struct {
	sessionID string
	reason    string
	startedAt time.Time // zero value when the row had no started_at
}

// resolvePrevContinuedSession resolves the workflow instance (directly from
// instanceID when set, else by ticket/project lookup) and the most recent
// 'continued' agent_sessions row for agentType/modelID/phase within it.
// ok is false when no workflow instance or no continued session was found.
func (s *Spawner) resolvePrevContinuedSession(projectID, ticketID, workflowName, agentType, modelID, phase, instanceID string) (prev prevContinuedSession, wfiID string, ok bool) {
	if phase == "" {
		return prevContinuedSession{}, "", false
	}
	pool := s.pool()
	if pool == nil {
		return prevContinuedSession{}, "", false
	}

	wfiID = instanceID
	var err error
	if wfiID == "" {
		if ticketID == "" {
			err = pool.QueryRow(`
				SELECT id FROM workflow_instances
				WHERE LOWER(project_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?) AND scope_type = 'project' AND status = 'active'
				ORDER BY created_at DESC LIMIT 1`,
				projectID, workflowName).Scan(&wfiID)
		} else {
			err = pool.QueryRow(`
				SELECT id FROM workflow_instances
				WHERE LOWER(project_id) = LOWER(?) AND LOWER(ticket_id) = LOWER(?) AND LOWER(workflow_id) = LOWER(?)`,
				projectID, ticketID, workflowName).Scan(&wfiID)
		}
		if err != nil {
			return prevContinuedSession{}, "", false
		}
	}

	var sessionID string
	var reasonStr, startedAtStr sql.NullString
	err = pool.QueryRow(`
		SELECT id, result_reason, started_at FROM agent_sessions
		WHERE workflow_instance_id = ? AND agent_type = ? AND model_id = ? AND node_id = ? AND status = 'continued'
		ORDER BY ended_at DESC LIMIT 1`,
		wfiID, agentType, modelID, phase).Scan(&sessionID, &reasonStr, &startedAtStr)
	if err != nil {
		return prevContinuedSession{}, "", false
	}

	reason := ""
	if reasonStr.Valid {
		reason = reasonStr.String
	}
	var startedAt time.Time
	if startedAtStr.Valid {
		startedAt, _ = time.Parse(time.RFC3339Nano, startedAtStr.String)
	}

	return prevContinuedSession{sessionID: sessionID, reason: reason, startedAt: startedAt}, wfiID, true
}

// previousDataFor resolves ${PREVIOUS_DATA} for the low-context injectable
// from an already-resolved prevContinuedSession: stepwise defs short-circuit
// to the server-owned cursor snapshot; full-mode defs prefer a fresh
// autonomous refinery slot digest over the to_resume finding, and wrap
// either through handoff.Compose.
func (s *Spawner) previousDataFor(prev prevContinuedSession, wfiID, agentType, projectID, workflowName, phase string) string {
	pool := s.pool()
	if pool == nil {
		return ""
	}

	// Stepwise defs never read the to_resume finding / handoff.Compose — the
	// server-owned cursor is the source of truth for what a relaunch has
	// already done.
	if s.stepwiseDefFor(agentType, projectID, workflowName) {
		return s.stepwiseResumeData(pool, s.config.Clock, wfiID, phase, prev.startedAt)
	}

	// Fresh autonomous refinery slot digest takes priority over the
	// to_resume finding — one canonical source for the low-context
	// injectable's data (see digest_freshness.go).
	if !prev.startedAt.IsZero() {
		if content, ok := freshSlotDigest(pool, s.config.Clock, wfiID, phase, prev.startedAt); ok {
			return composeOrNarrative(pool, s.config.Clock, prev.sessionID, content)
		}
	}

	findingRepo := repo.NewFindingRepo(pool, s.config.Clock)
	rawFindings, err := findingRepo.GetOwn("session", prev.sessionID)
	if err != nil || len(rawFindings) == 0 {
		return ""
	}

	rawVal, ok := rawFindings["to_resume"]
	if !ok {
		return ""
	}
	var str string
	if json.Unmarshal(rawVal, &str) != nil || str == "" {
		return ""
	}
	return composeOrNarrative(pool, s.config.Clock, prev.sessionID, str)
}

// composeOrNarrative wraps a resolved narrative (fresh slot digest or
// to_resume finding — both model free text) with the deterministic Verified
// State channel via handoff.Compose, falling back to the raw narrative when
// Compose has nothing to add (e.g. no findings, no messages, no root_path).
func composeOrNarrative(pool *db.Pool, clk clock.Clock, sessionID, narrative string) string {
	if composed := handoff.Compose(context.Background(), pool, clk, sessionID, narrative); composed != "" {
		return composed
	}
	return narrative
}
