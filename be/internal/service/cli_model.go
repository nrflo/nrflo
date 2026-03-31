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
		SELECT id, cli_type, display_name, mapped_model, reasoning_effort, context_length, read_only, created_at, updated_at
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

// Get retrieves a single CLI model by id (case-insensitive)
func (s *CLIModelService) Get(id string) (*model.CLIModel, error) {
	var createdAt, updatedAt string
	var readOnly int
	m := &model.CLIModel{}

	err := s.pool.QueryRow(`
		SELECT id, cli_type, display_name, mapped_model, reasoning_effort, context_length, read_only, created_at, updated_at
		FROM cli_models
		WHERE LOWER(id) = LOWER(?)`, id).Scan(
		&m.ID, &m.CLIType, &m.DisplayName, &m.MappedModel,
		&m.ReasoningEffort, &m.ContextLength, &readOnly,
		&createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("cli model not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	m.ReadOnly = readOnly == 1
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
		CreatedAt:       ts,
		UpdatedAt:       ts,
	}, nil
}

// Update partially updates a CLI model
func (s *CLIModelService) Update(id string, req types.CLIModelUpdateRequest) (*model.CLIModel, error) {
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

// IsValidModel checks if a model ID exists (case-insensitive)
func (s *CLIModelService) IsValidModel(id string) (bool, error) {
	var exists int
	err := s.pool.QueryRow(
		"SELECT 1 FROM cli_models WHERE LOWER(id) = LOWER(?)", id,
	).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// scanCLIModel scans a row into a CLIModel
func scanCLIModel(rows *sql.Rows) (*model.CLIModel, error) {
	m := &model.CLIModel{}
	var createdAt, updatedAt string
	var readOnly int

	err := rows.Scan(
		&m.ID, &m.CLIType, &m.DisplayName, &m.MappedModel,
		&m.ReasoningEffort, &m.ContextLength, &readOnly,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	m.ReadOnly = readOnly == 1
	m.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	m.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return m, nil
}
