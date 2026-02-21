package service

import (
	"database/sql"
	"fmt"
	"os/user"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/types"
)

// WorkflowService handles workflow business logic
type WorkflowService struct {
	clock   clock.Clock
	pool    *db.Pool
	wfiRepo *repo.WorkflowInstanceRepo
}

// NewWorkflowService creates a new workflow service
func NewWorkflowService(pool *db.Pool, clk clock.Clock) *WorkflowService {
	return &WorkflowService{
		clock:   clk,
		pool:    pool,
		wfiRepo: repo.NewWorkflowInstanceRepo(pool, clk),
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
		now := s.clock.Now().UTC().Format(time.RFC3339Nano)
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

// InitProjectWorkflow initializes a project-scoped workflow (no ticket required).
// Returns the created workflow instance.
func (s *WorkflowService) InitProjectWorkflow(projectID string, req *types.ProjectWorkflowRunRequest) (*model.WorkflowInstance, error) {
	if req.Workflow == "" {
		return nil, fmt.Errorf("workflow name is required")
	}

	wf, err := s.GetWorkflowDef(projectID, req.Workflow)
	if err != nil {
		return nil, err
	}

	if wf.ScopeType != "project" {
		return nil, fmt.Errorf("workflow '%s' is not a project-scoped workflow", req.Workflow)
	}

	wi := s.buildWorkflowInstance(projectID, req.Workflow, wf)
	wi.ScopeType = "project"

	if err := s.wfiRepo.Create(wi); err != nil {
		return nil, err
	}
	return wi, nil
}

// buildWorkflowInstance creates a WorkflowInstance from a workflow definition
func (s *WorkflowService) buildWorkflowInstance(projectID, workflowName string, wf *WorkflowDef) *model.WorkflowInstance {
	return &model.WorkflowInstance{
		ID:         uuid.New().String(),
		ProjectID:  projectID,
		WorkflowID: workflowName,
		Status:     model.WorkflowInstanceActive,
		Findings:   "{}",
		RetryCount: 0,
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

	// Load workflow definition for phase derivation
	var phaseOrder []string
	phases := make(map[string]model.PhaseStatus)
	currentPhase := ""
	var phaseLayers map[string]int

	if wf, err := s.GetWorkflowDef(wi.ProjectID, wi.WorkflowID); err == nil {
		phaseOrder = make([]string, len(wf.Phases))
		phaseLayers = make(map[string]int, len(wf.Phases))
		for i, p := range wf.Phases {
			phaseOrder[i] = p.ID
			phaseLayers[p.ID] = p.Layer
		}
		phases = s.derivePhaseStatuses(wi.ID, wf.Phases)
		currentPhase = s.deriveCurrentPhase(wi.ID)
	}

	result := map[string]interface{}{
		"version":        4,
		"initialized_at": wi.CreatedAt.Format(time.RFC3339Nano),
		"instance_id":    wi.ID,
		"scope_type":     scopeType,
		"current_phase":  currentPhase,
		"retry_count":    wi.RetryCount,
		"phases":         phases,
		"phase_order":    phaseOrder,
		"workflow":       wi.WorkflowID,
		"agent_retries":  map[string]int{},
	}
	if phaseLayers != nil {
		result["phase_layers"] = phaseLayers
	}
	if wi.ParentSession.Valid {
		result["parent_session"] = wi.ParentSession.String
	}

	// Completion stats
	result["status"] = string(wi.Status)
	if wi.Status == model.WorkflowInstanceCompleted || wi.Status == model.WorkflowInstanceProjectCompleted {
		result["completed_at"] = wi.UpdatedAt.Format(time.RFC3339Nano)
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

// Set sets a workflow field (restricted to current_phase, retry_count, parent_session)
func (s *WorkflowService) Set(projectID, ticketID string, req *types.WorkflowSetRequest) error {
	if req.Workflow == "" {
		return fmt.Errorf("workflow is required")
	}

	wi, err := s.wfiRepo.GetByTicketAndWorkflow(projectID, ticketID, req.Workflow)
	if err != nil {
		return err
	}

	switch req.Key {
	case "retry_count":
		count, err := strconv.Atoi(req.Value)
		if err != nil {
			return fmt.Errorf("retry_count must be an integer")
		}
		return s.wfiRepo.UpdateRetryCount(wi.ID, count)
	case "parent_session":
		now := s.clock.Now().UTC().Format(time.RFC3339Nano)
		_, err := s.pool.Exec(
			`UPDATE workflow_instances SET parent_session = ?, updated_at = ? WHERE id = ?`,
			sql.NullString{String: req.Value, Valid: req.Value != ""}, now, wi.ID)
		return err
	default:
		return fmt.Errorf("unknown key '%s'. Allowed: retry_count, parent_session", req.Key)
	}
}

// GetWorkflowInstance returns the workflow instance for a ticket+workflow
func (s *WorkflowService) GetWorkflowInstance(projectID, ticketID, workflowName string) (*model.WorkflowInstance, error) {
	return s.wfiRepo.GetByTicketAndWorkflow(projectID, ticketID, workflowName)
}

// GetProjectWorkflowInstance returns the most recent project-scoped workflow instance
// matching the given workflow name. Returns error if none found.
func (s *WorkflowService) GetProjectWorkflowInstance(projectID, workflowName string) (*model.WorkflowInstance, error) {
	instances, err := s.wfiRepo.ListByProjectScope(projectID)
	if err != nil {
		return nil, err
	}
	// Return the most recently created matching instance
	var latest *model.WorkflowInstance
	for _, wi := range instances {
		if wi.WorkflowID == workflowName {
			latest = wi
		}
	}
	if latest == nil {
		return nil, fmt.Errorf("project workflow '%s' not found on %s", workflowName, projectID)
	}
	return latest, nil
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

// AddSkipTag adds a skip tag to a running workflow instance.
// Returns (projectID, ticketID, workflowID, error) for broadcast context.
func (s *WorkflowService) AddSkipTag(instanceID, tag string) (string, string, string, error) {
	wi, err := s.wfiRepo.Get(instanceID)
	if err != nil {
		return "", "", "", err
	}

	// Load workflow def to validate tag against groups
	wf, err := s.GetWorkflowDef(wi.ProjectID, wi.WorkflowID)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to load workflow definition: %w", err)
	}

	found := false
	for _, g := range wf.Groups {
		if g == tag {
			found = true
			break
		}
	}
	if !found {
		return "", "", "", fmt.Errorf("tag '%s' is not in workflow groups %v", tag, wf.Groups)
	}

	// AddSkipTag handles dedup
	wi.AddSkipTag(tag)

	if err := s.wfiRepo.UpdateSkipTags(wi.ID, wi.SkipTags); err != nil {
		return "", "", "", err
	}

	return wi.ProjectID, wi.TicketID, wi.WorkflowID, nil
}
