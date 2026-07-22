package service

import (
	"database/sql"
	"fmt"

	"be/internal/types"
)

// revalidateModel re-checks the model/tier/reasoning-effort invariants for an
// update: the effective definition (after applying req on top of the stored
// row) must have a non-empty model override or a tier — and a non-empty
// model must still validate against the effective execution_mode and
// reasoning effort.
func (s *SystemAgentDefinitionService) revalidateModel(id string, req *types.SystemAgentDefUpdateRequest) error {
	if req.ExecutionMode == nil && req.Model == nil && req.ReasoningEffort == nil && req.Tier == nil {
		return nil
	}
	var mode, modelName string
	var effort sql.NullString
	var tier sql.NullInt64
	if err := s.pool.QueryRow(`SELECT execution_mode, model, reasoning_effort, tier
		FROM system_agent_definitions WHERE LOWER(id) = LOWER(?)`, id).Scan(&mode, &modelName, &effort, &tier); err != nil {
		return fmt.Errorf("failed to load system agent definition: %w", err)
	}
	if req.ExecutionMode != nil {
		mode = *req.ExecutionMode
	}
	if req.Model != nil {
		modelName = *req.Model
	}
	effectiveTier := tier.Valid
	if req.Tier != nil {
		effectiveTier = true
	}
	if modelName == "" && !effectiveTier {
		return validationErrorf("model or tier is required")
	}
	var effectiveEffort *string
	if effort.Valid {
		value := effort.String
		effectiveEffort = &value
	}
	if req.ReasoningEffort != nil {
		effectiveEffort = req.ReasoningEffort
	}
	if modelName == "" {
		return nil
	}
	valid, err := s.modelSvc.IsValidModelForMode(modelName, registryMode(mode))
	if err != nil {
		return fmt.Errorf("failed to validate model: %w", err)
	}
	if !valid {
		return fmt.Errorf("invalid model: %q", modelName)
	}
	return validateDefReasoningEffort(s.modelSvc, mode, modelName, effectiveEffort)
}
