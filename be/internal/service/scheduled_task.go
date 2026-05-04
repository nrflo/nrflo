package service

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

// ScheduleReloader is implemented by scheduler.Scheduler.
// Defined here so the service package does not import scheduler (would cycle).
type ScheduleReloader interface {
	Reload() error
}

// ScheduledTaskService handles scheduled task business logic.
type ScheduledTaskService struct {
	pool         *db.Pool
	clock        clock.Clock
	reloader     ScheduleReloader
	wfChainSvc   *WorkflowChainService
}

// NewScheduledTaskService creates a new ScheduledTaskService.
func NewScheduledTaskService(pool *db.Pool, clk clock.Clock, reloader ScheduleReloader, wfChainSvc *WorkflowChainService) *ScheduledTaskService {
	return &ScheduledTaskService{pool: pool, clock: clk, reloader: reloader, wfChainSvc: wfChainSvc}
}

// List returns all scheduled tasks for a project.
func (s *ScheduledTaskService) List(projectID string) ([]*model.ScheduledTask, error) {
	r := repo.NewScheduledTaskRepo(s.pool, s.clock)
	tasks, err := r.List(projectID)
	if tasks == nil {
		tasks = []*model.ScheduledTask{}
	}
	return tasks, err
}

// Get retrieves a single scheduled task by ID.
func (s *ScheduledTaskService) Get(id string) (*model.ScheduledTask, error) {
	r := repo.NewScheduledTaskRepo(s.pool, s.clock)
	return r.Get(id)
}

// Create validates and persists a new scheduled task.
func (s *ScheduledTaskService) Create(projectID string, req *types.ScheduledTaskCreateRequest) (*model.ScheduledTask, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if req.CronExpression == "" {
		return nil, fmt.Errorf("cron_expression is required")
	}
	if _, err := cron.ParseStandard(req.CronExpression); err != nil {
		return nil, fmt.Errorf("invalid cron expression: %s", err.Error())
	}
	if len(req.Workflows) == 0 && len(req.WorkflowChainIDs) == 0 {
		return nil, fmt.Errorf("workflows_required")
	}
	if err := s.validateWorkflows(projectID, req.Workflows); err != nil {
		return nil, err
	}
	if err := s.validateChainIDs(projectID, req.WorkflowChainIDs); err != nil {
		return nil, err
	}

	id := req.ID
	if id == "" {
		id = uuid.New().String()
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	chainIDs := req.WorkflowChainIDs
	if chainIDs == nil {
		chainIDs = []string{}
	}

	task := &model.ScheduledTask{
		ID:               id,
		ProjectID:        projectID,
		Name:             req.Name,
		Description:      req.Description,
		CronExpression:   req.CronExpression,
		Workflows:        req.Workflows,
		WorkflowChainIDs: chainIDs,
		Enabled:          enabled,
	}

	r := repo.NewScheduledTaskRepo(s.pool, s.clock)
	if err := r.Create(task); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return nil, fmt.Errorf("scheduled task already exists: %s", id)
		}
		return nil, err
	}

	if s.reloader != nil {
		_ = s.reloader.Reload()
	}
	return task, nil
}

// Update applies partial updates to a scheduled task.
func (s *ScheduledTaskService) Update(id string, req *types.ScheduledTaskUpdateRequest) (*model.ScheduledTask, error) {
	r := repo.NewScheduledTaskRepo(s.pool, s.clock)
	task, err := r.Get(id)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		task.Name = *req.Name
	}
	if req.Description != nil {
		task.Description = *req.Description
	}
	if req.CronExpression != nil {
		if _, parseErr := cron.ParseStandard(*req.CronExpression); parseErr != nil {
			return nil, fmt.Errorf("invalid cron expression: %s", parseErr.Error())
		}
		task.CronExpression = *req.CronExpression
	}
	if req.Workflows != nil {
		if err := s.validateWorkflows(task.ProjectID, *req.Workflows); err != nil {
			return nil, err
		}
		task.Workflows = *req.Workflows
	}
	if req.WorkflowChainIDs != nil {
		if err := s.validateChainIDs(task.ProjectID, *req.WorkflowChainIDs); err != nil {
			return nil, err
		}
		task.WorkflowChainIDs = *req.WorkflowChainIDs
	}
	// After applying both fields, ensure at least one is non-empty
	if len(task.Workflows) == 0 && len(task.WorkflowChainIDs) == 0 {
		return nil, fmt.Errorf("workflows_required")
	}
	if req.Enabled != nil {
		task.Enabled = *req.Enabled
	}

	if err := r.Update(task); err != nil {
		return nil, err
	}

	if s.reloader != nil {
		_ = s.reloader.Reload()
	}
	return task, nil
}

// Delete removes a scheduled task and its runs.
func (s *ScheduledTaskService) Delete(id string) error {
	r := repo.NewScheduledTaskRepo(s.pool, s.clock)
	if err := r.Delete(id); err != nil {
		return err
	}

	if s.reloader != nil {
		_ = s.reloader.Reload()
	}
	return nil
}

// ListRuns returns paginated schedule run history for a task.
func (s *ScheduledTaskService) ListRuns(taskID string, limit, offset int) ([]*model.ScheduleRun, error) {
	if limit <= 0 {
		limit = 50
	}
	r := repo.NewScheduleRunRepo(s.pool, s.clock)
	return r.ListByTask(taskID, limit, offset)
}

// validateWorkflows ensures every workflow ID exists for the project and has scope_type=project.
func (s *ScheduledTaskService) validateWorkflows(projectID string, workflows []string) error {
	wfRepo := repo.NewWorkflowRepo(s.pool, s.clock)
	for _, wfID := range workflows {
		wf, err := wfRepo.Get(projectID, wfID)
		if err != nil {
			return fmt.Errorf("invalid_workflow: %s", wfID)
		}
		if wf.ScopeType != "project" {
			return fmt.Errorf("not_project_scope: workflow %s has scope_type=%s", wfID, wf.ScopeType)
		}
	}
	return nil
}

// validateChainIDs ensures every chain ID exists for the project.
func (s *ScheduledTaskService) validateChainIDs(projectID string, chainIDs []string) error {
	if s.wfChainSvc == nil {
		return nil
	}
	for _, chainID := range chainIDs {
		if _, err := s.wfChainSvc.GetChain(projectID, chainID); err != nil {
			return fmt.Errorf("invalid_chain: %s", chainID)
		}
	}
	return nil
}
