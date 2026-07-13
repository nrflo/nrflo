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
	revNum, err := s.planRepo.Append(instanceID, string(raw), HashManifest(m), model.PlanAuthorPlanner, sessionID, m.Goal)
	if err != nil {
		return nil, err
	}
	return s.planRepo.GetRevision(instanceID, revNum)
}
