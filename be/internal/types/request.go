package types

import "encoding/json"

// TicketCreateRequest is the request for creating a ticket
type TicketCreateRequest struct {
	ID             string `json:"id,omitempty"`
	Title          string `json:"title"`
	Description    string `json:"description,omitempty"`
	Type           string `json:"type,omitempty"`
	Priority       int    `json:"priority,omitempty"`
	ParentTicketID string `json:"parent_ticket_id,omitempty"`
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
	Name          string `json:"name,omitempty"`
	RootPath      string `json:"root_path,omitempty"`
	DefaultBranch string `json:"default_branch,omitempty"`
}

// WorkflowInitRequest is the request for initializing a workflow
type WorkflowInitRequest struct {
	Workflow        string            `json:"workflow,omitempty"`
	ScheduledTaskID string            `json:"scheduled_task_id,omitempty"`
	ExternalID      string            `json:"external_id,omitempty"`
	ExternalContext string            `json:"external_context,omitempty"`
	SeedFindings    map[string]string `json:"seed_findings,omitempty"`

	Origin          string `json:"-"` // launch surface, server-set only (model.RunOriginConsole/RunOriginHuman)
	OriginSessionID string `json:"-"` // launching console session id, server-set only
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
	Layer      *int     `json:"layer,omitempty"`       // layer-keyed read; mutually exclusive with agent_type
	NodeID     string   `json:"node_id,omitempty"`     // node-keyed read; mutually exclusive with agent_type and layer
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

// FindingSchema is a workflow-scoped validation contract for a finding key: Schema is a
// JSON Schema (Draft 2020) the value must satisfy; Example is echoed back on failure.
type FindingSchema struct {
	Key     string          `json:"key"`
	Schema  json.RawMessage `json:"schema"`
	Example json.RawMessage `json:"example"`
}

// FindingsEmitRequest is the request for emitting a schema-validated finding.
// Value is the raw JSON to validate against the key's workflow-scoped schema
// and, on success, store as a session finding.
type FindingsEmitRequest struct {
	Key        string          `json:"key"`
	Value      json.RawMessage `json:"value"`
	SessionID  string          `json:"session_id,omitempty"`
	InstanceID string          `json:"instance_id,omitempty"`
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
	Level       int      `json:"level"`
	Mode        string   `json:"mode,omitempty"`
	TargetAgent string   `json:"target_agent,omitempty"`
	Chain       []string `json:"chain,omitempty"`
}

// WorkflowDefCreateRequest is the request for creating a workflow definition
type WorkflowDefCreateRequest struct {
	ID                      string          `json:"id"`
	Description             string          `json:"description,omitempty"`
	ScopeType               string          `json:"scope_type,omitempty"` // "ticket" (default) or "project"
	Groups                  []string        `json:"groups,omitempty"`
	CloseTicketOnComplete   *bool           `json:"close_ticket_on_complete,omitempty"`
	PurgeOnCompletion       *bool           `json:"purge_on_completion,omitempty"`
	CallableAsSubworkflow   *bool           `json:"callable_as_subworkflow,omitempty"`
	NextWorkflowOnSuccess   string          `json:"next_workflow_on_success,omitempty"`
	FinalizeSuccessCommand  string          `json:"finalize_success_command,omitempty"`
	FinalizeSuccessScriptID string          `json:"finalize_success_script_id,omitempty"`
	FinalizeFailureCommand  string          `json:"finalize_failure_command,omitempty"`
	FinalizeFailureScriptID string          `json:"finalize_failure_script_id,omitempty"`
	PauseEventCommand       string          `json:"pause_event_command,omitempty"`
	PauseEventScriptID      string          `json:"pause_event_script_id,omitempty"`
	ObserverContext         string          `json:"observer_context,omitempty"`
	ObserverProvider        *string         `json:"observer_provider,omitempty"`
	ObserverModel           *string         `json:"observer_model,omitempty"`
	FindingSchemas          []FindingSchema `json:"finding_schemas,omitempty"`
}

// WorkflowDefUpdateRequest is the request for updating a workflow definition
type WorkflowDefUpdateRequest struct {
	Description             *string          `json:"description,omitempty"`
	ScopeType               *string          `json:"scope_type,omitempty"`
	Groups                  *[]string        `json:"groups,omitempty"`
	CloseTicketOnComplete   *bool            `json:"close_ticket_on_complete,omitempty"`
	PurgeOnCompletion       *bool            `json:"purge_on_completion,omitempty"`
	CallableAsSubworkflow   *bool            `json:"callable_as_subworkflow,omitempty"`
	NextWorkflowOnSuccess   *string          `json:"next_workflow_on_success,omitempty"`
	FinalizeSuccessCommand  *string          `json:"finalize_success_command,omitempty"`
	FinalizeSuccessScriptID *string          `json:"finalize_success_script_id,omitempty"`
	FinalizeFailureCommand  *string          `json:"finalize_failure_command,omitempty"`
	FinalizeFailureScriptID *string          `json:"finalize_failure_script_id,omitempty"`
	PauseEventCommand       *string          `json:"pause_event_command,omitempty"`
	PauseEventScriptID      *string          `json:"pause_event_script_id,omitempty"`
	ObserverContext         *string          `json:"observer_context,omitempty"`
	ObserverProvider        *string          `json:"observer_provider,omitempty"`
	ObserverModel           *string          `json:"observer_model,omitempty"`
	FindingSchemas          *[]FindingSchema `json:"finding_schemas,omitempty"`
}

// ContinueWorkflowRequest is the request for continuing a paused workflow instance.
type ContinueWorkflowRequest struct {
	InstanceID   string `json:"instance_id"`
	Instructions string `json:"instructions,omitempty"`
}

// FailWorkflowRequest is the request for failing a workflow instance.
type FailWorkflowRequest struct {
	InstanceID string `json:"instance_id"`
	Reason     string `json:"reason,omitempty"`
}

// ProjectWorkflowRunRequest is the request for running a project-scoped workflow
type ProjectWorkflowRunRequest struct {
	Workflow         string             `json:"workflow"`
	Instructions     string             `json:"instructions,omitempty"`
	Interactive      bool               `json:"interactive,omitempty"`
	PlanMode         bool               `json:"plan_mode,omitempty"`
	EndlessLoop      bool               `json:"endless_loop,omitempty"`
	PlanAutoApprove  bool               `json:"plan_auto_approve,omitempty"`
	ScheduledTaskID  string             `json:"scheduled_task_id,omitempty"`
	ExternalID       string             `json:"external_id,omitempty"`
	ExternalContext  string             `json:"external_context,omitempty"`
	SeedFindings     map[string]string  `json:"seed_findings,omitempty"`
	InputArtifacts   []InputArtifactRef `json:"input_artifacts,omitempty"`
	LaunchDepth      int                `json:"-"` // set by the orchestrator (sub-workflow / next-on-success starts), never by clients
	ParentInstanceID string             `json:"-"` // run_subworkflow caller's instance id; orchestrator-set only
	SubworkflowDepth int                `json:"-"` // run_subworkflow nesting; orchestrator-set only
	Origin           string             `json:"-"` // launch surface, server-set only (model.RunOriginConsole/RunOriginHuman)
	OriginSessionID  string             `json:"-"` // launching console session id, server-set only
}
