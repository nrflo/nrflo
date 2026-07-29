package spawner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

// GetDelegation implements apirun.Delegator: returns the delegation's current
// aggregated status without blocking (the delegate/get_delegation builtin
// handlers own the bounded, heartbeated poll loop around this). The
// delegations row (migration 000216) is never deleted — only marked
// completed/failed and consumed once a terminal result has been read back.
func (s *Spawner) GetDelegation(ctx context.Context, callerSessionID, delegationID string) (string, error) {
	pool := s.pool()
	if pool == nil {
		return "", fmt.Errorf("delegate: no database pool")
	}
	if !strings.Contains(delegationID, ".") {
		return "", fmt.Errorf("delegate: malformed delegation_id %q", delegationID)
	}

	callerSession, err := repo.NewAgentSessionRepo(pool, s.config.Clock).Get(callerSessionID)
	if err != nil {
		return "", fmt.Errorf("delegate: resolve caller session: %w", err)
	}

	delegationRepo := repo.NewDelegationRepo(pool, s.config.Clock)
	d, err := delegationRepo.Get(delegationID)
	if err != nil {
		return "", fmt.Errorf("delegate: unknown delegation %q", delegationID)
	}
	if !strings.EqualFold(d.ProjectID, callerSession.ProjectID) {
		return "", fmt.Errorf("delegate: delegation %q was not started by this caller", delegationID)
	}

	// Already consumed by an earlier terminal read: return the stored status
	// with no results rather than re-reading (and re-deleting) worker
	// findings that are already gone.
	if d.ConsumedAt != nil {
		out := map[string]interface{}{"delegation_id": delegationID, "status": d.Status, "consumed": true}
		b, _ := json.Marshal(out)
		return string(b), nil
	}

	// Fanout not yet done: workers are still being spawned/running and the
	// final worker session-id list is not known yet — report running.
	if !d.FanoutDone {
		return delegateStatusJSON(delegationID, "running", nil), nil
	}

	results, allDone, anyFailed := s.collectDelegateResults(pool, d.WorkerSessionIDs, d.SpawnErrors)
	if !allDone {
		return delegateStatusJSON(delegationID, "running", results), nil
	}

	status := "completed"
	if anyFailed {
		status = "failed"
	}
	delegationRepo.MarkTerminal(delegationID, status) //nolint:errcheck

	return delegateStatusJSON(delegationID, status, results), nil
}

// collectDelegateResults reads each worker's terminal status and (if
// present) its `_delegate_findings` session finding — read+deleted, mirroring
// consult's _consult_answer readback. allDone is true once every session has
// left the running/continued states. spawnErrors is index-aligned with
// sessionIDs (may be shorter/nil for pre-existing tracking records).
func (s *Spawner) collectDelegateResults(pool *db.Pool, sessionIDs, spawnErrors []string) ([]map[string]interface{}, bool, bool) {
	sessionRepo := repo.NewAgentSessionRepo(pool, s.config.Clock)
	findingRepo := repo.NewFindingRepo(pool, s.config.Clock)

	results := make([]map[string]interface{}, 0, len(sessionIDs))
	allDone := true
	anyFailed := false

	for i, sid := range sessionIDs {
		if sid == "" {
			msg := "worker failed to start"
			if i < len(spawnErrors) && spawnErrors[i] != "" {
				msg += ": " + spawnErrors[i]
			}
			results = append(results, map[string]interface{}{"status": "failed", "error": msg})
			anyFailed = true
			continue
		}
		sess, err := sessionRepo.Get(sid)
		if err != nil {
			results = append(results, map[string]interface{}{"session_id": sid, "status": "failed", "error": err.Error()})
			anyFailed = true
			continue
		}
		if isDelegateSessionRunning(sess.Status) {
			allDone = false
			results = append(results, map[string]interface{}{"session_id": sid, "status": "running"})
			continue
		}

		entry := map[string]interface{}{"session_id": sid, "status": "completed"}
		switch {
		case sess.Status == model.AgentSessionCallback:
			// Nothing ever answers a delegate worker's callback (there is no
			// coordinator), so callback is terminal here, not transient.
			entry["status"] = "failed"
			entry["reason"] = "worker ended in callback; delegate workers have no coordinator to answer it"
			anyFailed = true
		case sess.Status == model.AgentSessionTimeout:
			entry["status"] = "failed"
			entry["reason"] = "worker timed out"
			anyFailed = true
		case sess.Result.Valid && sess.Result.String == "fail":
			entry["status"] = "failed"
			anyFailed = true
			if sess.ResultReason.Valid {
				entry["reason"] = sess.ResultReason.String
			}
		}
		if findings, ferr := findingRepo.GetOwn("session", sid); ferr == nil {
			if raw, ok := findings["_delegate_findings"]; ok {
				entry["findings"] = raw
				findingRepo.DeleteKeys("session", sid, []string{"_delegate_findings"}, repo.Actor{Source: "system", ID: "delegate"}) //nolint:errcheck
			}
		}
		results = append(results, entry)
	}
	return results, allDone, anyFailed
}

func isDelegateSessionRunning(status model.AgentSessionStatus) bool {
	switch status {
	case model.AgentSessionRunning, model.AgentSessionContinued:
		return true
	default:
		return false
	}
}

func delegateStatusJSON(delegationID, status string, results []map[string]interface{}) string {
	out := map[string]interface{}{"delegation_id": delegationID, "status": status}
	if results != nil {
		out["results"] = results
	}
	b, _ := json.Marshal(out)
	return string(b)
}
