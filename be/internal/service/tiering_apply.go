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
// current model still matches TierMap's OriginalSeedModel (i.e. it was never
// hand-customized), and it is named in confirmation.DefKeys or
// confirmation.ConfirmAll is set. Everything else is reported as skipped
// with a reason. Idempotent: a def already at its recommended
// model/effort/template is reported "unchanged" and left alone.
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

		if skip := tieringSkipReason(raw.tieringDefRowRaw, role, isTierCustomized(raw.model, target)); skip != "" {
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

		if strings.EqualFold(raw.model, target.RecommendedModel) &&
			raw.effort.String == target.RecommendedEffort &&
			raw.systemTemplateID == target.SystemTemplateID {
			outcome.Outcome = "unchanged"
			result.Applied = append(result.Applied, outcome)
			continue
		}

		if valid, vErr := s.modelSvc.IsValidModelForMode(target.RecommendedModel, registryMode("cli_interactive")); vErr != nil || !valid {
			outcome.Outcome = "skipped-customized"
			outcome.Reason = fmt.Sprintf("recommended model %q failed validation for this def's mode", target.RecommendedModel)
			result.Skipped = append(result.Skipped, outcome)
			continue
		}
		effort := target.RecommendedEffort
		var effortPtr *string
		if effort != "" {
			effortPtr = &effort
		}
		if vErr := validateDefReasoningEffort(s.modelSvc, "cli_interactive", target.RecommendedModel, effortPtr); vErr != nil {
			outcome.Outcome = "skipped-customized"
			outcome.Reason = vErr.Error()
			result.Skipped = append(result.Skipped, outcome)
			continue
		}

		if _, err := s.pool.Exec(`
			UPDATE agent_definitions SET model = ?, reasoning_effort = ?, system_template_id = ?, updated_at = ?
			WHERE project_id = ? AND workflow_id = ? AND id = ?`,
			target.RecommendedModel, target.RecommendedEffort, target.SystemTemplateID, now,
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
}

func (s *TieringService) loadDefsForUpdate(projectID string) ([]tieringDefRowForUpdate, error) {
	rows, err := s.pool.Query(`
		SELECT id, workflow_id, model, reasoning_effort, consultant, node_role, system_template_id
		FROM agent_definitions WHERE project_id = ?`, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to list agent definitions for project %s: %w", projectID, err)
	}
	defer rows.Close()

	var out []tieringDefRowForUpdate
	for rows.Next() {
		var raw tieringDefRowForUpdate
		if err := rows.Scan(&raw.id, &raw.workflowID, &raw.model, &raw.effort, &raw.consultant, &raw.nodeRole, &raw.systemTemplateID); err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
	return out, rows.Err()
}
