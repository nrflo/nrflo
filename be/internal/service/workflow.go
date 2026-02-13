package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os/user"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

// WorkflowService handles workflow business logic
type WorkflowService struct {
	pool    *db.Pool
	wfiRepo *repo.WorkflowInstanceRepo
}

// NewWorkflowService creates a new workflow service
func NewWorkflowService(pool *db.Pool) *WorkflowService {
	return &WorkflowService{
		pool:    pool,
		wfiRepo: repo.NewWorkflowInstanceRepo(pool),
	}
}

// --- Workflow Runtime Methods ---

// Init initializes a workflow on a ticket
func (s *WorkflowService) Init(projectID, ticketID string, req *types.WorkflowInitRequest) error {
	workflowName := req.Workflow
	if workflowName == "" {
		workflowName = "feature"
	}

	// Load workflow definition from DB
	wf, err := s.GetWorkflowDef(projectID, workflowName)
	if err != nil {
		return err
	}

	if wf.ScopeType == "project" {
		return fmt.Errorf("workflow '%s' is project-scoped; use project workflow API instead", workflowName)
	}

	// Ensure ticket exists (auto-create if not found)
	var exists int
	err = s.pool.QueryRow("SELECT 1 FROM tickets WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
		projectID, ticketID).Scan(&exists)
	if err == sql.ErrNoRows {
		currentUser, userErr := user.Current()
		if userErr != nil {
			return fmt.Errorf("failed to get current user: %w", userErr)
		}
		now := time.Now().UTC().Format(time.RFC3339)
		_, createErr := s.pool.Exec(`
			INSERT INTO tickets (id, project_id, title, description, status, priority, issue_type, created_at, updated_at, created_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			strings.ToLower(ticketID),
			strings.ToLower(projectID),
			ticketID,
			sql.NullString{},
			"open",
			2,
			"task",
			now, now,
			currentUser.Username,
		)
		if createErr != nil {
			return fmt.Errorf("failed to auto-create ticket: %w", createErr)
		}
	} else if err != nil {
		return fmt.Errorf("failed to query ticket: %w", err)
	}

	wi := s.buildWorkflowInstance(projectID, workflowName, wf)
	wi.TicketID = ticketID
	wi.ScopeType = "ticket"

	return s.wfiRepo.Create(wi)
}

// InitProjectWorkflow initializes a project-scoped workflow (no ticket required)
func (s *WorkflowService) InitProjectWorkflow(projectID string, req *types.ProjectWorkflowRunRequest) error {
	if req.Workflow == "" {
		return fmt.Errorf("workflow name is required")
	}

	wf, err := s.GetWorkflowDef(projectID, req.Workflow)
	if err != nil {
		return err
	}

	if wf.ScopeType != "project" {
		return fmt.Errorf("workflow '%s' is not a project-scoped workflow", req.Workflow)
	}

	wi := s.buildWorkflowInstance(projectID, req.Workflow, wf)
	wi.ScopeType = "project"

	return s.wfiRepo.Create(wi)
}

// buildWorkflowInstance creates a WorkflowInstance from a workflow definition
func (s *WorkflowService) buildWorkflowInstance(projectID, workflowName string, wf *WorkflowDef) *model.WorkflowInstance {
	phaseOrder := make([]string, len(wf.Phases))
	phases := make(map[string]model.PhaseStatus)
	var firstPhase string

	for i, p := range wf.Phases {
		phaseOrder[i] = p.ID
		phases[p.ID] = model.PhaseStatus{Status: "pending"}
		if i == 0 {
			firstPhase = p.ID
		}
	}

	phaseOrderJSON, _ := json.Marshal(phaseOrder)
	phasesJSON, _ := json.Marshal(phases)

	return &model.WorkflowInstance{
		ID:           uuid.New().String(),
		ProjectID:    projectID,
		WorkflowID:   workflowName,
		Status:       model.WorkflowInstanceActive,
		CurrentPhase: sql.NullString{String: firstPhase, Valid: firstPhase != ""},
		PhaseOrder:   string(phaseOrderJSON),
		Phases:       string(phasesJSON),
		Findings:     "{}",
		RetryCount:   0,
	}
}

// GetStatusByInstance builds v4-compatible status from a workflow instance directly
func (s *WorkflowService) GetStatusByInstance(wi *model.WorkflowInstance) (map[string]interface{}, error) {
	result := s.buildV4State(wi)

	return result, nil
}

// GetStatus gets workflow status for a ticket
func (s *WorkflowService) GetStatus(projectID, ticketID string, req *types.WorkflowGetRequest) (map[string]interface{}, error) {
	workflowName := req.Workflow

	// Resolve workflow name if not specified
	if workflowName == "" {
		instances, err := s.wfiRepo.ListByTicket(projectID, ticketID)
		if err != nil {
			return nil, err
		}
		if len(instances) == 0 {
			return nil, fmt.Errorf("ticket %s not initialized", ticketID)
		}
		if len(instances) == 1 {
			workflowName = instances[0].WorkflowID
		} else {
			names := make([]string, len(instances))
			for i, wi := range instances {
				names[i] = wi.WorkflowID
			}
			return nil, fmt.Errorf("multiple workflows on %s: %s. Use workflow parameter to specify.", ticketID, strings.Join(names, ", "))
		}
	}

	wi, err := s.wfiRepo.GetByTicketAndWorkflow(projectID, ticketID, workflowName)
	if err != nil {
		return nil, err
	}

	result := s.buildV4State(wi)

	// Handle field extraction
	if req.Field != "" {
		value, ok := result[req.Field]
		if !ok {
			return nil, fmt.Errorf("field '%s' not found", req.Field)
		}
		return map[string]interface{}{"value": value}, nil
	}

	return result, nil
}

// buildV4State builds the v4-compatible response from a workflow instance
func (s *WorkflowService) buildV4State(wi *model.WorkflowInstance) map[string]interface{} {
	scopeType := wi.ScopeType
	if scopeType == "" {
		scopeType = "ticket"
	}

	result := map[string]interface{}{
		"version":        4,
		"initialized_at": wi.CreatedAt.Format(time.RFC3339),
		"scope_type":     scopeType,
		"current_phase":  "",
		"category":       "",
		"retry_count":    wi.RetryCount,
		"phases":         wi.GetPhases(),
		"phase_order":    wi.GetPhaseOrder(),
		"workflow":       wi.WorkflowID,
		"agent_retries":  map[string]int{},
	}
	if wi.CurrentPhase.Valid {
		result["current_phase"] = wi.CurrentPhase.String
	}
	if wi.Category.Valid {
		result["category"] = wi.Category.String
	}
	if wi.ParentSession.Valid {
		result["parent_session"] = wi.ParentSession.String
	}

	// Completion stats
	result["status"] = string(wi.Status)
	if wi.Status == model.WorkflowInstanceCompleted || wi.Status == model.WorkflowInstanceProjectCompleted {
		result["completed_at"] = wi.UpdatedAt.Format(time.RFC3339)
		result["total_duration_sec"] = wi.UpdatedAt.Sub(wi.CreatedAt).Seconds()
	}

	// Active agents from agent_sessions
	result["active_agents"] = s.buildActiveAgentsMap(wi.ID)

	// Agent history from completed sessions
	agentHistory := s.buildAgentHistory(wi.ID)
	result["agent_history"] = agentHistory

	// Total tokens used (200K context window per agent)
	if wi.Status == model.WorkflowInstanceCompleted || wi.Status == model.WorkflowInstanceProjectCompleted {
		var totalTokens int64
		for _, entry := range agentHistory {
			if m, ok := entry.(map[string]interface{}); ok {
				if cl, exists := m["context_left"]; exists {
					if contextLeft, ok := cl.(int64); ok {
						totalTokens += 200000 * (100 - contextLeft) / 100
					}
				}
			}
		}
		result["total_tokens_used"] = totalTokens
	}

	// Combined findings: workflow-level + per-session
	result["findings"] = s.BuildCombinedFindings(wi)

	return result
}

// Set sets a workflow field (restricted to category, current_phase, retry_count)
func (s *WorkflowService) Set(projectID, ticketID string, req *types.WorkflowSetRequest) error {
	if req.Workflow == "" {
		return fmt.Errorf("workflow is required")
	}

	wi, err := s.wfiRepo.GetByTicketAndWorkflow(projectID, ticketID, req.Workflow)
	if err != nil {
		return err
	}

	switch req.Key {
	case "category":
		return s.wfiRepo.UpdateCategory(wi.ID, req.Value)
	case "current_phase":
		return s.wfiRepo.UpdateCurrentPhase(wi.ID, req.Value)
	case "retry_count":
		count, err := strconv.Atoi(req.Value)
		if err != nil {
			return fmt.Errorf("retry_count must be an integer")
		}
		return s.wfiRepo.UpdateRetryCount(wi.ID, count)
	case "parent_session":
		now := time.Now().UTC().Format(time.RFC3339)
		_, err := s.pool.Exec(
			`UPDATE workflow_instances SET parent_session = ?, updated_at = ? WHERE id = ?`,
			sql.NullString{String: req.Value, Valid: req.Value != ""}, now, wi.ID)
		return err
	default:
		return fmt.Errorf("unknown key '%s'. Allowed: category, current_phase, retry_count, parent_session", req.Key)
	}
}

// StartPhase starts a phase
func (s *WorkflowService) StartPhase(projectID, ticketID string, req *types.PhaseUpdateRequest) error {
	if req.Workflow == "" {
		return fmt.Errorf("workflow is required")
	}
	wi, err := s.wfiRepo.GetByTicketAndWorkflow(projectID, ticketID, req.Workflow)
	if err != nil {
		return err
	}
	return s.wfiRepo.StartPhase(wi.ID, req.Phase)
}

// CompletePhase completes a phase
func (s *WorkflowService) CompletePhase(projectID, ticketID string, req *types.PhaseUpdateRequest) error {
	if req.Result != "pass" && req.Result != "fail" && req.Result != "skipped" {
		return fmt.Errorf("result must be 'pass', 'fail', or 'skipped'")
	}
	if req.Workflow == "" {
		return fmt.Errorf("workflow is required")
	}
	wi, err := s.wfiRepo.GetByTicketAndWorkflow(projectID, ticketID, req.Workflow)
	if err != nil {
		return err
	}
	return s.wfiRepo.CompletePhase(wi.ID, req.Phase, req.Result)
}

// GetWorkflowInstance returns the workflow instance for a ticket+workflow
func (s *WorkflowService) GetWorkflowInstance(projectID, ticketID, workflowName string) (*model.WorkflowInstance, error) {
	return s.wfiRepo.GetByTicketAndWorkflow(projectID, ticketID, workflowName)
}

// GetProjectWorkflowInstance returns the workflow instance for a project-scoped workflow
func (s *WorkflowService) GetProjectWorkflowInstance(projectID, workflowName string) (*model.WorkflowInstance, error) {
	return s.wfiRepo.GetByProjectAndWorkflow(projectID, workflowName)
}

// ListWorkflowInstances returns all workflow instances for a ticket
func (s *WorkflowService) ListWorkflowInstances(projectID, ticketID string) ([]*model.WorkflowInstance, error) {
	return s.wfiRepo.ListByTicket(projectID, ticketID)
}

// ListProjectWorkflowInstances returns all project-scoped workflow instances
func (s *WorkflowService) ListProjectWorkflowInstances(projectID string) ([]*model.WorkflowInstance, error) {
	return s.wfiRepo.ListByProjectScope(projectID)
}

// ListWorkflows lists available workflows (loads from DB)
func (s *WorkflowService) ListWorkflows(projectID string) (map[string]WorkflowDef, error) {
	return s.ListWorkflowDefs(projectID)
}
