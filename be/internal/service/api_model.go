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

var validAPIProviders = map[string]bool{
	"anthropic": true,
	"openai":    true,
}

// validateAPIReasoningEffort checks that effort is one of the allowed levels and
// enforces that "xhigh" is only used with Anthropic Opus 4.7 models.
func validateAPIReasoningEffort(provider, mappedModel, effort string) error {
	if !validReasoningEfforts[effort] {
		return fmt.Errorf("invalid reasoning_effort %q: must be one of low, medium, high, xhigh, max", effort)
	}
	if effort == "xhigh" && (provider != "anthropic" || !strings.HasPrefix(mappedModel, "claude-opus-4-7")) {
		return fmt.Errorf("reasoning_effort 'xhigh' is only supported on Anthropic Opus 4.7 models")
	}
	return nil
}

// APIModelService handles API model business logic
type APIModelService struct {
	pool  *db.Pool
	clock clock.Clock
}

// NewAPIModelService creates a new API model service
func NewAPIModelService(pool *db.Pool, clk clock.Clock) *APIModelService {
	return &APIModelService{pool: pool, clock: clk}
}

// List retrieves all API models ordered by id
func (s *APIModelService) List() ([]*model.APIModel, error) {
	rows, err := s.pool.Query(`
		SELECT id, provider, display_name, mapped_model, reasoning_effort, context_length, read_only, enabled, created_at, updated_at
		FROM api_models
		ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	models := []*model.APIModel{}
	for rows.Next() {
		m, err := scanAPIModel(rows)
		if err != nil {
			return nil, err
		}
		models = append(models, m)
	}
	return models, nil
}

// ListEnabled retrieves only enabled API models ordered by id
func (s *APIModelService) ListEnabled() ([]*model.APIModel, error) {
	rows, err := s.pool.Query(`
		SELECT id, provider, display_name, mapped_model, reasoning_effort, context_length, read_only, enabled, created_at, updated_at
		FROM api_models
		WHERE enabled = 1
		ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	models := []*model.APIModel{}
	for rows.Next() {
		m, err := scanAPIModel(rows)
		if err != nil {
			return nil, err
		}
		models = append(models, m)
	}
	return models, nil
}

// Get retrieves a single API model by id (case-insensitive)
func (s *APIModelService) Get(id string) (*model.APIModel, error) {
	var createdAt, updatedAt string
	var readOnly, enabled int
	m := &model.APIModel{}

	err := s.pool.QueryRow(`
		SELECT id, provider, display_name, mapped_model, reasoning_effort, context_length, read_only, enabled, created_at, updated_at
		FROM api_models
		WHERE LOWER(id) = LOWER(?)`, id).Scan(
		&m.ID, &m.Provider, &m.DisplayName, &m.MappedModel,
		&m.ReasoningEffort, &m.ContextLength, &readOnly, &enabled,
		&createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("api model not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	m.ReadOnly = readOnly == 1
	m.Enabled = enabled == 1
	m.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	m.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return m, nil
}

// Create creates a new API model
func (s *APIModelService) Create(req types.APIModelCreateRequest) (*model.APIModel, error) {
	if req.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	if req.DisplayName == "" {
		return nil, fmt.Errorf("display_name is required")
	}
	if req.MappedModel == "" {
		return nil, fmt.Errorf("mapped_model is required")
	}
	if !validAPIProviders[req.Provider] {
		return nil, fmt.Errorf("invalid provider: must be one of anthropic, openai")
	}
	if err := validateAPIReasoningEffort(req.Provider, req.MappedModel, req.ReasoningEffort); err != nil {
		return nil, err
	}

	contextLength := req.ContextLength
	if contextLength == 0 {
		contextLength = 200000
	}

	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
	id := strings.ToLower(req.ID)

	_, err := s.pool.Exec(`
		INSERT INTO api_models (id, provider, display_name, mapped_model, reasoning_effort, context_length, read_only, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, 1, ?, ?)`,
		id, req.Provider, req.DisplayName, req.MappedModel, req.ReasoningEffort, contextLength, now, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return nil, fmt.Errorf("api model already exists: %s", id)
		}
		return nil, err
	}

	ts, _ := time.Parse(time.RFC3339Nano, now)
	return &model.APIModel{
		ID:              id,
		Provider:        req.Provider,
		DisplayName:     req.DisplayName,
		MappedModel:     req.MappedModel,
		ReasoningEffort: req.ReasoningEffort,
		ContextLength:   contextLength,
		ReadOnly:        false,
		Enabled:         true,
		CreatedAt:       ts,
		UpdatedAt:       ts,
	}, nil
}

// Update partially updates an API model
func (s *APIModelService) Update(id string, req types.APIModelUpdateRequest) (*model.APIModel, error) {
	current, err := s.Get(id)
	if err != nil {
		return nil, err
	}

	if current.ReadOnly {
		if req.DisplayName != nil || req.MappedModel != nil || req.ContextLength != nil || req.Enabled != nil {
			return nil, fmt.Errorf("only reasoning_effort can be updated on built-in models")
		}
	}

	if req.ReasoningEffort != nil || req.MappedModel != nil {
		mappedModel := current.MappedModel
		if req.MappedModel != nil {
			mappedModel = *req.MappedModel
		}
		effort := current.ReasoningEffort
		if req.ReasoningEffort != nil {
			effort = *req.ReasoningEffort
		}
		if err := validateAPIReasoningEffort(current.Provider, mappedModel, effort); err != nil {
			return nil, err
		}
	}

	updates := []string{}
	args := []interface{}{}

	if req.DisplayName != nil {
		updates = append(updates, "display_name = ?")
		args = append(args, *req.DisplayName)
	}
	if req.MappedModel != nil {
		updates = append(updates, "mapped_model = ?")
		args = append(args, *req.MappedModel)
	}
	if req.ReasoningEffort != nil {
		updates = append(updates, "reasoning_effort = ?")
		args = append(args, *req.ReasoningEffort)
	}
	if req.ContextLength != nil {
		updates = append(updates, "context_length = ?")
		args = append(args, *req.ContextLength)
	}
	if req.Enabled != nil {
		if !*req.Enabled {
			if err := s.ModelInUseCheck(id); err != nil {
				return nil, err
			}
		}
		enabledInt := 0
		if *req.Enabled {
			enabledInt = 1
		}
		updates = append(updates, "enabled = ?")
		args = append(args, enabledInt)
	}

	if len(updates) == 0 {
		return s.Get(id)
	}

	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
	updates = append(updates, "updated_at = ?")
	args = append(args, now)
	args = append(args, id)

	query := "UPDATE api_models SET " + strings.Join(updates, ", ") +
		" WHERE LOWER(id) = LOWER(?)"

	result, err := s.pool.Exec(query, args...)
	if err != nil {
		return nil, err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return nil, fmt.Errorf("api model not found: %s", id)
	}

	return s.Get(id)
}

// Delete deletes an API model (rejects read_only models)
func (s *APIModelService) Delete(id string) error {
	var readOnly int
	err := s.pool.QueryRow(
		"SELECT read_only FROM api_models WHERE LOWER(id) = LOWER(?)", id,
	).Scan(&readOnly)
	if err == sql.ErrNoRows {
		return fmt.Errorf("api model not found: %s", id)
	}
	if err != nil {
		return err
	}
	if readOnly == 1 {
		return fmt.Errorf("cannot delete system model: %s", id)
	}

	_, err = s.pool.Exec("DELETE FROM api_models WHERE LOWER(id) = LOWER(?)", id)
	return err
}

// IsValidModel checks if an enabled API model ID exists (case-insensitive)
func (s *APIModelService) IsValidModel(id string) (bool, error) {
	var exists int
	err := s.pool.QueryRow(
		"SELECT 1 FROM api_models WHERE LOWER(id) = LOWER(?) AND enabled = 1", id,
	).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ModelInUseCheck checks if a model is referenced by any agent or system agent definitions.
// Returns an error describing which agents use it, or nil if unused.
func (s *APIModelService) ModelInUseCheck(id string) error {
	type usage struct {
		ProjectID  string
		WorkflowID string
		AgentID    string
	}
	var usages []usage

	rows, err := s.pool.Query(`
		SELECT project_id, workflow_id, id FROM agent_definitions
		WHERE (LOWER(model) = LOWER(?) OR LOWER(low_consumption_model) = LOWER(?))
		  AND execution_mode = 'api'`,
		id, id)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var u usage
		if err := rows.Scan(&u.ProjectID, &u.WorkflowID, &u.AgentID); err != nil {
			return err
		}
		usages = append(usages, u)
	}

	sysRows, err := s.pool.Query(`
		SELECT id FROM system_agent_definitions
		WHERE LOWER(model) = LOWER(?) AND execution_mode = 'api'`, id)
	if err != nil {
		return err
	}
	defer sysRows.Close()
	for sysRows.Next() {
		var agentID string
		if err := sysRows.Scan(&agentID); err != nil {
			return err
		}
		usages = append(usages, usage{ProjectID: "system", AgentID: agentID})
	}

	if len(usages) == 0 {
		return nil
	}

	parts := make([]string, len(usages))
	for i, u := range usages {
		if u.WorkflowID != "" {
			parts[i] = u.ProjectID + "/" + u.WorkflowID + "/" + u.AgentID
		} else {
			parts[i] = u.ProjectID + "/" + u.AgentID
		}
	}
	return fmt.Errorf("model is in use by: %s", strings.Join(parts, ", "))
}

// scanAPIModel scans a row into an APIModel
func scanAPIModel(rows *sql.Rows) (*model.APIModel, error) {
	m := &model.APIModel{}
	var createdAt, updatedAt string
	var readOnly, enabled int

	err := rows.Scan(
		&m.ID, &m.Provider, &m.DisplayName, &m.MappedModel,
		&m.ReasoningEffort, &m.ContextLength, &readOnly, &enabled,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	m.ReadOnly = readOnly == 1
	m.Enabled = enabled == 1
	m.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	m.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return m, nil
}
