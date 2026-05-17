package service

import (
	"encoding/json"
	"fmt"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
	"be/internal/types"
)

// ProjectFindingsService handles project-level key-value findings stored in the findings table.
type ProjectFindingsService struct {
	clock       clock.Clock
	pool        *db.Pool
	findingRepo *repo.FindingRepo
}

// NewProjectFindingsService creates a new project findings service
func NewProjectFindingsService(pool *db.Pool, clk clock.Clock) *ProjectFindingsService {
	return &ProjectFindingsService{
		pool:        pool,
		clock:       clk,
		findingRepo: repo.NewFindingRepo(pool, clk),
	}
}

// Add upserts a single key-value finding for a project.
// The optional actor overrides the default {Source: "agent"} used by socket/apirun callers.
func (s *ProjectFindingsService) Add(projectID string, req *types.ProjectFindingsAddRequest, actors ...repo.Actor) error {
	if req.Key == "" {
		return fmt.Errorf("key is required")
	}
	val := json.RawMessage(normalizeJSONValue(req.Value))
	denorm := repo.Denorm{ProjectID: projectID}
	actor := defaultProjectActor(actors)
	return s.findingRepo.Upsert("project", projectID, req.Key, val, denorm, actor)
}

// AddBulk upserts multiple key-value findings.
func (s *ProjectFindingsService) AddBulk(projectID string, req *types.ProjectFindingsAddBulkRequest, actors ...repo.Actor) error {
	if len(req.KeyValues) == 0 {
		return fmt.Errorf("at least one key-value pair is required")
	}
	denorm := repo.Denorm{ProjectID: projectID}
	actor := defaultProjectActor(actors)
	for key, val := range req.KeyValues {
		v := json.RawMessage(normalizeJSONValue(val))
		if err := s.findingRepo.Upsert("project", projectID, key, v, denorm, actor); err != nil {
			return err
		}
	}
	return nil
}

// Get retrieves project findings. If no keys specified, returns all as map[string]interface{}.
func (s *ProjectFindingsService) Get(projectID string, req *types.ProjectFindingsGetRequest) (interface{}, error) {
	keys := req.Keys
	if req.Key != "" && len(keys) == 0 {
		keys = []string{req.Key}
	}
	raw, err := s.findingRepo.GetOwn("project", projectID)
	if err != nil {
		return nil, err
	}
	all := rawToInterface(raw)
	if len(keys) == 0 {
		return all, nil
	}
	if len(keys) == 1 {
		v, ok := all[keys[0]]
		if !ok {
			return nil, fmt.Errorf("finding '%s' not found", keys[0])
		}
		return v, nil
	}
	result := make(map[string]interface{})
	for _, k := range keys {
		if v, ok := all[k]; ok {
			result[k] = v
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("none of the requested keys found")
	}
	return result, nil
}

// Append appends to an existing value using the same array-merge logic as findings.
func (s *ProjectFindingsService) Append(projectID string, req *types.ProjectFindingsAppendRequest, actors ...repo.Actor) error {
	if req.Key == "" {
		return fmt.Errorf("key is required")
	}
	val := json.RawMessage(normalizeJSONValue(req.Value))
	denorm := repo.Denorm{ProjectID: projectID}
	actor := defaultProjectActor(actors)
	return s.findingRepo.Append("project", projectID, req.Key, val, denorm, actor)
}

// AppendBulk appends multiple values in sequence.
func (s *ProjectFindingsService) AppendBulk(projectID string, req *types.ProjectFindingsAppendBulkRequest, actors ...repo.Actor) error {
	if len(req.KeyValues) == 0 {
		return fmt.Errorf("at least one key-value pair is required")
	}
	denorm := repo.Denorm{ProjectID: projectID}
	actor := defaultProjectActor(actors)
	for key, val := range req.KeyValues {
		v := json.RawMessage(normalizeJSONValue(val))
		if err := s.findingRepo.Append("project", projectID, key, v, denorm, actor); err != nil {
			return err
		}
	}
	return nil
}

// Delete removes specified keys and returns the list of actually deleted keys.
func (s *ProjectFindingsService) Delete(projectID string, req *types.ProjectFindingsDeleteRequest, actors ...repo.Actor) ([]string, error) {
	if len(req.Keys) == 0 {
		return nil, fmt.Errorf("at least one key is required")
	}
	actor := defaultProjectActor(actors)
	return s.findingRepo.DeleteKeys("project", projectID, req.Keys, actor)
}

// defaultProjectActor returns the first actor or {Source:"agent"} as default.
func defaultProjectActor(actors []repo.Actor) repo.Actor {
	if len(actors) > 0 {
		return actors[0]
	}
	return repo.Actor{Source: "agent"}
}
