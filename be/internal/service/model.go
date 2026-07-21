package service

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/types"
)

const modelColumns = `id, provider, display_name, cli_model, api_model,
	cli_efforts, api_efforts, cli_context, api_context, fallback_models,
	default_effort, read_only, enabled, created_at, updated_at,
	price_in, price_out, price_cache_write, price_cache_read, release_date`

var validModelProviders = map[string]bool{"anthropic": true, "openai": true, "openrouter": true}

// ModelService owns the unified provider model registry.
type ModelService struct {
	pool  *db.Pool
	clock clock.Clock
}

func NewModelService(pool *db.Pool, clk clock.Clock) *ModelService {
	return &ModelService{pool: pool, clock: clk}
}

func (s *ModelService) List() ([]*model.Model, error) {
	return s.list("")
}

func (s *ModelService) ListEnabled() ([]*model.Model, error) {
	return s.list("WHERE enabled = 1")
}

func (s *ModelService) list(where string) ([]*model.Model, error) {
	rows, err := s.pool.Query("SELECT " + modelColumns + " FROM models " + where + " ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []*model.Model{}
	for rows.Next() {
		m, scanErr := scanModel(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

func (s *ModelService) Get(id string) (*model.Model, error) {
	m, err := scanModel(s.pool.QueryRow(
		"SELECT "+modelColumns+" FROM models WHERE LOWER(id) = LOWER(?)", id))
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("model not found: %s", id)
	}
	return m, err
}

func normalizeFallbackModels(provider, raw string) (string, error) {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return "", nil
	}
	if provider != "anthropic" {
		return "", fmt.Errorf("fallback_models is only supported for anthropic models")
	}
	if len(out) > 3 {
		return "", fmt.Errorf("fallback_models accepts at most 3 models")
	}
	return strings.Join(out, ","), nil
}

func validateModelModes(cliModel, apiModel string) error {
	if cliModel == "" && apiModel == "" {
		return fmt.Errorf("cli_model or api_model is required")
	}
	return nil
}

// validateProviderModes enforces provider-specific mode restrictions.
// openrouter is API-mode only: it has no CLI adapter, so a non-empty
// cli_model is rejected.
func validateProviderModes(provider, cliModel string) error {
	if provider == "openrouter" && cliModel != "" {
		return fmt.Errorf("openrouter models are API-mode only: cli_model must be empty")
	}
	return nil
}

func validateDefaultEffort(effort, cliModel, apiModel string, cliEfforts, apiEfforts []string) error {
	if effort == "" {
		return nil
	}
	if cliModel != "" {
		if err := ValidateEffortAllowed(effort, cliEfforts); err != nil {
			return fmt.Errorf("cli default_effort: %w", err)
		}
	}
	if apiModel != "" {
		if err := ValidateEffortAllowed(effort, apiEfforts); err != nil {
			return fmt.Errorf("api default_effort: %w", err)
		}
	}
	return nil
}

func (s *ModelService) Create(req types.ModelCreateRequest) (*model.Model, error) {
	if strings.TrimSpace(req.ID) == "" {
		return nil, fmt.Errorf("id is required")
	}
	if req.DisplayName == "" {
		return nil, fmt.Errorf("display_name is required")
	}
	if !validModelProviders[req.Provider] {
		return nil, fmt.Errorf("invalid provider: must be one of anthropic, openai, openrouter")
	}
	if err := validateModelModes(req.CLIModel, req.APIModel); err != nil {
		return nil, err
	}
	if err := validateProviderModes(req.Provider, req.CLIModel); err != nil {
		return nil, err
	}
	cliEfforts, err := NormalizeSupportedEfforts(req.CLIEfforts)
	if err != nil {
		return nil, fmt.Errorf("cli_efforts: %w", err)
	}
	apiEfforts, err := NormalizeSupportedEfforts(req.APIEfforts)
	if err != nil {
		return nil, fmt.Errorf("api_efforts: %w", err)
	}
	if err := validateDefaultEffort(req.DefaultEffort, req.CLIModel, req.APIModel, cliEfforts, apiEfforts); err != nil {
		return nil, err
	}
	fallback, err := normalizeFallbackModels(req.Provider, req.FallbackModels)
	if err != nil {
		return nil, err
	}
	if req.CLIContext == 0 {
		req.CLIContext = 200000
	}
	if req.APIContext == 0 {
		req.APIContext = 200000
	}
	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
	id := strings.ToLower(req.ID)
	_, err = s.pool.Exec(`INSERT INTO models (`+modelColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 1, ?, ?, NULL, NULL, NULL, NULL, NULL)`, id, req.Provider,
		req.DisplayName, req.CLIModel, req.APIModel, marshalSupportedEfforts(cliEfforts),
		marshalSupportedEfforts(apiEfforts), req.CLIContext, req.APIContext, fallback,
		req.DefaultEffort, now, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return nil, fmt.Errorf("model already exists: %s", id)
		}
		return nil, err
	}
	return s.Get(id)
}

// IsValidModelForMode checks that an enabled row supports cli or api mode.
func (s *ModelService) IsValidModelForMode(id, mode string) (bool, error) {
	column := ""
	switch mode {
	case "cli":
		column = "cli_model"
	case "api":
		column = "api_model"
	default:
		return false, fmt.Errorf("invalid model mode: %s", mode)
	}
	var exists int
	err := s.pool.QueryRow("SELECT 1 FROM models WHERE LOWER(id) = LOWER(?) AND enabled = 1 AND "+column+" <> ''", id).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}
