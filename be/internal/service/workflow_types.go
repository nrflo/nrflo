package service

import (
	"encoding/json"
)

// WorkflowState represents the state of a workflow (v4 format)
type WorkflowState struct {
	Version       int                    `json:"version"`
	InitializedAt string                 `json:"initialized_at"`
	ScopeType     string                 `json:"scope_type"`
	CurrentPhase  string                 `json:"current_phase"`
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
	Description           string            `json:"description"`
	ScopeType             string            `json:"scope_type"` // "ticket" or "project"
	CloseTicketOnComplete bool              `json:"close_ticket_on_complete"`
	Groups                []string          `json:"groups"`
	Phases                []PhaseDef        `json:"-"`
	RawPhases             []json.RawMessage `json:"-"` // Internal, used during parsing
}

// MarshalJSON serializes WorkflowDef with parsed phases
func (wf WorkflowDef) MarshalJSON() ([]byte, error) {
	type Alias struct {
		Description           string     `json:"description"`
		ScopeType             string     `json:"scope_type"`
		CloseTicketOnComplete bool       `json:"close_ticket_on_complete"`
		Groups                []string   `json:"groups"`
		Phases                []PhaseDef `json:"phases"`
	}
	phases := wf.Phases
	if phases == nil {
		phases = []PhaseDef{}
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
		Description:           wf.Description,
		ScopeType:             scopeType,
		CloseTicketOnComplete: wf.CloseTicketOnComplete,
		Groups:                groups,
		Phases:                phases,
	})
}

// UnmarshalJSON deserializes WorkflowDef, parsing mixed-format phases
func (wf *WorkflowDef) UnmarshalJSON(data []byte) error {
	var raw struct {
		Description           string            `json:"description"`
		ScopeType             string            `json:"scope_type"`
		CloseTicketOnComplete bool              `json:"close_ticket_on_complete"`
		Phases                []json.RawMessage `json:"phases"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	wf.Description = raw.Description
	wf.ScopeType = raw.ScopeType
	wf.CloseTicketOnComplete = raw.CloseTicketOnComplete
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

// RestartDetail contains per-restart information for enriched tooltips.
type RestartDetail struct {
	Reason       string  `json:"reason"`
	DurationSec  float64 `json:"duration_sec"`
	ContextLeft  *int64  `json:"context_left,omitempty"`
	MessageCount int     `json:"message_count"`
}

// PhaseDef represents a phase definition
type PhaseDef struct {
	ID    string `json:"id"`
	Agent string `json:"agent"`
	Layer int    `json:"layer"`
	Order int    `json:"order,omitempty"`
}
