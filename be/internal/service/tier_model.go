package service

import (
	"fmt"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/types"
)

// TierModelService manages the tier_models ordered fallback chains.
type TierModelService struct {
	pool     *db.Pool
	clock    clock.Clock
	modelSvc *ModelService
}

// NewTierModelService creates a new tier model service.
func NewTierModelService(pool *db.Pool, clk clock.Clock, modelSvc *ModelService) *TierModelService {
	return &TierModelService{pool: pool, clock: clk, modelSvc: modelSvc}
}

// List returns every tier_models row ordered by tier, then position.
func (s *TierModelService) List() ([]model.TierModel, error) {
	rows, err := s.pool.Query(
		`SELECT tier, position, provider, execution_mode, model_id, reasoning_effort
		 FROM tier_models ORDER BY tier ASC, position ASC`)
	if err != nil {
		return nil, fmt.Errorf("failed to list tier models: %w", err)
	}
	defer rows.Close()

	result := []model.TierModel{}
	for rows.Next() {
		var tm model.TierModel
		if err := rows.Scan(&tm.Tier, &tm.Position, &tm.Provider, &tm.ExecutionMode, &tm.ModelID, &tm.ReasoningEffort); err != nil {
			return nil, fmt.Errorf("failed to scan tier model: %w", err)
		}
		result = append(result, tm)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate tier models: %w", err)
	}
	return result, nil
}

// SetTierChain replaces the entire ordered chain for tier with entries,
// validating each entry's model and deriving provider from the model row.
// An empty entries slice clears the tier.
func (s *TierModelService) SetTierChain(tier int, entries []types.TierChainEntry) error {
	if tier < 1 || tier > 5 {
		return validationErrorf("tier must be between 1 and 5")
	}

	built := make([]model.TierModel, 0, len(entries))
	for i, entry := range entries {
		if entry.ModelID == "" {
			return validationErrorf("entry %d: model_id is required", i)
		}
		if entry.ExecutionMode != "" && entry.ExecutionMode != "cli_interactive" && entry.ExecutionMode != "api" {
			return validationErrorf("entry %d: invalid execution_mode %q", i, entry.ExecutionMode)
		}

		if entry.ExecutionMode == "" {
			// '' = inherit the agent's own mode at resolve time — the model
			// must be valid for BOTH registry modes so resolution can never
			// fail on mode grounds later, whichever mode it ends up inheriting.
			for _, mode := range []string{"cli", "api"} {
				valid, err := s.modelSvc.IsValidModelForMode(entry.ModelID, mode)
				if err != nil {
					return fmt.Errorf("failed to validate model: %w", err)
				}
				if !valid {
					return validationErrorf("entry %d: invalid model %q for mode %q (inherit-mode entries must be valid for both cli and api)", i, entry.ModelID, mode)
				}
			}
		} else {
			mode := registryMode(entry.ExecutionMode)
			valid, err := s.modelSvc.IsValidModelForMode(entry.ModelID, mode)
			if err != nil {
				return fmt.Errorf("failed to validate model: %w", err)
			}
			if !valid {
				return validationErrorf("entry %d: invalid model %q for mode %q", i, entry.ModelID, entry.ExecutionMode)
			}
		}

		row, err := s.modelSvc.Get(entry.ModelID)
		if err != nil {
			return fmt.Errorf("failed to load model row: %w", err)
		}

		effort := entry.ReasoningEffort
		if effort == "" {
			effort = row.DefaultEffort
		}

		built = append(built, model.TierModel{
			Tier:            tier,
			Position:        i,
			Provider:        row.Provider,
			ExecutionMode:   entry.ExecutionMode,
			ModelID:         entry.ModelID,
			ReasoningEffort: effort,
		})
	}

	return db.WithBusyRetry(func() error {
		return s.setTierChainOnce(tier, built)
	})
}

func (s *TierModelService) setTierChainOnce(tier int, entries []model.TierModel) error {
	tx, err := s.pool.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec("DELETE FROM tier_models WHERE tier = ?", tier); err != nil {
		return fmt.Errorf("failed to clear tier chain: %w", err)
	}

	for _, e := range entries {
		if _, err := tx.Exec(
			`INSERT INTO tier_models (tier, position, provider, execution_mode, model_id, reasoning_effort)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			e.Tier, e.Position, e.Provider, e.ExecutionMode, e.ModelID, e.ReasoningEffort,
		); err != nil {
			return fmt.Errorf("failed to insert tier chain entry: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit tier chain: %w", err)
	}
	return nil
}
