package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

// ErrStalePlanRevision is returned by Revise/Approve when the caller's
// req.Revision does not match the current head's latest_revision.
var ErrStalePlanRevision = errors.New("plan: revision is stale")

// ErrPlanNotDraft is returned when Revise/Approve is attempted against a plan
// head that is no longer in draft state.
var ErrPlanNotDraft = errors.New("plan: not in draft state")

// PlannerInput carries the caller-supplied planner context. Goal/Instructions
// resolution beyond this (workflow user_instructions, template library
// rendering) happens inside the PlannerRunner implementation, which has
// direct DB/orchestrator access.
type PlannerInput struct {
	Goal             string
	Feedback         string
	Answers          []types.PlanAnswer
	PreviousManifest string
}

// PlannerRunner spawns a fresh one-off planner child session under the
// caller's workflow instance and returns its session id once it settles with
// a validated `_workflow_plan` finding. Implemented by
// *orchestrator.Orchestrator — service must not import orchestrator (the
// dependency runs the other way), so api.Server wires the concrete value in.
type PlannerRunner interface {
	RunPlanner(ctx context.Context, instanceID string, in PlannerInput) (sessionID string, err error)
}

// PlanDraft is the read model for GET .../plan: the mutable head, the latest
// revision's parsed manifest (nil until a first revision exists), and the
// template library available to bind plan nodes against.
type PlanDraft struct {
	Head      *model.WorkflowPlan `json:"head"`
	Manifest  *PlanManifest       `json:"manifest,omitempty"`
	Questions []PlanQuestion      `json:"questions,omitempty"`
	Templates []PlanTemplate      `json:"templates"`
}

// PlanService orchestrates the plan lifecycle: draft state, revise (edited
// manifest or planner re-run), approve (revision-pinned), cancel, and the
// draft TTL sweep. Constructed per-request from s.pool + s.clock, same as
// every other service (see service/CLAUDE.md Constructor Pattern) — never
// stored on api.Server.
type PlanService struct {
	pool     *db.Pool
	clock    clock.Clock
	planRepo *repo.PlanRepo
	runner   PlannerRunner
}

// NewPlanService creates a new PlanService.
func NewPlanService(pool *db.Pool, clk clock.Clock, runner PlannerRunner) *PlanService {
	return &PlanService{pool: pool, clock: clk, planRepo: repo.NewPlanRepo(pool, clk), runner: runner}
}

// GetDraft returns the plan head, its latest manifest (if any), and the
// template library resolvable for the workflow instance's project+workflow.
func (s *PlanService) GetDraft(instanceID string) (*PlanDraft, error) {
	head, err := s.planRepo.GetHead(instanceID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no plan for workflow instance %s", instanceID)
	}
	if err != nil {
		return nil, err
	}

	wfi, err := repo.NewWorkflowInstanceRepo(s.pool, s.clock).Get(instanceID)
	if err != nil {
		return nil, err
	}
	templates, err := AllowedTemplates(s.pool, wfi.ProjectID, wfi.WorkflowID)
	if err != nil {
		return nil, err
	}

	draft := &PlanDraft{Head: head, Templates: templates}
	if head.LatestRevision > 0 {
		rev, err := s.planRepo.GetRevision(instanceID, head.LatestRevision)
		if err != nil {
			return nil, err
		}
		m, err := ParsePlanManifest(json.RawMessage(rev.Manifest))
		if err != nil {
			return nil, err
		}
		draft.Manifest = &m
		draft.Questions = m.Questions
	}
	return draft, nil
}

// ListRevisions returns every revision for a workflow instance's plan.
func (s *PlanService) ListRevisions(instanceID string) ([]*model.PlanRevision, error) {
	return s.planRepo.ListRevisions(instanceID)
}

