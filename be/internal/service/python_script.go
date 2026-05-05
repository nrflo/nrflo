package service

import (
	"fmt"
	"strings"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/id"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

// PythonScriptService handles python script business logic
type PythonScriptService struct {
	pool  *db.Pool
	clock clock.Clock
}

// NewPythonScriptService creates a new python script service
func NewPythonScriptService(pool *db.Pool, clk clock.Clock) *PythonScriptService {
	return &PythonScriptService{pool: pool, clock: clk}
}

// Create creates a new python script for a project
func (s *PythonScriptService) Create(projectID string, req *types.PythonScriptCreateRequest) (*model.PythonScript, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	gen := id.New("ps")
	scriptID, err := gen.Generate()
	if err != nil {
		return nil, fmt.Errorf("failed to generate id: %w", err)
	}

	r := repo.NewPythonScriptRepo(s.pool, s.clock)
	script := &model.PythonScript{
		ID:          scriptID,
		ProjectID:   strings.ToLower(projectID),
		Name:        req.Name,
		Description: req.Description,
		Code:        req.Code,
	}

	if err := r.Create(script); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") || strings.Contains(err.Error(), "already exists") {
			return nil, fmt.Errorf("python script already exists: %s", scriptID)
		}
		return nil, err
	}

	return script, nil
}

// Get retrieves a python script by project and ID
func (s *PythonScriptService) Get(projectID, id string) (*model.PythonScript, error) {
	r := repo.NewPythonScriptRepo(s.pool, s.clock)
	return r.Get(projectID, id)
}

// List retrieves all python scripts for a project
func (s *PythonScriptService) List(projectID string) ([]*model.PythonScript, error) {
	r := repo.NewPythonScriptRepo(s.pool, s.clock)
	scripts, err := r.List(projectID)
	if err != nil {
		return nil, err
	}
	if scripts == nil {
		return []*model.PythonScript{}, nil
	}
	return scripts, nil
}

// Update updates a python script
func (s *PythonScriptService) Update(projectID, id string, req *types.PythonScriptUpdateRequest) error {
	if req.Name != nil && *req.Name == "" {
		return fmt.Errorf("name cannot be empty")
	}
	r := repo.NewPythonScriptRepo(s.pool, s.clock)
	return r.Update(projectID, id, req)
}

// Delete deletes a python script
func (s *PythonScriptService) Delete(projectID, id string) error {
	r := repo.NewPythonScriptRepo(s.pool, s.clock)
	return r.Delete(projectID, id)
}
