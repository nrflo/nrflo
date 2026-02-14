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

// FindingsAddRequest is the request for adding findings
type FindingsAddRequest struct {
	Workflow  string `json:"workflow"`
	AgentType string `json:"agent_type"`
	Key       string `json:"key"`
	Value     string `json:"value"`
	Model     string `json:"model,omitempty"`
}

// FindingsAddBulkRequest is the request for adding multiple findings at once
type FindingsAddBulkRequest struct {
	Workflow  string            `json:"workflow"`
	AgentType string            `json:"agent_type"`
	KeyValues map[string]string `json:"key_values"` // key -> value (string or JSON)
	Model     string            `json:"model,omitempty"`
}

// FindingsGetRequest is the request for getting findings
type FindingsGetRequest struct {
	Workflow  string   `json:"workflow"`
	AgentType string   `json:"agent_type"`
	Key       string   `json:"key,omitempty"`
	Keys      []string `json:"keys,omitempty"` // Multiple keys to fetch
	Model     string   `json:"model,omitempty"`
}

// FindingsAppendRequest is the request for appending to findings
type FindingsAppendRequest struct {
	Workflow  string `json:"workflow"`
	AgentType string `json:"agent_type"`
	Key       string `json:"key"`
	Value     string `json:"value"`
	Model     string `json:"model,omitempty"`
}

// FindingsAppendBulkRequest is the request for appending multiple values
type FindingsAppendBulkRequest struct {
	Workflow  string            `json:"workflow"`
	AgentType string            `json:"agent_type"`
	KeyValues map[string]string `json:"key_values"`
	Model     string            `json:"model,omitempty"`
}

// FindingsDeleteRequest is the request for deleting finding keys
type FindingsDeleteRequest struct {
	Workflow  string   `json:"workflow"`
	AgentType string   `json:"agent_type"`
	Keys      []string `json:"keys"`
	Model     string   `json:"model,omitempty"`
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

// AgentCompleteRequest is the request for marking an agent as complete/failed
type AgentCompleteRequest struct {
	Workflow  string `json:"workflow"`
	AgentType string `json:"agent_type"`
	Model     string `json:"model,omitempty"`
}

// AgentCallbackRequest is the request for marking an agent as callback
type AgentCallbackRequest struct {
	AgentCompleteRequest
	Level int `json:"level"`
}

// WorkflowDefCreateRequest is the request for creating a workflow definition
type WorkflowDefCreateRequest struct {
	ID          string          `json:"id"`
	Description string          `json:"description,omitempty"`
	ScopeType   string          `json:"scope_type,omitempty"` // "ticket" (default) or "project"
	Phases      json.RawMessage `json:"phases"`               // accepts both string and object entries
}

// WorkflowDefUpdateRequest is the request for updating a workflow definition
type WorkflowDefUpdateRequest struct {
	Description *string          `json:"description,omitempty"`
	ScopeType   *string          `json:"scope_type,omitempty"`
	Phases      *json.RawMessage `json:"phases,omitempty"`
}

// ProjectWorkflowRunRequest is the request for running a project-scoped workflow
type ProjectWorkflowRunRequest struct {
	Workflow     string `json:"workflow"`
	Instructions string `json:"instructions,omitempty"`
}

// AgentDefCreateRequest is the request for creating an agent definition
type AgentDefCreateRequest struct {
	ID               string `json:"id"`
	Model            string `json:"model,omitempty"`
	Timeout          int    `json:"timeout,omitempty"`
	Prompt           string `json:"prompt"`
	RestartThreshold *int   `json:"restart_threshold,omitempty"`
}

// AgentDefUpdateRequest is the request for updating an agent definition
type AgentDefUpdateRequest struct {
	Model            *string `json:"model,omitempty"`
	Timeout          *int    `json:"timeout,omitempty"`
	Prompt           *string `json:"prompt,omitempty"`
	RestartThreshold *int    `json:"restart_threshold,omitempty"`
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