// Revise appends a new plan revision, either from a caller-edited manifest
// (validated then stored as-is, author='caller') or by re-running the planner
// agent from feedback/answers (author='planner'). Rejects a stale revision
// (req.Revision != head.LatestRevision) and a non-draft head.
func (s *PlanService) Revise(ctx context.Context, instanceID string, req types.PlanReviseRequest) (*model.PlanRevision, error) {
	head, err := s.planRepo.GetHead(instanceID)
	headExists := true
	if err == sql.ErrNoRows {
		headExists, err = false, nil
	}
	if err != nil {
		return nil, err
	}
	if headExists {
		if head.Status != model.PlanStatusDraft {
			return nil, ErrPlanNotDraft
		}
		if req.Revision != head.LatestRevision {
			return nil, ErrStalePlanRevision
		}
	} else if req.Revision != 0 {
		return nil, ErrStalePlanRevision
	}

	wfi, err := repo.NewWorkflowInstanceRepo(s.pool, s.clock).Get(instanceID)
	if err != nil {
		return nil, err
	}

	var rev *model.PlanRevision
	if len(req.Manifest) > 0 {
		rev, err = s.reviseWithManifest(instanceID, wfi.ProjectID, wfi.WorkflowID, req.Manifest)
	} else {
		rev, err = s.reviseWithPlanner(ctx, instanceID, headExists, head, req)
	}
	if err != nil {
		return nil, err
	}
	s.syncPlanSuspendedStatus(ctx, instanceID)
	return rev, nil
}

// Approve transitions a draft plan to approved at the given revision.
// Revision-pinned (stale -> ErrStalePlanRevision) and re-validates the
// manifest before approving — a referenced template's model may have been
// disabled since the draft was made. Open questions never block approval.
// Also rejects a manifest binding more than dynwf_max_premium_workers nodes to
// a premium-tier template (EnforcePremiumWorkerCap, canRevise=true) — the
// unattended counterpart is ApproveAuto, which downgrades instead of rejects.
func (s *PlanService) Approve(instanceID string, revision int) (*model.PlanRevision, error) {
	head, err := s.planRepo.GetHead(instanceID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no plan for workflow instance %s", instanceID)
	}
	if err != nil {
		return nil, err
	}
	if head.Status != model.PlanStatusDraft {
		return nil, ErrPlanNotDraft
	}
	if revision != head.LatestRevision {
		return nil, ErrStalePlanRevision
	}

	rev, err := s.planRepo.GetRevision(instanceID, revision)
	if err != nil {
		return nil, err
	}
	wfi, err := repo.NewWorkflowInstanceRepo(s.pool, s.clock).Get(instanceID)
	if err != nil {
		return nil, err
	}
	m, err := ParsePlanManifest(json.RawMessage(rev.Manifest))
	if err != nil {
		return nil, err
	}
	if err := ValidatePlanManifest(s.pool, wfi.ProjectID, wfi.WorkflowID, m); err != nil {
		return nil, err
	}
	if _, _, err := EnforcePremiumWorkerCap(s.pool, s.clock, wfi.ProjectID, wfi.WorkflowID, m, true); err != nil {
		return nil, err
	}

	if err := s.planRepo.Approve(instanceID, revision); err != nil {
		if errors.Is(err, repo.ErrPlanStaleOrNotDraft) {
			return nil, ErrStalePlanRevision
		}
		return nil, err
	}

	// Materialize in the same request so the caller sees a materialization
	// failure (e.g. a template's model disabled since draft) as a 4xx instead
	// of a mid-run failure at the plan boundary. Idempotent — the boundary may
	// call it again for the same approved revision.
	if _, err := s.Materialize(instanceID); err != nil {
		return nil, err
	}
	return rev, nil
}

// Cancel transitions a draft plan to cancelled.
func (s *PlanService) Cancel(instanceID string) error {
	return s.planRepo.Cancel(instanceID)
}

// SweepExpiredDrafts cancels every workflow_plans row still in 'draft' whose
// updated_at is older than the global plan_draft_ttl_min config (project
// overrides are not consulted here — same fixed-window precedent as
// reapStaleUploads). Returns the number of drafts cancelled.
func (s *PlanService) SweepExpiredDrafts(now time.Time) (int, error) {
	ttlMin := SubworkflowCap(s.pool, "", PlanDraftTTLMinKey, DefaultPlanDraftTTLMin)
	cutoff := now.Add(-time.Duration(ttlMin) * time.Minute).UTC().Format(time.RFC3339Nano)
	ids, err := s.planRepo.ListExpiredDrafts(cutoff)
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		if err := s.planRepo.Cancel(id); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}
