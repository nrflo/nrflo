package service

import (
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

// WorkflowChainService handles workflow chain business logic.
type WorkflowChainService struct {
	pool        *db.Pool
	clock       clock.Clock
	workflowSvc *WorkflowService
}

// NewWorkflowChainService creates a new WorkflowChainService.
func NewWorkflowChainService(pool *db.Pool, clk clock.Clock, wfSvc *WorkflowService) *WorkflowChainService {
	return &WorkflowChainService{pool: pool, clock: clk, workflowSvc: wfSvc}
}

// WorkflowChainWithSteps holds a chain and its steps together.
type WorkflowChainWithSteps struct {
	*model.WorkflowChain
	Steps []*model.WorkflowChainStep `json:"steps"`
}

// validate checks that steps are dense (0..N-1), step 0 is project-scope,
// all workflow names resolve, and require_ticket_handoff is only set for ticket-scope steps.
func (s *WorkflowChainService) validate(projectID string, steps []*model.WorkflowChainStep) error {
	if len(steps) == 0 {
		return nil
	}

	sorted := make([]*model.WorkflowChainStep, len(steps))
	copy(sorted, steps)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Position < sorted[j].Position })

	for i, step := range sorted {
		if step.Position != i {
			return fmt.Errorf("step positions must be dense (0..%d), got position %d at index %d", len(steps)-1, step.Position, i)
		}
	}

	if sorted[0].ScopeType != "project" {
		return fmt.Errorf("step 0 must have scope_type=project, got %q", sorted[0].ScopeType)
	}

	for _, step := range sorted {
		if step.ScopeType != "project" && step.ScopeType != "ticket" {
			return fmt.Errorf("invalid scope_type %q: must be project or ticket", step.ScopeType)
		}
		if _, err := s.workflowSvc.GetWorkflowDef(projectID, step.WorkflowName); err != nil {
			if strings.Contains(err.Error(), "not found") {
				return fmt.Errorf("workflow not found: %s", step.WorkflowName)
			}
			return err
		}
		if step.RequireTicketHandoff && step.ScopeType != "ticket" {
			return fmt.Errorf("require_ticket_handoff is only valid for ticket-scope steps")
		}
	}
	return nil
}

// ListChains returns all workflow chains for a project.
func (s *WorkflowChainService) ListChains(projectID string) ([]*model.WorkflowChain, error) {
	r := repo.NewWorkflowChainRepo(s.pool, s.clock)
	chains, err := r.ListChains(projectID)
	if chains == nil {
		chains = []*model.WorkflowChain{}
	}
	return chains, err
}

// GetChain retrieves a chain with its steps.
func (s *WorkflowChainService) GetChain(projectID, id string) (*WorkflowChainWithSteps, error) {
	cr := repo.NewWorkflowChainRepo(s.pool, s.clock)
	chain, err := cr.GetChain(projectID, id)
	if err != nil {
		return nil, err
	}
	sr := repo.NewWorkflowChainStepRepo(s.pool, s.clock)
	steps, err := sr.ListSteps(chain.ID)
	if err != nil {
		return nil, err
	}
	if steps == nil {
		steps = []*model.WorkflowChainStep{}
	}
	return &WorkflowChainWithSteps{WorkflowChain: chain, Steps: steps}, nil
}

