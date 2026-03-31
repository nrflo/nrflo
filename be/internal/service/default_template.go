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

// DefaultTemplateService handles default template business logic
type DefaultTemplateService struct {
	clock clock.Clock
	pool  *db.Pool
}

// NewDefaultTemplateService creates a new default template service
func NewDefaultTemplateService(pool *db.Pool, clk clock.Clock) *DefaultTemplateService {
	return &DefaultTemplateService{pool: pool, clock: clk}
}

// List returns all default templates ordered by name
func (s *DefaultTemplateService) List() ([]*model.DefaultTemplate, error) {
	rows, err := s.pool.Query(`
		SELECT id, name, template, readonly, created_at, updated_at
		FROM default_templates
		ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	templates := []*model.DefaultTemplate{}
	for rows.Next() {
		tmpl := &model.DefaultTemplate{}
		var createdAt, updatedAt string
		var readonly int

		err := rows.Scan(&tmpl.ID, &tmpl.Name, &tmpl.Template, &readonly, &createdAt, &updatedAt)
		if err != nil {
			return nil, err
		}

		tmpl.Readonly = readonly != 0
		tmpl.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		tmpl.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		templates = append(templates, tmpl)
	}

	return templates, nil
}

// Get retrieves a single default template
func (s *DefaultTemplateService) Get(id string) (*model.DefaultTemplate, error) {
	tmpl := &model.DefaultTemplate{}
	var createdAt, updatedAt string
	var readonly int

	err := s.pool.QueryRow(`
		SELECT id, name, template, readonly, created_at, updated_at
		FROM default_templates
		WHERE id = ?`, id).Scan(
		&tmpl.ID, &tmpl.Name, &tmpl.Template, &readonly, &createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("default template not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	tmpl.Readonly = readonly != 0
	tmpl.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	tmpl.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return tmpl, nil
}

// Create creates a new default template (always non-readonly)
func (s *DefaultTemplateService) Create(req *types.DefaultTemplateCreateRequest) (*model.DefaultTemplate, error) {
	if req.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if req.Template == "" {
		return nil, fmt.Errorf("template is required")
	}

	now := s.clock.Now().UTC().Format(time.RFC3339Nano)

	_, err := s.pool.Exec(`
		INSERT INTO default_templates (id, name, template, readonly, created_at, updated_at)
		VALUES (?, ?, ?, 0, ?, ?)`,
		req.ID, req.Name, req.Template, now, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") || strings.Contains(err.Error(), "already exists") {
			return nil, fmt.Errorf("default template already exists: %s", req.ID)
		}
		return nil, err
	}

	ts, _ := time.Parse(time.RFC3339Nano, now)
	return &model.DefaultTemplate{
		ID:        req.ID,
		Name:      req.Name,
		Template:  req.Template,
		Readonly:  false,
		CreatedAt: ts,
		UpdatedAt: ts,
	}, nil
}

// Update updates a default template (rejects readonly templates)
func (s *DefaultTemplateService) Update(id string, req *types.DefaultTemplateUpdateRequest) error {
	// Check if template exists and is not readonly
	tmpl, err := s.Get(id)
	if err != nil {
		return err
	}
	if tmpl.Readonly {
		return fmt.Errorf("cannot modify readonly template: %s", id)
	}

	updates := []string{}
	args := []interface{}{}

	if req.Name != nil {
		updates = append(updates, "name = ?")
		args = append(args, *req.Name)
	}
	if req.Template != nil {
		updates = append(updates, "template = ?")
		args = append(args, *req.Template)
	}

	if len(updates) == 0 {
		return nil
	}

	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
	updates = append(updates, "updated_at = ?")
	args = append(args, now)
	args = append(args, id)

	query := "UPDATE default_templates SET " + strings.Join(updates, ", ") + " WHERE id = ?"
	_, err = s.pool.Exec(query, args...)
	return err
}

// Delete deletes a default template (rejects readonly templates)
func (s *DefaultTemplateService) Delete(id string) error {
	tmpl, err := s.Get(id)
	if err != nil {
		return err
	}
	if tmpl.Readonly {
		return fmt.Errorf("cannot delete readonly template: %s", id)
	}

	_, err = s.pool.Exec("DELETE FROM default_templates WHERE id = ?", id)
	return err
}
