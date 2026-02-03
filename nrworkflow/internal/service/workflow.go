package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"nrworkflow/internal/db"
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
	Model    string `json:"model"`
	MaxTurns int    `json:"max_turns"`
	Timeout  int    `json:"timeout"`
}

// WorkflowDef represents a workflow definition
type WorkflowDef struct {
	Description string     `json:"description"`
	Categories  []string   `json:"categories"`
	Phases      []PhaseDef `json:"phases"`
}

// PhaseDef represents a phase definition
type PhaseDef struct {
	ID       string   `json:"id"`
	Agent    string   `json:"agent"`
	SkipFor  []string `json:"skip_for,omitempty"`
	Parallel *struct {
		Enabled bool     `json:"enabled"`
		Models  []string `json:"models"`
	} `json:"parallel,omitempty"`
}

// WorkflowConfig represents the workflow configuration
type WorkflowConfig struct {
	CLI struct {
		Default string `json:"default"`
	} `json:"cli"`
	Agents    map[string]AgentConfig `json:"agents"`
	Workflows map[string]WorkflowDef `json:"workflows"`
}

// WorkflowService handles workflow business logic
type WorkflowService struct {
	pool *db.Pool
}

// NewWorkflowService creates a new workflow service
func NewWorkflowService(pool *db.Pool) *WorkflowService {
	return &WorkflowService{pool: pool}
}

