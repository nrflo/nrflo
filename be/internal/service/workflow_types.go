package service

import (
	"encoding/json"

	"be/internal/types"
)

// WorkflowDef represents a workflow definition (parsed from DB)
type WorkflowDef struct {
	Description             string                `json:"description"`
	ScopeType               string                `json:"scope_type"` // "ticket" or "project"
	CloseTicketOnComplete   bool                  `json:"close_ticket_on_complete"`
	PurgeOnCompletion       bool                  `json:"purge_on_completion"`
	CallableAsSubworkflow   bool                  `json:"callable_as_subworkflow"`
	IsGlobal                bool                  `json:"is_global"`
	Groups                  []string              `json:"groups"`
	NextWorkflowOnSuccess   string                `json:"next_workflow_on_success"`
	FinalizeSuccessCommand  string                `json:"finalize_success_command,omitempty"`
	FinalizeSuccessScriptID string                `json:"finalize_success_script_id,omitempty"`
	FinalizeFailureCommand  string                `json:"finalize_failure_command,omitempty"`
	FinalizeFailureScriptID string                `json:"finalize_failure_script_id,omitempty"`
	PauseEventCommand       string                `json:"pause_event_command,omitempty"`
	PauseEventScriptID      string                `json:"pause_event_script_id,omitempty"`
	Phases                  []PhaseDef            `json:"-"`
	LayerPolicies           map[int]string        `json:"layer_policies,omitempty"`
	ObserverContext         string                `json:"-"`
	ObserverProvider        *string               `json:"-"`
	ObserverModel           *string               `json:"-"`
	FindingSchemas          []types.FindingSchema `json:"finding_schemas"`

	// defProjectID is the project the definition row was found under
	// (GlobalProjectID when resolved via the global fallback). Set by
	// GetWorkflowDef; consumed by buildWorkflowInstance for the workflows FK.
	defProjectID string
}

// MarshalJSON serializes WorkflowDef with parsed phases
func (wf WorkflowDef) MarshalJSON() ([]byte, error) {
	type Alias struct {
		Description             string                `json:"description"`
		ScopeType               string                `json:"scope_type"`
		CloseTicketOnComplete   bool                  `json:"close_ticket_on_complete"`
		PurgeOnCompletion       bool                  `json:"purge_on_completion"`
		CallableAsSubworkflow   bool                  `json:"callable_as_subworkflow"`
		IsGlobal                bool                  `json:"is_global"`
		Groups                  []string              `json:"groups"`
		NextWorkflowOnSuccess   string                `json:"next_workflow_on_success"`
		FinalizeSuccessCommand  string                `json:"finalize_success_command,omitempty"`
		FinalizeSuccessScriptID string                `json:"finalize_success_script_id,omitempty"`
		FinalizeFailureCommand  string                `json:"finalize_failure_command,omitempty"`
		FinalizeFailureScriptID string                `json:"finalize_failure_script_id,omitempty"`
		PauseEventCommand       string                `json:"pause_event_command,omitempty"`
		PauseEventScriptID      string                `json:"pause_event_script_id,omitempty"`
		Phases                  []PhaseDef            `json:"phases"`
		LayerPolicies           map[int]string        `json:"layer_policies,omitempty"`
		ObserverContext         string                `json:"observer_context,omitempty"`
		ObserverProvider        *string               `json:"observer_provider,omitempty"`
		ObserverModel           *string               `json:"observer_model,omitempty"`
		FindingSchemas          []types.FindingSchema `json:"finding_schemas"`
	}
	phases := wf.Phases
	if phases == nil {
		phases = []PhaseDef{}
	}
	findingSchemas := wf.FindingSchemas
	if findingSchemas == nil {
		findingSchemas = []types.FindingSchema{}
	}
	scopeType := wf.ScopeType
	if scopeType == "" {
		scopeType = "ticket"
	}
	groups := wf.Groups
	if groups == nil {
		groups = []string{}
	}
	return json.Marshal(Alias{
		Description:             wf.Description,
		ScopeType:               scopeType,
		CloseTicketOnComplete:   wf.CloseTicketOnComplete,
		PurgeOnCompletion:       wf.PurgeOnCompletion,
		CallableAsSubworkflow:   wf.CallableAsSubworkflow,
		IsGlobal:                wf.IsGlobal,
		Groups:                  groups,
		NextWorkflowOnSuccess:   wf.NextWorkflowOnSuccess,
		FinalizeSuccessCommand:  wf.FinalizeSuccessCommand,
		FinalizeSuccessScriptID: wf.FinalizeSuccessScriptID,
		FinalizeFailureCommand:  wf.FinalizeFailureCommand,
		FinalizeFailureScriptID: wf.FinalizeFailureScriptID,
		PauseEventCommand:       wf.PauseEventCommand,
		PauseEventScriptID:      wf.PauseEventScriptID,
		Phases:                  phases,
		LayerPolicies:           wf.LayerPolicies,
		ObserverContext:         wf.ObserverContext,
		ObserverProvider:        wf.ObserverProvider,
		ObserverModel:           wf.ObserverModel,
		FindingSchemas:          findingSchemas,
	})
}