// CreateChain validates and persists a new workflow chain with its steps.
func (s *WorkflowChainService) CreateChain(projectID string, req *types.WorkflowChainCreateRequest) (*WorkflowChainWithSteps, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	id := req.ID
	if id == "" {
		id = uuid.New().String()
	}

	steps := make([]*model.WorkflowChainStep, len(req.Steps))
	for i, sr := range req.Steps {
		stepID := sr.ID
		if stepID == "" {
			stepID = uuid.New().String()
		}
		steps[i] = &model.WorkflowChainStep{
			ID:                   stepID,
			ProjectID:            projectID,
			ChainID:              id,
			Position:             i,
			WorkflowName:         sr.WorkflowName,
			ScopeType:            sr.ScopeType,
			BaseInstructions:     sr.BaseInstructions,
			RequireTicketHandoff: sr.RequireTicketHandoff,
		}
	}

	if err := s.validate(projectID, steps); err != nil {
		return nil, err
	}

	chain := &model.WorkflowChain{
		ID:          id,
		ProjectID:   projectID,
		Name:        req.Name,
		Description: req.Description,
	}

	cr := repo.NewWorkflowChainRepo(s.pool, s.clock)
	if err := cr.CreateChain(chain); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return nil, fmt.Errorf("workflow chain already exists: %s", id)
		}
		return nil, err
	}

	sr := repo.NewWorkflowChainStepRepo(s.pool, s.clock)
	for _, step := range steps {
		if err := sr.UpsertStep(step); err != nil {
			return nil, err
		}
	}

	return &WorkflowChainWithSteps{WorkflowChain: chain, Steps: steps}, nil
}

// UpdateChain applies partial updates to a workflow chain's metadata.
func (s *WorkflowChainService) UpdateChain(projectID, id string, req *types.WorkflowChainUpdateRequest) (*WorkflowChainWithSteps, error) {
	cr := repo.NewWorkflowChainRepo(s.pool, s.clock)
	chain, err := cr.GetChain(projectID, id)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		chain.Name = *req.Name
	}
	if req.Description != nil {
		chain.Description = *req.Description
	}

	if err := cr.UpdateChain(chain); err != nil {
		return nil, err
	}

	sr := repo.NewWorkflowChainStepRepo(s.pool, s.clock)
	steps, err := sr.ListSteps(chain.ID)
	if err != nil {
		return nil, err
	}
	if steps == nil {
		steps = []*model.WorkflowChainStep{}
	}
	return &WorkflowChainWithSteps{WorkflowChain: chain, Steps: steps}, nil
}

// DeleteChain removes a workflow chain (steps cascade via FK).
func (s *WorkflowChainService) DeleteChain(projectID, id string) error {
	r := repo.NewWorkflowChainRepo(s.pool, s.clock)
	return r.DeleteChain(projectID, id)
}

// AppendStep adds a new step at the end of a chain.
func (s *WorkflowChainService) AppendStep(projectID, chainID string, req *types.WorkflowChainStepRequest) (*WorkflowChainWithSteps, error) {
	cr := repo.NewWorkflowChainRepo(s.pool, s.clock)
	chain, err := cr.GetChain(projectID, chainID)
	if err != nil {
		return nil, err
	}

	sr := repo.NewWorkflowChainStepRepo(s.pool, s.clock)
	existing, err := sr.ListSteps(chain.ID)
	if err != nil {
		return nil, err
	}

	stepID := req.ID
	if stepID == "" {
		stepID = uuid.New().String()
	}
	newStep := &model.WorkflowChainStep{
		ID:                   stepID,
		ProjectID:            projectID,
		ChainID:              chain.ID,
		Position:             len(existing),
		WorkflowName:         req.WorkflowName,
		ScopeType:            req.ScopeType,
		BaseInstructions:     req.BaseInstructions,
		RequireTicketHandoff: req.RequireTicketHandoff,
	}

	allSteps := append(existing, newStep)
	if err := s.validate(projectID, allSteps); err != nil {
		return nil, err
	}

	if err := sr.UpsertStep(newStep); err != nil {
		return nil, err
	}

	return &WorkflowChainWithSteps{WorkflowChain: chain, Steps: allSteps}, nil
}