// Init initializes a workflow on a ticket
func (s *WorkflowService) Init(projectID, ticketID string, req *types.WorkflowInitRequest, projectRoot string) error {
	workflowName := req.Workflow
	if workflowName == "" {
		workflowName = "feature"
	}

	// Load config
	config, err := LoadMergedWorkflowConfig(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	wf, ok := config.Workflows[workflowName]
	if !ok {
		names := []string{}
		for n := range config.Workflows {
			names = append(names, n)
		}
		return fmt.Errorf("unknown workflow '%s'. Available: %s", workflowName, strings.Join(names, ", "))
	}

	// Get ticket or auto-create if not found
	var agentsStateStr string
	err = s.pool.QueryRow("SELECT COALESCE(agents_state, '') FROM tickets WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
		projectID, ticketID).Scan(&agentsStateStr)
	if err == sql.ErrNoRows {
		// Auto-create ticket
		currentUser, userErr := user.Current()
		if userErr != nil {
			return fmt.Errorf("failed to get current user: %w", userErr)
		}
		now := time.Now().UTC().Format(time.RFC3339)
		_, createErr := s.pool.Exec(`
			INSERT INTO tickets (id, project_id, title, description, status, priority, issue_type, created_at, updated_at, created_by, agents_state)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			strings.ToLower(ticketID),
			strings.ToLower(projectID),
			ticketID, // Use ticket ID as title
			sql.NullString{},
			"open",
			2, // default priority
			"task",
			now,
			now,
			currentUser.Username,
			sql.NullString{},
		)
		if createErr != nil {
			return fmt.Errorf("failed to auto-create ticket: %w", createErr)
		}
		agentsStateStr = ""
	} else if err != nil {
		return fmt.Errorf("failed to query ticket: %w", err)
	}

	// Parse existing state
	var allState map[string]interface{}
	if agentsStateStr != "" {
		if err := json.Unmarshal([]byte(agentsStateStr), &allState); err != nil {
			allState = make(map[string]interface{})
		}
	} else {
		allState = make(map[string]interface{})
	}

	// Check if workflow already initialized
	if _, exists := allState[workflowName]; exists {
		return fmt.Errorf("workflow '%s' already initialized on %s", workflowName, ticketID)
	}

	// Build default state
	phases := make(map[string]PhaseState)
	findings := make(map[string]interface{})
	var firstPhase string

	for i, p := range wf.Phases {
		phases[p.ID] = PhaseState{Status: "pending"}
		findings[p.Agent] = make(map[string]interface{})
		if i == 0 {
			firstPhase = p.ID
		}
	}

	state := WorkflowState{
		Version:       4,
		InitializedAt: time.Now().UTC().Format(time.RFC3339),
		CurrentPhase:  firstPhase,
		RetryCount:    0,
		Phases:        phases,
		ActiveAgents:  make(map[string]interface{}),
		AgentHistory:  []interface{}{},
		AgentRetries:  make(map[string]int),
		Findings:      findings,
	}

	allState[workflowName] = state

	// Save state
	stateJSON, err := json.Marshal(allState)
	if err != nil {
		return fmt.Errorf("failed to serialize state: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.pool.Exec(
		"UPDATE tickets SET agents_state = ?, updated_at = ? WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
		string(stateJSON), now, projectID, ticketID)
	if err != nil {
		return fmt.Errorf("failed to update ticket: %w", err)
	}

	return nil
}

// GetStatus gets workflow status for a ticket
func (s *WorkflowService) GetStatus(projectID, ticketID string, req *types.WorkflowGetRequest, projectRoot string) (map[string]interface{}, error) {
	// Get ticket
	var agentsStateStr string
	err := s.pool.QueryRow("SELECT COALESCE(agents_state, '') FROM tickets WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
		projectID, ticketID).Scan(&agentsStateStr)
	if err != nil {
		return nil, fmt.Errorf("ticket not found: %s", ticketID)
	}

	if agentsStateStr == "" {
		return nil, fmt.Errorf("ticket %s not initialized", ticketID)
	}

	var allState map[string]interface{}
	if err := json.Unmarshal([]byte(agentsStateStr), &allState); err != nil {
		return nil, fmt.Errorf("failed to parse state: %w", err)
	}

	// Resolve workflow
	workflowName := req.Workflow
	if workflowName == "" {
		if len(allState) == 1 {
			for k := range allState {
				workflowName = k
			}
		} else if len(allState) > 1 {
			names := []string{}
			for k := range allState {
				names = append(names, k)
			}
			return nil, fmt.Errorf("multiple workflows on %s: %s. Use workflow parameter to specify.", ticketID, strings.Join(names, ", "))
		} else {
			return nil, fmt.Errorf("ticket %s not initialized", ticketID)
		}
	}

	stateRaw, ok := allState[workflowName]
	if !ok {
		return nil, fmt.Errorf("workflow '%s' not found on %s", workflowName, ticketID)
	}

	state, ok := stateRaw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid state format")
	}

	// Get specific field if requested
	if req.Field != "" {
		value, ok := state[req.Field]
		if !ok {
			return nil, fmt.Errorf("field '%s' not found", req.Field)
		}
		return map[string]interface{}{"value": value}, nil
	}

	state["workflow"] = workflowName
	return state, nil
}

// Set sets a workflow field
func (s *WorkflowService) Set(projectID, ticketID string, req *types.WorkflowSetRequest) error {
	if req.Workflow == "" {
		return fmt.Errorf("workflow is required")
	}

	// Get ticket
	var agentsStateStr string
	err := s.pool.QueryRow("SELECT COALESCE(agents_state, '') FROM tickets WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
		projectID, ticketID).Scan(&agentsStateStr)
	if err != nil {
		return fmt.Errorf("ticket not found: %s", ticketID)
	}

	if agentsStateStr == "" {
		return fmt.Errorf("ticket %s not initialized", ticketID)
	}

	var allState map[string]interface{}
	if err := json.Unmarshal([]byte(agentsStateStr), &allState); err != nil {
		return fmt.Errorf("failed to parse state: %w", err)
	}

	stateRaw, ok := allState[req.Workflow]
	if !ok {
		return fmt.Errorf("workflow '%s' not found", req.Workflow)
	}

	state, ok := stateRaw.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid state format")
	}

	// Try to parse value as JSON
	var parsed interface{}
	if err := json.Unmarshal([]byte(req.Value), &parsed); err != nil {
		parsed = req.Value // Use as string
	}

	state[req.Key] = parsed
	allState[req.Workflow] = state

	stateJSON, _ := json.Marshal(allState)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.pool.Exec(
		"UPDATE tickets SET agents_state = ?, updated_at = ? WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
		string(stateJSON), now, projectID, ticketID)
	if err != nil {
		return fmt.Errorf("failed to update: %w", err)
	}

	return nil
}

// StartPhase starts a phase
func (s *WorkflowService) StartPhase(projectID, ticketID string, req *types.PhaseUpdateRequest) error {
	return s.updatePhaseState(projectID, ticketID, req.Workflow, req.Phase, "in_progress", "")
}

// CompletePhase completes a phase
func (s *WorkflowService) CompletePhase(projectID, ticketID string, req *types.PhaseUpdateRequest) error {
	if req.Result != "pass" && req.Result != "fail" && req.Result != "skipped" {
		return fmt.Errorf("result must be 'pass', 'fail', or 'skipped'")
	}
	return s.updatePhaseState(projectID, ticketID, req.Workflow, req.Phase, "completed", req.Result)
}

func (s *WorkflowService) updatePhaseState(projectID, ticketID, workflowName, phase, status, result string) error {
	if workflowName == "" {
		return fmt.Errorf("workflow is required")
	}

	// Get ticket
	var agentsStateStr string
	err := s.pool.QueryRow("SELECT COALESCE(agents_state, '') FROM tickets WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
		projectID, ticketID).Scan(&agentsStateStr)
	if err != nil {
		return fmt.Errorf("ticket not found: %s", ticketID)
	}

	if agentsStateStr == "" {
		return fmt.Errorf("ticket %s not initialized", ticketID)
	}

	var allState map[string]interface{}
	if err := json.Unmarshal([]byte(agentsStateStr), &allState); err != nil {
		return fmt.Errorf("failed to parse state: %w", err)
	}

	stateRaw, ok := allState[workflowName]
	if !ok {
		return fmt.Errorf("workflow '%s' not found", workflowName)
	}

	state, ok := stateRaw.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid state format")
	}

	phases, ok := state["phases"].(map[string]interface{})
	if !ok {
		phases = make(map[string]interface{})
		state["phases"] = phases
	}

	phaseState, ok := phases[phase].(map[string]interface{})
	if !ok {
		phaseState = make(map[string]interface{})
	}

	phaseState["status"] = status
	if result != "" {
		phaseState["result"] = result
	}
	phases[phase] = phaseState

	if status == "in_progress" {
		state["current_phase"] = phase
	}

	allState[workflowName] = state

	stateJSON, _ := json.Marshal(allState)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.pool.Exec(
		"UPDATE tickets SET agents_state = ?, updated_at = ? WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)",
		string(stateJSON), now, projectID, ticketID)
	if err != nil {
		return fmt.Errorf("failed to update: %w", err)
	}

	return nil
}

// ListWorkflows lists available workflows
func (s *WorkflowService) ListWorkflows(projectRoot string) (map[string]WorkflowDef, error) {
	config, err := LoadMergedWorkflowConfig(projectRoot)
	if err != nil {
		return nil, err
	}
	return config.Workflows, nil
}

// LoadWorkflowConfig loads the global workflow config
func LoadWorkflowConfig() (*WorkflowConfig, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	configPath := filepath.Join(home, ".nrworkflow", "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultWorkflowConfig(), nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var config WorkflowConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &config, nil
}

// LoadMergedWorkflowConfig loads global config and merges with project config
func LoadMergedWorkflowConfig(projectRoot string) (*WorkflowConfig, error) {
	globalConfig, err := LoadWorkflowConfig()
	if err != nil {
		return nil, err
	}

	if projectRoot == "" || projectRoot == "." {
		return globalConfig, nil
	}

	projectConfigPath := filepath.Join(projectRoot, ".claude", "nrworkflow", "config.json")
	data, err := os.ReadFile(projectConfigPath)
	if err != nil {
		return globalConfig, nil
	}

	var projectConfig WorkflowConfig
	if err := json.Unmarshal(data, &projectConfig); err != nil {
		return globalConfig, nil
	}

	return mergeConfigs(globalConfig, &projectConfig), nil
}

func mergeConfigs(global, project *WorkflowConfig) *WorkflowConfig {
	result := &WorkflowConfig{
		CLI:       global.CLI,
		Agents:    make(map[string]AgentConfig),
		Workflows: make(map[string]WorkflowDef),
	}

	for name, agent := range global.Agents {
		result.Agents[name] = agent
	}

	for name, projectAgent := range project.Agents {
		if existingAgent, ok := result.Agents[name]; ok {
			if projectAgent.Model != "" {
				existingAgent.Model = projectAgent.Model
			}
			if projectAgent.MaxTurns > 0 {
				existingAgent.MaxTurns = projectAgent.MaxTurns
			}
			if projectAgent.Timeout > 0 {
				existingAgent.Timeout = projectAgent.Timeout
			}
			result.Agents[name] = existingAgent
		} else {
			result.Agents[name] = projectAgent
		}
	}

	for name, workflow := range global.Workflows {
		result.Workflows[name] = workflow
	}

	for name, workflow := range project.Workflows {
		result.Workflows[name] = workflow
	}

	if project.CLI.Default != "" {
		result.CLI.Default = project.CLI.Default
	}

	return result
}

func defaultWorkflowConfig() *WorkflowConfig {
	return &WorkflowConfig{
		Workflows: map[string]WorkflowDef{
			"feature": {
				Description: "Full TDD feature development workflow",
				Phases: []PhaseDef{
					{ID: "investigation", Agent: "setup-analyzer"},
					{ID: "test-design", Agent: "test-writer", SkipFor: []string{"docs", "simple"}},
					{ID: "implementation", Agent: "implementor"},
					{ID: "verification", Agent: "qa-verifier", SkipFor: []string{"docs"}},
					{ID: "docs", Agent: "doc-updater"},
				},
			},
			"bugfix": {
				Description: "Bug fix workflow",
				Phases: []PhaseDef{
					{ID: "investigation", Agent: "setup-analyzer"},
					{ID: "implementation", Agent: "implementor"},
					{ID: "verification", Agent: "qa-verifier"},
				},
			},
			"hotfix": {
				Description: "Emergency hotfix - implementation only",
				Phases: []PhaseDef{
					{ID: "implementation", Agent: "implementor"},
				},
			},
		},
	}
}
