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

// delegationTracking is the `_delegation_<id>` finding value persisted on the
// (real or hidden-host) workflow instance a delegate call spawned workers
// under — survives across turns/requests so GetDelegation can poll it. Seeded
// with Done=false before the workers run (SessionIDs empty), then rewritten
// with Done=true and the full SessionIDs list once the fanout finishes.
type delegationTracking struct {
	Tier       string   `json:"tier"`
	SessionIDs []string `json:"session_ids"`
	Done       bool     `json:"done"`
}

func delegationFindingKey(delegationID string) string {
	return "_delegation_" + delegationID
}

// trackDelegation persists the fanout's worker session ids on wfiID, keyed by
// delegationID, so a later GetDelegation call (which resolves wfiID back out
// of delegationID's "<wfiID>.<rand>" shape) can find them again. Called twice:
// once with done=false (seed, empty sessionIDs) and once with done=true (final
// session-id list) after the workers finish.
func (s *Spawner) trackDelegation(wfiID, projectID, delegationID, tier string, sessionIDs []string, done bool) error {
	pool := s.pool()
	if pool == nil {
		return fmt.Errorf("delegate: no database pool")
	}
	val, err := json.Marshal(delegationTracking{Tier: tier, SessionIDs: sessionIDs, Done: done})
	if err != nil {
		return err
	}
	findingRepo := repo.NewFindingRepo(pool, s.config.Clock)
	return findingRepo.Upsert("workflow_instance", wfiID, delegationFindingKey(delegationID), val,
		repo.Denorm{ProjectID: projectID, WorkflowInstanceID: wfiID}, repo.Actor{Source: "system", ID: "delegate"})
}

// GetDelegation implements apirun.Delegator: returns the delegation's current
// aggregated status without blocking (the delegate/get_delegation builtin
// handlers own the bounded, heartbeated poll loop around this).
func (s *Spawner) GetDelegation(ctx context.Context, callerSessionID, delegationID string) (string, error) {
	pool := s.pool()
	if pool == nil {
		return "", fmt.Errorf("delegate: no database pool")
	}
	wfiID, _, ok := strings.Cut(delegationID, ".")
	if !ok || wfiID == "" {
		return "", fmt.Errorf("delegate: malformed delegation_id %q", delegationID)
	}

	callerSession, err := repo.NewAgentSessionRepo(pool, s.config.Clock).Get(callerSessionID)
	if err != nil {
		return "", fmt.Errorf("delegate: resolve caller session: %w", err)
	}
	wfi, err := repo.NewWorkflowInstanceRepo(pool, s.config.Clock).Get(wfiID)
	if err != nil {
		return "", fmt.Errorf("delegate: resolve workflow instance: %w", err)
	}
	if !strings.EqualFold(wfi.ProjectID, callerSession.ProjectID) {
		return "", fmt.Errorf("delegate: delegation %q was not started by this caller", delegationID)
	}

	findingRepo := repo.NewFindingRepo(pool, s.config.Clock)
	findings, err := findingRepo.GetOwn("workflow_instance", wfiID)
	if err != nil {
		return "", fmt.Errorf("delegate: read tracking: %w", err)
	}
	raw, ok := findings[delegationFindingKey(delegationID)]
	if !ok {
		return "", fmt.Errorf("delegate: unknown delegation %q", delegationID)
	}
	var tracking delegationTracking
	if err := json.Unmarshal(raw, &tracking); err != nil {
		return "", fmt.Errorf("delegate: decode tracking: %w", err)
	}

	// Not-yet-done seed record: workers are still being spawned/running and the
	// final session-id list is not known yet — report running without deleting
	// the tracking record.
	if !tracking.Done {
		return delegateStatusJSON(delegationID, "running", nil), nil
	}

	results, allDone, anyFailed := s.collectDelegateResults(pool, tracking.SessionIDs)
	if !allDone {
		return delegateStatusJSON(delegationID, "running", results), nil
	}

	findingRepo.DeleteKeys("workflow_instance", wfiID, []string{delegationFindingKey(delegationID)}, repo.Actor{Source: "system", ID: "delegate"}) //nolint:errcheck

	status := "completed"
	if anyFailed {
		status = "failed"
	}
	return delegateStatusJSON(delegationID, status, results), nil
}

// collectDelegateResults reads each worker's terminal status and (if
// present) its `_delegate_findings` session finding — read+deleted, mirroring
// consult's _consult_answer readback. allDone is true once every session has
// left the running/continued/callback states.
func (s *Spawner) collectDelegateResults(pool *db.Pool, sessionIDs []string) ([]map[string]interface{}, bool, bool) {
	sessionRepo := repo.NewAgentSessionRepo(pool, s.config.Clock)
	findingRepo := repo.NewFindingRepo(pool, s.config.Clock)

	results := make([]map[string]interface{}, 0, len(sessionIDs))
	allDone := true
	anyFailed := false

	for _, sid := range sessionIDs {
		if sid == "" {
			results = append(results, map[string]interface{}{"status": "failed", "error": "worker failed to start"})
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
		if sess.Result.Valid && sess.Result.String == "fail" {
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
	case model.AgentSessionRunning, model.AgentSessionContinued, model.AgentSessionCallback:
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
