package service

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"be/internal/model"
	"be/internal/types"
)

func (s *ModelService) Update(id string, req types.ModelUpdateRequest) (*model.Model, error) {
	current, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if current.ReadOnly && (req.DisplayName != nil || req.CLIModel != nil || req.APIModel != nil ||
		req.CLIEfforts != nil || req.APIEfforts != nil || req.CLIContext != nil || req.APIContext != nil || req.Enabled != nil) {
		return nil, fmt.Errorf("only default_effort and fallback_models can be updated on built-in models")
	}
	cliModel, apiModel := current.CLIModel, current.APIModel
	cliEfforts, apiEfforts := current.CLIEfforts, current.APIEfforts
	defaultEffort := current.DefaultEffort
	if req.CLIModel != nil {
		cliModel = *req.CLIModel
	}
	if req.APIModel != nil {
		apiModel = *req.APIModel
	}
	if req.CLIEfforts != nil {
		cliEfforts, err = NormalizeSupportedEfforts(*req.CLIEfforts)
		if err != nil {
			return nil, fmt.Errorf("cli_efforts: %w", err)
		}
	}
	if req.APIEfforts != nil {
		apiEfforts, err = NormalizeSupportedEfforts(*req.APIEfforts)
		if err != nil {
			return nil, fmt.Errorf("api_efforts: %w", err)
		}
	}
	if req.DefaultEffort != nil {
		defaultEffort = *req.DefaultEffort
	}
	if err := validateModelModes(cliModel, apiModel); err != nil {
		return nil, err
	}
	apiOnly, _, err := s.resolveProvider(current.Provider)
	if err != nil {
		return nil, err
	}
	if err := validateProviderModes(apiOnly, cliModel); err != nil {
		return nil, err
	}
	if err := validateDefaultEffort(defaultEffort, cliModel, apiModel, cliEfforts, apiEfforts); err != nil {
		return nil, err
	}
	if req.CLIModel != nil && *req.CLIModel == "" && current.CLIModel != "" {
		if err := s.ModelInUseCheckForMode(id, "cli"); err != nil {
			return nil, err
		}
	}
	if req.APIModel != nil && *req.APIModel == "" && current.APIModel != "" {
		if err := s.ModelInUseCheckForMode(id, "api"); err != nil {
			return nil, err
		}
	}

	updates := []string{}
	args := []any{}
	add := func(column string, value any) { updates = append(updates, column+" = ?"); args = append(args, value) }
	if req.DisplayName != nil {
		add("display_name", *req.DisplayName)
	}
	if req.CLIModel != nil {
		add("cli_model", *req.CLIModel)
	}
	if req.APIModel != nil {
		add("api_model", *req.APIModel)
	}
	if req.CLIEfforts != nil {
		add("cli_efforts", marshalSupportedEfforts(cliEfforts))
	}
	if req.APIEfforts != nil {
		add("api_efforts", marshalSupportedEfforts(apiEfforts))
	}
	if req.CLIContext != nil {
		add("cli_context", *req.CLIContext)
	}
	if req.APIContext != nil {
		add("api_context", *req.APIContext)
	}
	if req.FallbackModels != nil {
		fallback, normalizeErr := normalizeFallbackModels(current.Provider, *req.FallbackModels)
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		add("fallback_models", fallback)
	}
	if req.DefaultEffort != nil {
		add("default_effort", *req.DefaultEffort)
	}
	if req.Enabled != nil {
		if !*req.Enabled {
			if err := s.ModelInUseCheck(id); err != nil {
				return nil, err
			}
		}
		if *req.Enabled {
			add("enabled", 1)
		} else {
			add("enabled", 0)
		}
	}
	if len(updates) == 0 {
		return current, nil
	}
	add("updated_at", s.clock.Now().UTC().Format(time.RFC3339Nano))
	args = append(args, id)
	result, err := s.pool.Exec("UPDATE models SET "+strings.Join(updates, ", ")+" WHERE LOWER(id) = LOWER(?)", args...)
	if err != nil {
		return nil, err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return nil, fmt.Errorf("model not found: %s", id)
	}
	return s.Get(id)
}

func (s *ModelService) Delete(id string) error {
	var readOnly int
	err := s.pool.QueryRow("SELECT read_only FROM models WHERE LOWER(id) = LOWER(?)", id).Scan(&readOnly)
	if err == sql.ErrNoRows {
		return fmt.Errorf("model not found: %s", id)
	}
	if err != nil {
		return err
	}
	if readOnly == 1 {
		return fmt.Errorf("cannot delete system model: %s", id)
	}
	if err := s.ModelInUseCheck(id); err != nil {
		return err
	}
	_, err = s.pool.Exec("DELETE FROM models WHERE LOWER(id) = LOWER(?)", id)
	return err
}