// UnmarshalJSON deserializes WorkflowDef
func (wf *WorkflowDef) UnmarshalJSON(data []byte) error {
	var raw struct {
		Description             string                `json:"description"`
		ScopeType               string                `json:"scope_type"`
		CloseTicketOnComplete   bool                  `json:"close_ticket_on_complete"`
		PurgeOnCompletion       bool                  `json:"purge_on_completion"`
		CallableAsSubworkflow   bool                  `json:"callable_as_subworkflow"`
		IsGlobal                bool                  `json:"is_global"`
		NextWorkflowOnSuccess   string                `json:"next_workflow_on_success"`
		FinalizeSuccessCommand  string                `json:"finalize_success_command,omitempty"`
		FinalizeSuccessScriptID string                `json:"finalize_success_script_id,omitempty"`
		FinalizeFailureCommand  string                `json:"finalize_failure_command,omitempty"`
		FinalizeFailureScriptID string                `json:"finalize_failure_script_id,omitempty"`
		PauseEventCommand       string                `json:"pause_event_command,omitempty"`
		PauseEventScriptID      string                `json:"pause_event_script_id,omitempty"`
		Phases                  []PhaseDef            `json:"phases"`
		LayerPolicies           map[int]string        `json:"layer_policies,omitempty"`
		ObserverContext         string                `json:"observer_context,omitempty"`
		ObserverProvider        *string               `json:"observer_provider,omitempty"`
		ObserverModel           *string               `json:"observer_model,omitempty"`
		FindingSchemas          []types.FindingSchema `json:"finding_schemas"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	wf.Description = raw.Description
	wf.ScopeType = raw.ScopeType
	wf.CloseTicketOnComplete = raw.CloseTicketOnComplete
	wf.PurgeOnCompletion = raw.PurgeOnCompletion
	wf.CallableAsSubworkflow = raw.CallableAsSubworkflow
	wf.IsGlobal = raw.IsGlobal
	wf.NextWorkflowOnSuccess = raw.NextWorkflowOnSuccess
	wf.FinalizeSuccessCommand = raw.FinalizeSuccessCommand
	wf.FinalizeSuccessScriptID = raw.FinalizeSuccessScriptID
	wf.FinalizeFailureCommand = raw.FinalizeFailureCommand
	wf.FinalizeFailureScriptID = raw.FinalizeFailureScriptID
	wf.PauseEventCommand = raw.PauseEventCommand
	wf.PauseEventScriptID = raw.PauseEventScriptID
	wf.Phases = raw.Phases
	wf.LayerPolicies = raw.LayerPolicies
	wf.ObserverContext = raw.ObserverContext
	wf.ObserverProvider = raw.ObserverProvider
	wf.ObserverModel = raw.ObserverModel
	wf.FindingSchemas = raw.FindingSchemas
	return nil
}

// RestartDetail contains per-restart information for enriched tooltips.
type RestartDetail struct {
	Reason       string  `json:"reason"`
	DurationSec  float64 `json:"duration_sec"`
	ContextLeft  *int64  `json:"context_left,omitempty"`
	MessageCount int     `json:"message_count"`
}

// PhaseDef represents a phase definition. NodeID is the execution identity
// (which slot in the run); Agent is the agent_definitions template key. The
// json tag stays "id" so the workflow-def API payload is unchanged.
type PhaseDef struct {
	NodeID string `json:"id"`
	Agent  string `json:"agent"`
	Layer  int    `json:"layer"`
	Order  int    `json:"order,omitempty"`
}
