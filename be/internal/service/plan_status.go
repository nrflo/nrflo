package service

import (
	"context"
	"database/sql"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
)

// DerivePlanInstanceStatus derives the plan-boundary suspend status for a
// plan-driven workflow instance from its workflow_plans head and any
// in-flight planner child: no head, latest_revision==0, or a running planner
// session -> planning; a draft with open questions -> waiting_input; an
// approvable draft -> waiting_approval. Approved/cancelled heads are not
// suspend states — the orchestrator materializes (approved) or fails the run
// (cancelled) instead of calling this.
func DerivePlanInstanceStatus(pool *db.Pool, clk clock.Clock, instanceID string) (model.WorkflowInstanceStatus, error) {
	running, err := plannerChildRunning(pool, instanceID)
	if err != nil {
		return "", err
	}
	if running {
		return model.WorkflowInstancePlanning, nil
	}

	planRepo := repo.NewPlanRepo(pool, clk)
	head, err := planRepo.GetHead(instanceID)
	if err == sql.ErrNoRows {
		return model.WorkflowInstancePlanning, nil
	}
	if err != nil {
		return "", err
	}
	if head.Status != model.PlanStatusDraft || head.LatestRevision == 0 {
		return model.WorkflowInstancePlanning, nil
	}

	rev, err := planRepo.GetRevision(instanceID, head.LatestRevision)
	if err != nil {
		return "", err
	}
	m, err := ParsePlanManifest([]byte(rev.Manifest))
	if err != nil || len(m.Questions) > 0 {
		return model.WorkflowInstanceWaitingInput, nil
	}
	return model.WorkflowInstanceWaitingApproval, nil
}

// plannerChildRunning reports whether a `_planner` child session is still
// active under the instance (a revise-via-planner call in flight).
func plannerChildRunning(pool *db.Pool, instanceID string) (bool, error) {
	var count int
	err := pool.QueryRow(
		`SELECT COUNT(*) FROM agent_sessions
		 WHERE workflow_instance_id = ? AND node_id = '_planner' AND status IN ('running', 'continued')`,
		instanceID,
	).Scan(&count)
	return count > 0, err
}

// SetPlanInstanceStatus updates a workflow instance's status to a plan
// status, guarded so it only overwrites a status that is itself already a
// plan status — a run actively executing its static layers can never be
// clobbered by a concurrent plan-lifecycle write.
func SetPlanInstanceStatus(pool *db.Pool, clk clock.Clock, instanceID string, status model.WorkflowInstanceStatus) error {
	now := clk.Now().UTC().Format(time.RFC3339Nano)
	_, err := pool.Exec(
		`UPDATE workflow_instances SET status = ?, updated_at = ?
		 WHERE id = ? AND status IN ('planning', 'plan_ready', 'waiting_input', 'waiting_approval')`,
		string(status), now, instanceID,
	)
	return err
}

// syncPlanSuspendedStatus re-derives the plan status of an instance already
// suspended at the plan boundary and writes it, so a caller polling
// get_subworkflow observes waiting_input -> waiting_approval once a draft's
// open questions are answered. The write is guarded by SetPlanInstanceStatus:
// a run still executing its static layers keeps its own status. Best-effort —
// the revision is already committed, and a stale status self-heals on the next
// revision.
func (s *PlanService) syncPlanSuspendedStatus(ctx context.Context, instanceID string) {
	status, err := DerivePlanInstanceStatus(s.pool, s.clock, instanceID)
	if err != nil {
		logger.Warn(ctx, "plan: derive instance status after revise", "instance_id", instanceID, "error", err)
		return
	}
	if err := SetPlanInstanceStatus(s.pool, s.clock, instanceID, status); err != nil {
		logger.Warn(ctx, "plan: sync suspended instance status", "instance_id", instanceID, "status", status, "error", err)
	}
}