// UpdateStep applies partial updates to a chain step.
func (s *WorkflowChainService) UpdateStep(projectID, chainID, stepID string, req *types.WorkflowChainStepUpdateRequest) (*WorkflowChainWithSteps, error) {
	cr := repo.NewWorkflowChainRepo(s.pool, s.clock)
	chain, err := cr.GetChain(projectID, chainID)
	if err != nil {
		return nil, err
	}

	sr := repo.NewWorkflowChainStepRepo(s.pool, s.clock)
	steps, err := sr.ListSteps(chain.ID)
	if err != nil {
		return nil, err
	}

	var target *model.WorkflowChainStep
	for _, st := range steps {
		if strings.EqualFold(st.ID, stepID) {
			target = st
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("workflow chain step not found: %s", stepID)
	}

	if req.WorkflowName != nil {
		target.WorkflowName = *req.WorkflowName
	}
	if req.ScopeType != nil {
		target.ScopeType = *req.ScopeType
	}
	if req.BaseInstructions != nil {
		target.BaseInstructions = *req.BaseInstructions
	}
	if req.RequireTicketHandoff != nil {
		target.RequireTicketHandoff = *req.RequireTicketHandoff
	}

	if err := s.validate(projectID, steps); err != nil {
		return nil, err
	}

	if err := sr.UpsertStep(target); err != nil {
		return nil, err
	}

	return &WorkflowChainWithSteps{WorkflowChain: chain, Steps: steps}, nil
}

// DeleteStep removes a step and re-densifies remaining positions.
func (s *WorkflowChainService) DeleteStep(projectID, chainID, stepID string) (*WorkflowChainWithSteps, error) {
	cr := repo.NewWorkflowChainRepo(s.pool, s.clock)
	chain, err := cr.GetChain(projectID, chainID)
	if err != nil {
		return nil, err
	}

	sr := repo.NewWorkflowChainStepRepo(s.pool, s.clock)
	if err := sr.DeleteStep(stepID); err != nil {
		return nil, err
	}

	remaining, err := sr.ListSteps(chain.ID)
	if err != nil {
		return nil, err
	}
	if remaining == nil {
		remaining = []*model.WorkflowChainStep{}
	}

	// Re-densify positions
	if len(remaining) > 0 {
		ids := make([]string, len(remaining))
		for i, st := range remaining {
			ids[i] = st.ID
		}
		if err := sr.BulkReorder(chain.ID, ids); err != nil {
			return nil, err
		}
		// Reload after reorder
		remaining, err = sr.ListSteps(chain.ID)
		if err != nil {
			return nil, err
		}
		if err := s.validate(projectID, remaining); err != nil {
			return nil, err
		}
	}

	return &WorkflowChainWithSteps{WorkflowChain: chain, Steps: remaining}, nil
}

// ReorderSteps reassigns step positions according to the provided ordered IDs.
func (s *WorkflowChainService) ReorderSteps(projectID, chainID string, req *types.ReorderStepsRequest) (*WorkflowChainWithSteps, error) {
	cr := repo.NewWorkflowChainRepo(s.pool, s.clock)
	chain, err := cr.GetChain(projectID, chainID)
	if err != nil {
		return nil, err
	}

	sr := repo.NewWorkflowChainStepRepo(s.pool, s.clock)
	existing, err := sr.ListSteps(chain.ID)
	if err != nil {
		return nil, err
	}

	if len(req.OrderedStepIDs) != len(existing) {
		return nil, fmt.Errorf("ordered_step_ids must contain exactly %d step IDs", len(existing))
	}

	existingSet := make(map[string]bool, len(existing))
	for _, st := range existing {
		existingSet[strings.ToLower(st.ID)] = true
	}
	for _, id := range req.OrderedStepIDs {
		if !existingSet[strings.ToLower(id)] {
			return nil, fmt.Errorf("step not found in chain: %s", id)
		}
	}

	if err := sr.BulkReorder(chain.ID, req.OrderedStepIDs); err != nil {
		return nil, err
	}

	steps, err := sr.ListSteps(chain.ID)
	if err != nil {
		return nil, err
	}
	if steps == nil {
		steps = []*model.WorkflowChainStep{}
	}

	if err := s.validate(projectID, steps); err != nil {
		return nil, err
	}

	return &WorkflowChainWithSteps{WorkflowChain: chain, Steps: steps}, nil
}
