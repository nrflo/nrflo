package service

import (
	"database/sql"
	"time"

	"be/internal/model"
)

// stopBlockCap is how many times the Stop hook will block (force-continue) an
// autonomous session that ends a turn without calling a completion tool before
// the session is failed instead. Kept ≤ Claude's CLAUDE_CODE_STOP_HOOK_BLOCK_CAP
// so nrflo records the explicit fail before the CLI stops honoring the block.
const stopBlockCap = 3

// stopFinishReminder is fed back to the model via the Stop hook's block decision.
const stopFinishReminder = "You ended your turn without calling a completion tool. " +
	"Call exactly one of agent_finished (on success) or agent_fail (with a reason, on failure) now. " +
	"If work remains, keep going, then call one of them. Do not stop without calling a completion tool."

// stopHookAllows reports whether a Stop should pass through without blocking: a
// completion tool already set a result, or the session is not an autonomous
// running agent (interactive/plan/waiting sessions must never be blocked).
func stopHookAllows(status, result string) bool {
	return result != "" || status != string(model.AgentSessionRunning)
}

// stopHookBudget maps the (already incremented) block count to a decision: within
// budget → block (force continue); over budget → caller should fail the session.
func stopHookBudget(blockCount int) (block, capExceeded bool) {
	if blockCount > stopBlockCap {
		return false, true
	}
	return true, false
}

// StopHookDecision decides what a Claude Stop hook should do for a session that
// ended a turn. block=true (with reason) forces the agent to keep going until it
// calls a completion tool; capExceeded=true means the block budget is spent and
// the caller should fail the session. Both false → allow the stop.
func (s *AgentService) StopHookDecision(sessionID string) (block bool, reason string, capExceeded bool, err error) {
	var status string
	var result sql.NullString
	qerr := s.pool.QueryRow(`SELECT status, result FROM agent_sessions WHERE id = ?`, sessionID).Scan(&status, &result)
	if qerr == sql.ErrNoRows {
		return false, "", false, nil
	}
	if qerr != nil {
		return false, "", false, qerr
	}
	if stopHookAllows(status, result.String) {
		return false, "", false, nil
	}
	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
	if _, uerr := s.pool.Exec(
		`UPDATE agent_sessions SET stop_block_count = stop_block_count + 1, updated_at = ? WHERE id = ?`,
		now, sessionID); uerr != nil {
		return false, "", false, uerr
	}
	var count int
	if cerr := s.pool.QueryRow(`SELECT stop_block_count FROM agent_sessions WHERE id = ?`, sessionID).Scan(&count); cerr != nil {
		return false, "", false, cerr
	}
	block, capExceeded = stopHookBudget(count)
	if block {
		reason = stopFinishReminder
	}
	return block, reason, capExceeded, nil
}
