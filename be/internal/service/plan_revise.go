package service

import (
	"context"
	"encoding/json"
	"fmt"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

func (s *PlanService) reviseWithManifest(instanceID, projectID, workflowID string, raw json.RawMessage) (*model.PlanRevision, error) {
	m, err := ParsePlanManifest(raw)
	if err != nil {
		return nil, err
	}
	if err := ValidatePlanManifest(s.pool, projectID, workflowID, m); err != nil {
		return nil, err
	}
	// A caller-edited manifest (revise_plan tool, or the UI) is rejected
	// outright when it binds too many premium nodes — reviseWithPlanner
	// deliberately skips this (planner output is handled at the approve
	// boundary instead, see plan.go Approve / plan_approve_auto.go).
	if _, _, err := EnforcePremiumWorkerCap(s.pool, s.clock, projectID, workflowID, m, true); err != nil {
		return nil, err
	}
	canonical, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	revNum, err := s.planRepo.Append(instanceID, string(canonical), HashManifest(m), model.PlanAuthorCaller, "", m.Goal)
	if err != nil {
		return nil, err
	}
	return s.planRepo.GetRevision(instanceID, revNum)
}

func (s *PlanService) reviseWithPlanner(ctx context.Context, instanceID string, headExists bool, head *model.WorkflowPlan, req types.PlanReviseRequest) (*model.PlanRevision, error) {
	if s.runner == nil {
		return nil, fmt.Errorf("plan: no planner runner configured")
	}

	goal := req.Goal
	if goal == "" && headExists {
		goal = head.Goal
	}
	if goal == "" {
		return nil, fmt.Errorf("goal is required to start a plan")
	}

	prevManifest := ""
	if headExists && head.LatestRevision > 0 {
		if rev, err := s.planRepo.GetRevision(instanceID, head.LatestRevision); err == nil {
			prevManifest = rev.Manifest
		}
	}

	sessionID, err := s.runner.RunPlanner(ctx, instanceID, PlannerInput{
		Goal:             goal,
		Feedback:         req.Feedback,
		Answers:          req.Answers,
		PreviousManifest: prevManifest,
	})
	if err != nil {
		return nil, err
	}

	// RunPlanner blocks until the child session settles with a
	// FindingsService.Emit-validated `_workflow_plan` finding, so this read is
	// guaranteed to succeed on a nil error above. The finding is left in place
	// as immutable audit (unlike consult, which deletes _consult_answer).
	findings, err := repo.NewFindingRepo(s.pool, s.clock).GetOwn("session", sessionID)
	if err != nil {
		return nil, fmt.Errorf("plan: read planner findings: %w", err)
	}
	raw, ok := findings[WorkflowPlanFindingKey]
	if !ok {
		return nil, fmt.Errorf("plan: planner session %s did not write %s", sessionID, WorkflowPlanFindingKey)
	}
	m, err := ParsePlanManifest(raw)
	if err != nil {
		return nil, err
	}

	// The stale check at Revise entry ran before the (minutes-long) planner
	// session; a caller revision landed via revise_plan in the meantime means
	// the caller authored the plan deliberately — drop the planner's draft and
	// return the caller's head instead of bumping it (nrworkflow-4d0243).
	expected := 0
	if headExists {
		expected = head.LatestRevision
	}
	if current, cerr := s.planRepo.GetHead(instanceID); cerr == nil && current.LatestRevision != expected {
		return s.planRepo.GetRevision(instanceID, current.LatestRevision)
	}

	revNum, err := s.planRepo.Append(instanceID, string(raw), HashManifest(m), model.PlanAuthorPlanner, sessionID, m.Goal)
	if err != nil {
		return nil, err
	}
	return s.planRepo.GetRevision(instanceID, revNum)
}
