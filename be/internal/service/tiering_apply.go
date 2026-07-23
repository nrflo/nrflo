package service

import (
	"fmt"
	"strings"
	"time"

	"be/internal/types"
)

// ApplyForProject re-tiers the confirmed defs of one project. A def is only
// mutated when it is a mapped worker (ClassifyRole succeeds), not a
// consultant, not the hotfix implementor, has node_role='static', its
// current model still matches TierMap's OriginalSeedModel or is already
// tier-driven (i.e. it was never hand-customized), and it is named in
// confirmation.DefKeys or confirmation.ConfirmAll is set. Everything else is
// reported as skipped with a reason. Idempotent: a def already at its
// recommended tier/template/tools is reported "unchanged" and left alone.
// Applying writes `tier=N, model="", reasoning_effort=NULL` — no model
// stamping, so the chain resolver picks the actual model at spawn time.
func (s *TieringService) ApplyForProject(confirmation types.TieringApplyConfirmation) (*types.TieringApplyResult, error) {
	if confirmation.ProjectID == "" {
		return nil, validationErrorf("project_id is required")
	}

	confirmed := make(map[string]bool, len(confirmation.DefKeys))
	for _, k := range confirmation.DefKeys {
		confirmed[tieringDefKey(k.WorkflowID, k.DefID)] = true
	}

	raws, err := s.loadDefsForUpdate(confirmation.ProjectID)
	if err != nil {
		return nil, err
	}

	result := &types.TieringApplyResult{}
	now := s.clock.Now().UTC().Format(time.RFC3339Nano)

	for _, raw := range raws {
		role, ok := ClassifyRole(raw.workflowID, raw.id)
		if !ok {
			continue
		}
		outcome := types.TieringApplyOutcome{
			ProjectID: confirmation.ProjectID, WorkflowID: raw.workflowID, DefID: raw.id, Role: role,
		}
		target := TierMap[role]

		recChain, chainErr := resolveChain(s.pool, s.modelSvc, TierSpec{ID: raw.id, ExecutionMode: raw.executionMode, Tier: &target.Tier})
		var recommendedModel string
		if chainErr == nil && len(recChain) > 0 {
			recommendedModel = recChain[0].ModelID
		}

		if skip := tieringSkipReason(raw.tieringDefRowRaw, role, isTierCustomized(raw.model, recommendedModel, target)); skip != "" {
			outcome.Outcome = "skipped-" + skip
			outcome.Reason = tieringSkipReasonText[skip]
			result.Skipped = append(result.Skipped, outcome)
			continue
		}

		if !confirmation.ConfirmAll && !confirmed[tieringDefKey(raw.workflowID, raw.id)] {
			outcome.Outcome = "skipped-unconfirmed"
			outcome.Reason = "not included in the confirmation payload"
			result.Skipped = append(result.Skipped, outcome)
			continue
		}

		wantTools, toolsChanged := raw.tools, false
		if target.GrantsDelegation {
			wantTools, toolsChanged = grantDelegationTools(raw.tools)
		}

		if raw.tier.Valid && int(raw.tier.Int64) == target.Tier && raw.model == "" &&
			raw.systemTemplateID == target.SystemTemplateID && !toolsChanged {
			outcome.Outcome = "unchanged"
			result.Applied = append(result.Applied, outcome)
			continue
		}

		if chainErr != nil || recommendedModel == "" {
			outcome.Outcome = "skipped-customized"
			outcome.Reason = fmt.Sprintf("recommended tier %d has no resolvable chain: %v", target.Tier, chainErr)
			result.Skipped = append(result.Skipped, outcome)
			continue
		}

		if _, err := s.pool.Exec(`
			UPDATE agent_definitions SET tier = ?, model = '', reasoning_effort = NULL, system_template_id = ?, tools = ?, updated_at = ?
			WHERE project_id = ? AND workflow_id = ? AND id = ?`,
			target.Tier, target.SystemTemplateID, wantTools, now,
			confirmation.ProjectID, raw.workflowID, raw.id,
		); err != nil {
			return nil, fmt.Errorf("failed to apply tiering to %s/%s: %w", raw.workflowID, raw.id, err)
		}
		outcome.Outcome = "applied"
		result.Applied = append(result.Applied, outcome)
	}

	return result, nil
}

var tieringSkipReasonText = map[string]string{
	"consultant": "consultant defs are never modified",
	"hotfix":     "hotfix implementor is deliberately left untouched",
	"non-static": "node_role is not static (not part of the execution graph)",
	"customized": "current model differs from the original seed value",
}

func tieringDefKey(workflowID, defID string) string {
	return strings.ToLower(workflowID) + "/" + strings.ToLower(defID)
}

// tieringDefRowForUpdate extends tieringDefRowRaw with system_template_id,
// needed to detect the already-applied (idempotent) case.
type tieringDefRowForUpdate struct {
	tieringDefRowRaw
	systemTemplateID string
	tools            string
}

func (s *TieringService) loadDefsForUpdate(projectID string) ([]tieringDefRowForUpdate, error) {
	rows, err := s.pool.Query(`
		SELECT id, workflow_id, model, reasoning_effort, consultant, node_role, system_template_id, tools, tier, execution_mode
		FROM agent_definitions WHERE project_id = ?`, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to list agent definitions for project %s: %w", projectID, err)
	}
	defer rows.Close()

	var out []tieringDefRowForUpdate
	for rows.Next() {
		var raw tieringDefRowForUpdate
		if err := rows.Scan(&raw.id, &raw.workflowID, &raw.model, &raw.effort, &raw.consultant, &raw.nodeRole, &raw.systemTemplateID, &raw.tools, &raw.tier, &raw.executionMode); err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
	return out, rows.Err()
}
