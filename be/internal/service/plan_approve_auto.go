package service

import (
	"context"
	"encoding/json"

	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
)

// ApproveAuto is the mode=auto counterpart to Approve, used by the plan
// boundary's unattended path (orchestrator draftPlanAndProceed): a
// premium-heavy drafted manifest is auto-downgraded (EnforcePremiumWorkerCap,
// canRevise=false) rather than rejected. When a downgrade happens, it is
// appended as a new caller-authored revision and a `_plan_premium_downgrade`
// warning finding is written, then the (now compliant) revision is approved
// through the normal Approve path so validation/materialization stay
// single-sourced.
func (s *PlanService) ApproveAuto(instanceID string, revision int) (*model.PlanRevision, error) {
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

	downgraded, warning, err := EnforcePremiumWorkerCap(s.pool, s.clock, wfi.ProjectID, wfi.WorkflowID, m, false)
	if err != nil {
		return nil, err
	}
	if warning == "" {
		return s.Approve(instanceID, revision)
	}

	canonical, err := json.Marshal(downgraded)
	if err != nil {
		return nil, err
	}
	newRevision, err := s.planRepo.Append(instanceID, string(canonical), HashManifest(downgraded), model.PlanAuthorCaller, "", downgraded.Goal)
	if err != nil {
		return nil, err
	}

	val, _ := json.Marshal(map[string]interface{}{
		"cap":        LoadDynwfMaxPremiumWorkers(s.pool, wfi.ProjectID),
		"downgraded": downgradedNodeIDs(m, downgraded),
		"message":    warning,
	})
	if err := repo.NewFindingRepo(s.pool, s.clock).Upsert("workflow_instance", instanceID, "_plan_premium_downgrade", val,
		repo.Denorm{ProjectID: wfi.ProjectID, WorkflowInstanceID: instanceID},
		repo.Actor{Source: "orchestrator"}); err != nil {
		// The downgrade itself still proceeds; only the audit finding failed.
		logger.Warn(context.Background(), "plan: write premium-downgrade warning finding", "instance_id", instanceID, "error", err)
	}

	return s.Approve(instanceID, newRevision)
}

// downgradedNodeIDs diffs before/after manifests (same shape, only Template
// fields may differ post-EnforcePremiumWorkerCap) and returns the ids of
// nodes whose template was rebound.
func downgradedNodeIDs(before, after PlanManifest) []string {
	var ids []string
	for li, layer := range before.Layers {
		if li >= len(after.Layers) {
			break
		}
		afterNodes := after.Layers[li].Nodes
		for ni, node := range layer.Nodes {
			if ni >= len(afterNodes) {
				break
			}
			if afterNodes[ni].Template != node.Template {
				ids = append(ids, node.ID)
			}
		}
	}
	return ids
}
