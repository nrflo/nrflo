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

	"nrworkflow/internal/db"
	"nrworkflow/internal/model"
	"nrworkflow/internal/repo"
	"nrworkflow/internal/types"
)

// WorkflowState represents the state of a workflow (v4 format)
type WorkflowState struct {
	Version       int                    `json:"version"`
	InitializedAt string                 `json:"initialized_at"`
	CurrentPhase  string                 `json:"current_phase"`
	Category      string                 `json:"category,omitempty"`
	RetryCount    int                    `json:"retry_count"`
	Phases        map[string]PhaseState  `json:"phases"`
	PhaseOrder    []string               `json:"phase_order"`
	ActiveAgents  map[string]interface{} `json:"active_agents"`
	AgentHistory  []interface{}          `json:"agent_history"`
	AgentRetries  map[string]int         `json:"agent_retries"`
	Findings      map[string]interface{} `json:"findings"`
	ParentSession string                 `json:"parent_session,omitempty"`
}

// PhaseState represents the state of a phase
type PhaseState struct {
	Status string `json:"status"`
	Result string `json:"result,omitempty"`
}

// AgentConfig holds agent-specific configuration
type AgentConfig struct {
	Model   string `json:"model"`
	Timeout int    `json:"timeout"`
}

// WorkflowDef represents a workflow definition (parsed from DB)
type WorkflowDef struct {
	Description string            `json:"description"`
	Categories  []string          `json:"categories"`
	Phases      []PhaseDef        `json:"-"`
	RawPhases   []json.RawMessage `json:"-"` // Internal, used during parsing
}

// MarshalJSON serializes WorkflowDef with parsed phases
func (wf WorkflowDef) MarshalJSON() ([]byte, error) {
	type Alias struct {
		Description string     `json:"description"`
		Categories  []string   `json:"categories"`
		Phases      []PhaseDef `json:"phases"`
	}
	cats := wf.Categories
	if cats == nil {
		cats = []string{}
	}
	phases := wf.Phases
	if phases == nil {
		phases = []PhaseDef{}
	}
	return json.Marshal(Alias{
		Description: wf.Description,
		Categories:  cats,
		Phases:      phases,
	})
}

