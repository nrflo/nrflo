package spawner

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner/apirun"
)

// delegateContext appends a hint naming the caller-supplied artifacts to the
// inline context so the worker knows which #{ARTIFACT:name} to fetch out of
// the #{ARTIFACTS} listing (all materialized artifacts on the shared
// instance, not just the ones the caller named).
func delegateContext(req apirun.DelegateRequest) string {
	if len(req.Artifacts) == 0 {
		return req.Context
	}
	hint := "Relevant artifacts: " + strings.Join(req.Artifacts, ", ")
	if req.Context == "" {
		return hint
	}
	return req.Context + "\n\n" + hint
}

// delegateRun bundles the per-fanout identifiers threaded through
// runDelegateFanout/spawnDelegateWorker, instead of growing their already-long
// parameter lists further.
type delegateRun struct {
	delegationID string
	// depth is this delegation row's depth, seeded onto every worker's child
	// Spawner as Config.DelegateDepth (replacing the old in-memory
	// s.config.DelegateDepth+1 threading).
	depth int
	// trx is the caller's log trx, carried across the detached fanout so
	// worker spawns log under the caller's grep key instead of "-".
	trx string
}

// createDelegationRecord seeds the durable delegations row (migration 000216)
// synchronously, before Delegate returns, so GetDelegation never sees an
// unknown delegation. depth is callerDepth+1, where callerDepth is the
// caller session's own position in the delegate tree (0 for a top-level,
// non-delegate caller) — derived via DepthForSession rather than threaded
// in-memory, which is what makes the console path (a fresh Spawner per call)
// resolve depth correctly too.
func (s *Spawner) createDelegationRecord(pool *db.Pool, wfiID, projectID, callerSessionID, tier, brief string, fanout int) (*delegateRun, error) {
	delegationRepo := repo.NewDelegationRepo(pool, s.config.Clock)
	callerDepth, err := delegationRepo.DepthForSession(callerSessionID)
	if err != nil {
		return nil, fmt.Errorf("resolve caller depth: %w", err)
	}
	depth := callerDepth + 1

	delegationID := wfiID + "." + uuid.New().String()[:8]
	d := &model.Delegation{
		ID:                 delegationID,
		CallerSessionID:    callerSessionID,
		WorkflowInstanceID: wfiID,
		ProjectID:          projectID,
		Tier:               tier,
		Brief:              brief,
		Fanout:             fanout,
		Depth:              depth,
	}
	if err := delegationRepo.Create(d); err != nil {
		return nil, fmt.Errorf("create delegation row: %w", err)
	}
	return &delegateRun{delegationID: delegationID, depth: depth}, nil
}

// recordWorkerSlot writes one fanout worker's session id/spawn error into its
// slot. Called at registration time (session id, empty error) so the slot is
// linkable while the worker is still running, and again after Spawn returns
// to finalize the spawn error (or re-confirm the same session id on
// success) — session ids land incrementally instead of only after wg.Wait.
func (s *Spawner) recordWorkerSlot(pool *db.Pool, delegationID string, idx int, sessionID, spawnErr string) error {
	return repo.NewDelegationRepo(pool, s.config.Clock).SetWorkerSlot(delegationID, idx, sessionID, spawnErr)
}

// markFanoutDone flips fanout_done once every worker has been spawned.
func (s *Spawner) markFanoutDone(pool *db.Pool, delegationID string) error {
	return repo.NewDelegationRepo(pool, s.config.Clock).MarkFanoutDone(delegationID)
}

// markDelegationCompleted stamps status/completed_at at fanout end from the
// workers' terminal outcomes — without consuming findings — so a delegation
// whose caller never polls still leaves 'running' (nrworkflow-23cbe6).
// Consumption stays with GetDelegation's result readback.
func (s *Spawner) markDelegationCompleted(delegationID string) {
	pool := s.pool()
	delegationRepo := repo.NewDelegationRepo(pool, s.config.Clock)
	d, err := delegationRepo.Get(delegationID)
	if err != nil {
		return
	}
	_, allDone, anyFailed := s.collectDelegateResults(pool, d.WorkerSessionIDs, d.SpawnErrors, false)
	if !allDone {
		return
	}
	status := "completed"
	if anyFailed {
		status = "failed"
	}
	delegationRepo.MarkCompleted(delegationID, status) //nolint:errcheck
}

// createDelegateHostInstance lazily seeds the hidden `_delegate_host` global
// workflow definition (idempotent, INSERT OR IGNORE — see
// service.EnsureGlobalDynamicWorkflow for the identical shape) and mints a
// fresh instance under it, scoped to projectID, stamped with
// RunOriginDelegate + callerSessionID so the flow graph can attribute it back
// to the caller that had no workflow instance of its own.
func (s *Spawner) createDelegateHostInstance(pool *db.Pool, projectID, callerSessionID string) (*model.WorkflowInstance, error) {
	now := s.config.Clock.Now().UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(
		`INSERT OR IGNORE INTO workflows (id, project_id, description, scope_type, groups, close_ticket_on_complete, purge_on_completion, callable_as_subworkflow, is_global, finding_schemas, created_at, updated_at)
		 VALUES (?, ?, ?, 'project', '[]', 0, 0, 0, 1, '{}', ?, ?)`,
		delegateHiddenWorkflow, service.GlobalProjectID,
		"Hidden host for delegate calls from a caller with no bound workflow instance (e.g. a console session)",
		now, now,
	); err != nil {
		return nil, fmt.Errorf("seed hidden workflow: %w", err)
	}

	wi := &model.WorkflowInstance{
		ID:              uuid.New().String(),
		ProjectID:       projectID,
		DefProjectID:    service.GlobalProjectID,
		WorkflowID:      delegateHiddenWorkflow,
		ScopeType:       "project",
		Status:          model.WorkflowInstanceActive,
		Origin:          model.RunOriginDelegate,
		OriginSessionID: callerSessionID,
	}
	if err := repo.NewWorkflowInstanceRepo(pool, s.config.Clock).Create(wi); err != nil {
		return nil, fmt.Errorf("create host instance: %w", err)
	}
	return wi, nil
}
