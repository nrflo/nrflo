package service

import (
	"fmt"
	"strings"

	"github.com/google/uuid"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

// WorkflowChainRunService handles workflow chain run business logic.
type WorkflowChainRunService struct {
	pool  *db.Pool
	clock clock.Clock
}

// NewWorkflowChainRunService creates a new WorkflowChainRunService.
func NewWorkflowChainRunService(pool *db.Pool, clk clock.Clock) *WorkflowChainRunService {
	return &WorkflowChainRunService{pool: pool, clock: clk}
}

// WorkflowChainRunDetail holds a run with its steps.
type WorkflowChainRunDetail struct {
	*model.WorkflowChainRun
	Steps []*model.WorkflowChainRunStep `json:"steps"`
}

// CreateRun creates a new WorkflowChainRun and materializes steps from the chain definition.
// Returns the run and materialized steps. The run is in 'pending' status; the caller
// (chainrunner) starts it by calling UpdateRunStatus to 'running'.
func (s *WorkflowChainRunService) CreateRun(projectID, chainID, instructions, triggeredBy string) (*WorkflowChainRunDetail, error) {
	cr := repo.NewWorkflowChainRepo(s.pool, s.clock)
	chain, err := cr.GetChain(projectID, chainID)
	if err != nil {
		return nil, err
	}

	sr := repo.NewWorkflowChainStepRepo(s.pool, s.clock)
	chainSteps, err := sr.ListSteps(chain.ID)
	if err != nil {
		return nil, err
	}
	if len(chainSteps) == 0 {
		return nil, fmt.Errorf("chain %s has no steps", chainID)
	}

	run := &model.WorkflowChainRun{
		ID:                  uuid.New().String(),
		ProjectID:           projectID,
		ChainID:             chain.ID,
		Status:              "pending",
		InitialInstructions: instructions,
		TriggeredBy:         triggeredBy,
		CurrentPosition:     0,
	}

	rr := repo.NewWorkflowChainRunRepo(s.pool, s.clock)
	if err := rr.CreateRun(run); err != nil {
		return nil, err
	}

	steps, err := rr.MaterializeRunSteps(run.ID, chainSteps)
	if err != nil {
		return nil, err
	}

	return &WorkflowChainRunDetail{WorkflowChainRun: run, Steps: steps}, nil
}

// GetRunDetail retrieves a run with its steps, validating project ownership.
func (s *WorkflowChainRunService) GetRunDetail(projectID, runID string) (*WorkflowChainRunDetail, error) {
	rr := repo.NewWorkflowChainRunRepo(s.pool, s.clock)
	run, err := rr.GetRun(runID)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(run.ProjectID, projectID) {
		return nil, fmt.Errorf("workflow chain run not found: %s", runID)
	}
	steps, err := rr.ListRunSteps(runID)
	if err != nil {
		return nil, err
	}
	if steps == nil {
		steps = []*model.WorkflowChainRunStep{}
	}
	return &WorkflowChainRunDetail{WorkflowChainRun: run, Steps: steps}, nil
}

// ListRuns returns runs for a project, optionally filtered to a specific chain.
func (s *WorkflowChainRunService) ListRuns(projectID, chainID string) ([]*model.WorkflowChainRun, error) {
	rr := repo.NewWorkflowChainRunRepo(s.pool, s.clock)
	runs, err := rr.ListRuns(projectID, "")
	if err != nil {
		return nil, err
	}
	if chainID == "" {
		if runs == nil {
			runs = []*model.WorkflowChainRun{}
		}
		return runs, nil
	}
	filtered := runs[:0]
	for _, r := range runs {
		if strings.EqualFold(r.ChainID, chainID) {
			filtered = append(filtered, r)
		}
	}
	if filtered == nil {
		filtered = []*model.WorkflowChainRun{}
	}
	return filtered, nil
}

// SetNextStepInstructions sets the instructions for the next pending step
// in the run associated with the given workflow instance ID.
func (s *WorkflowChainRunService) SetNextStepInstructions(instanceID, instructions string) error {
	rr := repo.NewWorkflowChainRunRepo(s.pool, s.clock)
	return rr.SetNextPendingStepInstructions(instanceID, instructions)
}

// SetNextStepTicket sets the ticket ID for the next pending step
// in the run associated with the given workflow instance ID.
func (s *WorkflowChainRunService) SetNextStepTicket(instanceID, ticketID string) error {
	rr := repo.NewWorkflowChainRunRepo(s.pool, s.clock)
	return rr.SetNextPendingStepTicket(instanceID, ticketID)
}