// UnmarshalJSON deserializes WorkflowDef, parsing mixed-format phases
func (wf *WorkflowDef) UnmarshalJSON(data []byte) error {
	var raw struct {
		Description string            `json:"description"`
		Categories  []string          `json:"categories"`
		Phases      []json.RawMessage `json:"phases"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	wf.Description = raw.Description
	wf.Categories = raw.Categories
	wf.RawPhases = raw.Phases

	if len(raw.Phases) > 0 {
		phases, err := parsePhaseDefs(raw.Phases)
		if err != nil {
			return err
		}
		wf.Phases = phases
	}
	return nil
}

// PhaseDef represents a phase definition
type PhaseDef struct {
	ID       string   `json:"id"`
	Agent    string   `json:"agent"`
	Order    int      `json:"order,omitempty"`
	SkipFor  []string `json:"skip_for,omitempty"`
	Parallel *struct {
		Enabled bool     `json:"enabled"`
		Models  []string `json:"models"`
	} `json:"parallel,omitempty"`
}

// SpawnerConfig holds spawner-specific configuration
type SpawnerConfig struct {
	MaxContinuations int `json:"max_continuations,omitempty"`
	ContextThreshold int `json:"context_threshold,omitempty"`
}

// WorkflowConfig represents the full workflow configuration (legacy, includes workflows from config.json)
type WorkflowConfig struct {
	CLI struct {
		Default string `json:"default"`
	} `json:"cli"`
	Agents    map[string]AgentConfig `json:"agents"`
	Workflows map[string]WorkflowDef `json:"workflows"`
	Spawner   SpawnerConfig          `json:"spawner,omitempty"`
}

// ProjectConfig represents the project configuration from config.json (without workflows)
type ProjectConfig struct {
	CLI struct {
		Default string `json:"default"`
	} `json:"cli"`
	Agents  map[string]AgentConfig `json:"agents"`
	Spawner SpawnerConfig          `json:"spawner,omitempty"`
}

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

	// Build phase data
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

	wi := &model.WorkflowInstance{
		ID:           uuid.New().String(),
		ProjectID:    projectID,
		TicketID:     ticketID,
		WorkflowID:   workflowName,
		Status:       model.WorkflowInstanceActive,
		CurrentPhase: sql.NullString{String: firstPhase, Valid: firstPhase != ""},
		PhaseOrder:   string(phaseOrderJSON),
		Phases:       string(phasesJSON),
		Findings:     "{}",
		RetryCount:   0,
	}

	return s.wfiRepo.Create(wi)
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

	// Build v4-compatible response
	result := map[string]interface{}{
		"version":        4,
		"initialized_at": wi.CreatedAt.Format(time.RFC3339),
		"current_phase":  "",
		"category":       "",
		"retry_count":    wi.RetryCount,
		"phases":         wi.GetPhases(),
		"phase_order":    wi.GetPhaseOrder(),
		"workflow":       workflowName,
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

	// Active agents from agent_sessions
	result["active_agents"] = s.buildActiveAgentsMap(wi.ID)

	// Agent history from completed sessions
	result["agent_history"] = s.buildAgentHistory(wi.ID)

	// Combined findings: workflow-level + per-session
	result["findings"] = s.BuildCombinedFindings(wi)

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

func (s *WorkflowService) buildActiveAgentsMap(wfiID string) map[string]interface{} {
	agents := make(map[string]interface{})
	rows, err := s.pool.Query(`
		SELECT id, phase, agent_type, model_id, pid, result, started_at
		FROM agent_sessions
		WHERE workflow_instance_id = ? AND status = 'running'`, wfiID)
	if err != nil {
		return agents
	}
	defer rows.Close()

	for rows.Next() {
		var id, agentType string
		var phase, modelID, agentResult, startedAt sql.NullString
		var pid sql.NullInt64
		rows.Scan(&id, &phase, &agentType, &modelID, &pid, &agentResult, &startedAt)

		key := agentType
		agent := map[string]interface{}{
			"agent_id":   id,
			"agent_type": agentType,
			"session_id": id,
			"result":     nil,
		}
		if phase.Valid {
			agent["phase"] = phase.String
		}
		if modelID.Valid && modelID.String != "" {
			key = agentType + ":" + modelID.String
			agent["model_id"] = modelID.String
			parts := strings.SplitN(modelID.String, ":", 2)
			if len(parts) == 2 {
				agent["cli"] = parts[0]
				agent["model"] = parts[1]
			}
		}
		if pid.Valid {
			agent["pid"] = pid.Int64
		}
		if agentResult.Valid {
			agent["result"] = agentResult.String
		}
		if startedAt.Valid {
			agent["started_at"] = startedAt.String
		}
		agents[key] = agent
	}
	return agents
}

func (s *WorkflowService) buildAgentHistory(wfiID string) []interface{} {
	history := []interface{}{}
	rows, err := s.pool.Query(`
		SELECT id, phase, agent_type, model_id, status, result, result_reason, pid, started_at, ended_at
		FROM agent_sessions
		WHERE workflow_instance_id = ? AND status != 'running'
		ORDER BY created_at`, wfiID)
	if err != nil {
		return history
	}
	defer rows.Close()

	for rows.Next() {
		var id, agentType string
		var phase, modelID, status, agentResult, resultReason, startedAt, endedAt sql.NullString
		var pid sql.NullInt64
		rows.Scan(&id, &phase, &agentType, &modelID, &status, &agentResult, &resultReason, &pid, &startedAt, &endedAt)

		entry := map[string]interface{}{
			"agent_id":   id,
			"agent_type": agentType,
			"session_id": id,
		}
		if phase.Valid {
			entry["phase"] = phase.String
		}
		if modelID.Valid {
			entry["model_id"] = modelID.String
		}
		if status.Valid {
			entry["status"] = status.String
		}
		if agentResult.Valid {
			entry["result"] = agentResult.String
		}
		if resultReason.Valid {
			entry["result_reason"] = resultReason.String
		}
		if startedAt.Valid {
			entry["started_at"] = startedAt.String
		}
		if endedAt.Valid {
			entry["ended_at"] = endedAt.String
		}
		history = append(history, entry)
	}
	return history
}

// BuildCombinedFindings merges workflow-level and per-session findings
func (s *WorkflowService) BuildCombinedFindings(wi *model.WorkflowInstance) map[string]interface{} {
	combined := wi.GetFindings()

	rows, err := s.pool.Query(`
		SELECT agent_type, model_id, findings
		FROM agent_sessions
		WHERE workflow_instance_id = ? AND findings IS NOT NULL AND findings != ''`, wi.ID)
	if err != nil {
		return combined
	}
	defer rows.Close()

	for rows.Next() {
		var agentType string
		var modelID, findingsStr sql.NullString
		rows.Scan(&agentType, &modelID, &findingsStr)

		if !findingsStr.Valid || findingsStr.String == "" {
			continue
		}
		var sessionFindings map[string]interface{}
		if json.Unmarshal([]byte(findingsStr.String), &sessionFindings) != nil {
			continue
		}

		key := agentType
		if modelID.Valid && modelID.String != "" {
			key = agentType + ":" + modelID.String
		}
		combined[key] = sessionFindings
	}
	return combined
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

// ListWorkflowInstances returns all workflow instances for a ticket
func (s *WorkflowService) ListWorkflowInstances(projectID, ticketID string) ([]*model.WorkflowInstance, error) {
	return s.wfiRepo.ListByTicket(projectID, ticketID)
}

// ListWorkflows lists available workflows (loads from DB)
func (s *WorkflowService) ListWorkflows(projectID string) (map[string]WorkflowDef, error) {
	return s.ListWorkflowDefs(projectID)
}

