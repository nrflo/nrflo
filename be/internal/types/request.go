package types

import "encoding/json"

// TicketCreateRequest is the request for creating a ticket
type TicketCreateRequest struct {
	ID          string `json:"id,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type,omitempty"`
	Priority    int    `json:"priority,omitempty"`
}

// TicketUpdateRequest is the request for updating a ticket
type TicketUpdateRequest struct {
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	Status      *string `json:"status,omitempty"`
	Priority    *int    `json:"priority,omitempty"`
	Type        *string `json:"type,omitempty"`
}

// TicketListRequest is the request for listing tickets
type TicketListRequest struct {
	Status string `json:"status,omitempty"`
	Type   string `json:"type,omitempty"`
}

// TicketCloseRequest is the request for closing a ticket
type TicketCloseRequest struct {
	Reason string `json:"reason,omitempty"`
}

// TicketSearchRequest is the request for searching tickets
type TicketSearchRequest struct {
	Query string `json:"query"`
}

// ProjectCreateRequest is the request for creating a project
type ProjectCreateRequest struct {
	Name            string `json:"name,omitempty"`
	RootPath        string `json:"root_path,omitempty"`
	DefaultWorkflow string `json:"default_workflow,omitempty"`
	DefaultBranch   string `json:"default_branch,omitempty"`
}

// WorkflowInitRequest is the request for initializing a workflow
type WorkflowInitRequest struct {
	Workflow string `json:"workflow,omitempty"`
}

// WorkflowGetRequest is the request for getting workflow state
type WorkflowGetRequest struct {
	Workflow string `json:"workflow,omitempty"`
	Field    string `json:"field,omitempty"`
}

// WorkflowSetRequest is the request for setting a workflow field
type WorkflowSetRequest struct {
	Workflow string `json:"workflow"`
	Key      string `json:"key"`
	Value    string `json:"value"`
}

// PhaseUpdateRequest is the request for starting/completing a phase
type PhaseUpdateRequest struct {
	Workflow string `json:"workflow"`
	Phase    string `json:"phase"`
	Result   string `json:"result,omitempty"` // for complete: pass, fail, skipped
}

// FindingsAddRequest is the request for adding a single finding
type FindingsAddRequest struct {
	Key        string `json:"key"`
	Value      string `json:"value"`
	SessionID  string `json:"session_id,omitempty"`
	InstanceID string `json:"instance_id,omitempty"`
}

// FindingsAddBulkRequest is the request for adding multiple findings at once
type FindingsAddBulkRequest struct {
	KeyValues  map[string]string `json:"key_values"` // key -> value (string or JSON)
	SessionID  string            `json:"session_id,omitempty"`
	InstanceID string            `json:"instance_id,omitempty"`
}

// FindingsGetRequest is the request for getting findings
type FindingsGetRequest struct {
	AgentType  string   `json:"agent_type,omitempty"` // omit = own session, provide = cross-agent read
	Key        string   `json:"key,omitempty"`
	Keys       []string `json:"keys,omitempty"` // Multiple keys to fetch
	Model      string   `json:"model,omitempty"`
	InstanceID string   `json:"instance_id,omitempty"` // required for cross-agent reads
	SessionID  string   `json:"session_id,omitempty"`  // required for own-session reads
}

// FindingsAppendRequest is the request for appending to findings
type FindingsAppendRequest struct {
	Key        string `json:"key"`
	Value      string `json:"value"`
	SessionID  string `json:"session_id,omitempty"`
	InstanceID string `json:"instance_id,omitempty"`
}

// FindingsAppendBulkRequest is the request for appending multiple values
type FindingsAppendBulkRequest struct {
	KeyValues  map[string]string `json:"key_values"`
	SessionID  string            `json:"session_id,omitempty"`
	InstanceID string            `json:"instance_id,omitempty"`
}

// FindingsDeleteRequest is the request for deleting finding keys
type FindingsDeleteRequest struct {
	Keys       []string `json:"keys"`
	SessionID  string   `json:"session_id,omitempty"`
	InstanceID string   `json:"instance_id,omitempty"`
}

// ProjectFindingsAddRequest is the request for adding a project finding
type ProjectFindingsAddRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// ProjectFindingsAddBulkRequest is the request for adding multiple project findings
type ProjectFindingsAddBulkRequest struct {
	KeyValues map[string]string `json:"key_values"`
}

// ProjectFindingsGetRequest is the request for getting project findings
type ProjectFindingsGetRequest struct {
	Key  string   `json:"key,omitempty"`
	Keys []string `json:"keys,omitempty"`
}

// ProjectFindingsAppendRequest is the request for appending to a project finding
type ProjectFindingsAppendRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// ProjectFindingsAppendBulkRequest is the request for appending multiple project findings
type ProjectFindingsAppendBulkRequest struct {
	KeyValues map[string]string `json:"key_values"`
}

// ProjectFindingsDeleteRequest is the request for deleting project finding keys
type ProjectFindingsDeleteRequest struct {
	Keys []string `json:"keys"`
}

// AgentSpawnRequest is the request for spawning an agent
type AgentSpawnRequest struct {
	AgentType     string `json:"agent_type"`
	Workflow      string `json:"workflow"`
	ParentSession string `json:"parent_session"`
	CLI           string `json:"cli,omitempty"`
}

