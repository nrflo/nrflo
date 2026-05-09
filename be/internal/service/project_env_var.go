package service

import (
	"fmt"
	"regexp"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

var validNameRegex = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var reservedNames = map[string]bool{
	"NRFLO_PROJECT":              true,
	"NRFLO_AGENT_TOKEN":          true,
	"NRFLO_SDK_DIR":              true,
	"NRFLO_HOME":                 true,
	"NRF_SESSION_ID":             true,
	"NRF_WORKFLOW_INSTANCE_ID":   true,
	"NRF_TRX":                    true,
	"NRF_SPAWNED":                true,
	"NRF_CONTEXT_THRESHOLD":      true,
	"NRF_MAX_CONTEXT":            true,
	"CLAUDECODE":                 true,
	"PATH":                       true,
	"HOME":                       true,
}

const maxValueLen = 4096

// ProjectEnvVarService owns validation and delegates persistence to the repo.
type ProjectEnvVarService struct {
	pool  *db.Pool
	clock clock.Clock
}

// NewProjectEnvVarService creates a new project env var service.
func NewProjectEnvVarService(pool *db.Pool, clk clock.Clock) *ProjectEnvVarService {
	return &ProjectEnvVarService{pool: pool, clock: clk}
}

// List returns all env vars for a project.
func (s *ProjectEnvVarService) List(projectID string) ([]*model.ProjectEnvVar, error) {
	r := repo.NewProjectEnvVarRepo(s.pool, s.clock)
	vars, err := r.List(projectID)
	if err != nil {
		return nil, err
	}
	if vars == nil {
		return []*model.ProjectEnvVar{}, nil
	}
	return vars, nil
}

// Upsert validates name/value and inserts or updates the env var.
func (s *ProjectEnvVarService) Upsert(projectID, name, value string) (*model.ProjectEnvVar, error) {
	if !validNameRegex.MatchString(name) {
		return nil, fmt.Errorf("invalid env var name %q: must match ^[A-Za-z_][A-Za-z0-9_]*$", name)
	}
	if reservedNames[name] {
		return nil, fmt.Errorf("env var name %q is reserved", name)
	}
	if len(value) > maxValueLen {
		return nil, fmt.Errorf("value exceeds maximum length of %d bytes", maxValueLen)
	}
	r := repo.NewProjectEnvVarRepo(s.pool, s.clock)
	return r.Upsert(projectID, name, value)
}

// Delete removes an env var by name.
func (s *ProjectEnvVarService) Delete(projectID, name string) error {
	r := repo.NewProjectEnvVarRepo(s.pool, s.clock)
	return r.Delete(projectID, name)
}
