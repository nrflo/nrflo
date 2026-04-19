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

var validCLITypes = map[string]bool{
	"claude":   true,
	"opencode": true,
	"codex":    true,
}

var validReasoningEfforts = map[string]bool{
	"":       true,
	"low":    true,
	"medium": true,
	"high":   true,
	"xhigh":  true,
	"max":    true,
}

// validateReasoningEffort checks that effort is one of the allowed levels and
// enforces that "xhigh" is only used with Claude Opus 4.7 models.
func validateReasoningEffort(cliType, mappedModel, effort string) error {
	if !validReasoningEfforts[effort] {
		return fmt.Errorf("invalid reasoning_effort %q: must be one of low, medium, high, xhigh, max", effort)
	}
	if effort == "xhigh" && cliType == "claude" && !strings.HasPrefix(mappedModel, "claude-opus-4-7") {
		return fmt.Errorf("reasoning_effort 'xhigh' is only supported on Opus 4.7 Claude models")
	}
	return nil
}

// CLIModelService handles CLI model business logic
type CLIModelService struct {
	pool  *db.Pool
	clock clock.Clock
}

// NewCLIModelService creates a new CLI model service
func NewCLIModelService(pool *db.Pool, clk clock.Clock) *CLIModelService {
	return &CLIModelService{pool: pool, clock: clk}
}

// List retrieves all CLI models ordered by id
func (s *CLIModelService) List() ([]*model.CLIModel, error) {
	rows, err := s.pool.Query(`
		SELECT id, cli_type, display_name, mapped_model, reasoning_effort, context_length, read_only, enabled, created_at, updated_at
		FROM cli_models
		ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	models := []*model.CLIModel{}
	for rows.Next() {
		m, err := scanCLIModel(rows)
		if err != nil {
			return nil, err
		}
		models = append(models, m)
	}
	return models, nil
}

// ListEnabled retrieves only enabled CLI models ordered by id
func (s *CLIModelService) ListEnabled() ([]*model.CLIModel, error) {
	rows, err := s.pool.Query(`
		SELECT id, cli_type, display_name, mapped_model, reasoning_effort, context_length, read_only, enabled, created_at, updated_at
		FROM cli_models
		WHERE enabled = 1
		ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	models := []*model.CLIModel{}
	for rows.Next() {
		m, err := scanCLIModel(rows)
		if err != nil {
			return nil, err
		}
		models = append(models, m)
	}
	return models, nil
}

// Get retrieves a single CLI model by id (case-insensitive)
func (s *CLIModelService) Get(id string) (*model.CLIModel, error) {
	var createdAt, updatedAt string
	var readOnly, enabled int
	m := &model.CLIModel{}

	err := s.pool.QueryRow(`
		SELECT id, cli_type, display_name, mapped_model, reasoning_effort, context_length, read_only, enabled, created_at, updated_at
		FROM cli_models
		WHERE LOWER(id) = LOWER(?)`, id).Scan(
		&m.ID, &m.CLIType, &m.DisplayName, &m.MappedModel,
		&m.ReasoningEffort, &m.ContextLength, &readOnly, &enabled,
		&createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("cli model not found: %s", id)
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

// Create creates a new CLI model
func (s *CLIModelService) Create(req types.CLIModelCreateRequest) (*model.CLIModel, error) {
	if req.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	if req.DisplayName == "" {
		return nil, fmt.Errorf("display_name is required")
	}
	if req.MappedModel == "" {
		return nil, fmt.Errorf("mapped_model is required")
	}
	if !validCLITypes[req.CLIType] {
		return nil, fmt.Errorf("invalid cli_type: must be one of claude, opencode, codex")
	}
	if err := validateReasoningEffort(req.CLIType, req.MappedModel, req.ReasoningEffort); err != nil {
		return nil, err
	}

	contextLength := req.ContextLength
	if contextLength == 0 {
		contextLength = 200000
	}

	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
	id := strings.ToLower(req.ID)

	_, err := s.pool.Exec(`
		INSERT INTO cli_models (id, cli_type, display_name, mapped_model, reasoning_effort, context_length, read_only, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?)`,
		id, req.CLIType, req.DisplayName, req.MappedModel, req.ReasoningEffort, contextLength, now, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return nil, fmt.Errorf("cli model already exists: %s", id)
		}
		return nil, err
	}

	ts, _ := time.Parse(time.RFC3339Nano, now)
	return &model.CLIModel{
		ID:              id,
		CLIType:         req.CLIType,
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

// Update partially updates a CLI model
func (s *CLIModelService) Update(id string, req types.CLIModelUpdateRequest) (*model.CLIModel, error) {
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
		if err := validateReasoningEffort(current.CLIType, mappedModel, effort); err != nil {
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

	query := "UPDATE cli_models SET " + strings.Join(updates, ", ") +
		" WHERE LOWER(id) = LOWER(?)"

	result, err := s.pool.Exec(query, args...)
	if err != nil {
		return nil, err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return nil, fmt.Errorf("cli model not found: %s", id)
	}

	return s.Get(id)
}

// Delete deletes a CLI model (rejects read_only models)
func (s *CLIModelService) Delete(id string) error {
	var readOnly int
	err := s.pool.QueryRow(
		"SELECT read_only FROM cli_models WHERE LOWER(id) = LOWER(?)", id,
	).Scan(&readOnly)
	if err == sql.ErrNoRows {
		return fmt.Errorf("cli model not found: %s", id)
	}
	if err != nil {
		return err
	}
	if readOnly == 1 {
		return fmt.Errorf("cannot delete system model: %s", id)
	}

	_, err = s.pool.Exec("DELETE FROM cli_models WHERE LOWER(id) = LOWER(?)", id)
	return err
}

// IsValidModel checks if an enabled model ID exists (case-insensitive)
func (s *CLIModelService) IsValidModel(id string) (bool, error) {
	var exists int
	err := s.pool.QueryRow(
		"SELECT 1 FROM cli_models WHERE LOWER(id) = LOWER(?) AND enabled = 1", id,
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
func (s *CLIModelService) ModelInUseCheck(id string) error {
	type usage struct {
		ProjectID  string
		WorkflowID string
		AgentID    string
	}
	var usages []usage

	// Check agent_definitions (model and low_consumption_model)
	rows, err := s.pool.Query(`
		SELECT project_id, workflow_id, id FROM agent_definitions
		WHERE LOWER(model) = LOWER(?) OR LOWER(low_consumption_model) = LOWER(?)`,
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

	// Check system_agent_definitions (model only, no low_consumption_model column)
	sysRows, err := s.pool.Query(`
		SELECT id FROM system_agent_definitions
		WHERE LOWER(model) = LOWER(?)`, id)
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

// scanCLIModel scans a row into a CLIModel
func scanCLIModel(rows *sql.Rows) (*model.CLIModel, error) {
	m := &model.CLIModel{}
	var createdAt, updatedAt string
	var readOnly, enabled int

	err := rows.Scan(
		&m.ID, &m.CLIType, &m.DisplayName, &m.MappedModel,
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