// AgentPreviewRequest is the request for previewing an agent prompt
type AgentPreviewRequest struct {
	AgentType string `json:"agent_type"`
	Workflow  string `json:"workflow,omitempty"`
}

// AgentActiveRequest is the request for listing active agents
type AgentActiveRequest struct {
	Workflow string `json:"workflow"`
}

// AgentKillRequest is the request for killing agents
type AgentKillRequest struct {
	Workflow string `json:"workflow"`
	Model    string `json:"model,omitempty"`
}

// AgentRequest is the shared request for agent lifecycle commands (fail/continue/callback).
// All context (project, ticket, workflow, agent_type) is derived from the session on the server side.
type AgentRequest struct {
	Reason     string `json:"reason,omitempty"`
	InstanceID string `json:"instance_id,omitempty"`
	SessionID  string `json:"session_id,omitempty"`
}

// AgentCallbackRequest is the request for marking an agent as callback
type AgentCallbackRequest struct {
	AgentRequest
	Level int `json:"level"`
}

// WorkflowDefCreateRequest is the request for creating a workflow definition
type WorkflowDefCreateRequest struct {
	ID          string          `json:"id"`
	Description string          `json:"description,omitempty"`
	ScopeType   string          `json:"scope_type,omitempty"` // "ticket" (default) or "project"
	Phases      json.RawMessage `json:"phases"`               // accepts both string and object entries
	Groups      []string        `json:"groups,omitempty"`
}

// WorkflowDefUpdateRequest is the request for updating a workflow definition
type WorkflowDefUpdateRequest struct {
	Description *string          `json:"description,omitempty"`
	ScopeType   *string          `json:"scope_type,omitempty"`
	Phases      *json.RawMessage `json:"phases,omitempty"`
	Groups      *[]string        `json:"groups,omitempty"`
}

// ProjectWorkflowRunRequest is the request for running a project-scoped workflow
type ProjectWorkflowRunRequest struct {
	Workflow     string `json:"workflow"`
	Instructions string `json:"instructions,omitempty"`
	Interactive  bool   `json:"interactive,omitempty"`
	PlanMode     bool   `json:"plan_mode,omitempty"`
}

// AgentDefCreateRequest is the request for creating an agent definition
type AgentDefCreateRequest struct {
	ID               string `json:"id"`
	Model            string `json:"model,omitempty"`
	Timeout          int    `json:"timeout,omitempty"`
	Prompt           string `json:"prompt"`
	RestartThreshold *int   `json:"restart_threshold,omitempty"`
	MaxFailRestarts        *int   `json:"max_fail_restarts,omitempty"`
	StallStartTimeoutSec   *int   `json:"stall_start_timeout_sec,omitempty"`
	StallRunningTimeoutSec *int   `json:"stall_running_timeout_sec,omitempty"`
	Tag                    string `json:"tag,omitempty"`
	LowConsumptionModel    string `json:"low_consumption_model,omitempty"`
}

// AgentDefUpdateRequest is the request for updating an agent definition
type AgentDefUpdateRequest struct {
	Model                  *string `json:"model,omitempty"`
	Timeout                *int    `json:"timeout,omitempty"`
	Prompt                 *string `json:"prompt,omitempty"`
	RestartThreshold       *int    `json:"restart_threshold,omitempty"`
	MaxFailRestarts        *int    `json:"max_fail_restarts,omitempty"`
	StallStartTimeoutSec   *int    `json:"stall_start_timeout_sec,omitempty"`
	StallRunningTimeoutSec *int    `json:"stall_running_timeout_sec,omitempty"`
	Tag                    *string `json:"tag,omitempty"`
	LowConsumptionModel    *string `json:"low_consumption_model,omitempty"`
}

// SystemAgentDefCreateRequest is the request for creating a system agent definition
type SystemAgentDefCreateRequest struct {
	ID                     string `json:"id"`
	Model                  string `json:"model,omitempty"`
	Timeout                int    `json:"timeout,omitempty"`
	Prompt                 string `json:"prompt"`
	RestartThreshold       *int   `json:"restart_threshold,omitempty"`
	MaxFailRestarts        *int   `json:"max_fail_restarts,omitempty"`
	StallStartTimeoutSec   *int   `json:"stall_start_timeout_sec,omitempty"`
	StallRunningTimeoutSec *int   `json:"stall_running_timeout_sec,omitempty"`
}

// SystemAgentDefUpdateRequest is the request for updating a system agent definition
type SystemAgentDefUpdateRequest struct {
	Model                  *string `json:"model,omitempty"`
	Timeout                *int    `json:"timeout,omitempty"`
	Prompt                 *string `json:"prompt,omitempty"`
	RestartThreshold       *int    `json:"restart_threshold,omitempty"`
	MaxFailRestarts        *int    `json:"max_fail_restarts,omitempty"`
	StallStartTimeoutSec   *int    `json:"stall_start_timeout_sec,omitempty"`
	StallRunningTimeoutSec *int    `json:"stall_running_timeout_sec,omitempty"`
}

// DependencyRequest is the request for adding/removing dependencies
type DependencyRequest struct {
	Child  string `json:"child"`
	Parent string `json:"parent"`
}

// StatusRequest is the request for ticket status summary
type StatusRequest struct {
	PendingLimit   int `json:"pending_limit,omitempty"`
	CompletedLimit int `json:"completed_limit,omitempty"`
}
