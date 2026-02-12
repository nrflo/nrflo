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
	ScopeType   string            `json:"scope_type"` // "ticket" or "project"
	Categories  []string          `json:"categories"`
	Phases      []PhaseDef        `json:"-"`
	RawPhases   []json.RawMessage `json:"-"` // Internal, used during parsing
}

// MarshalJSON serializes WorkflowDef with parsed phases
func (wf WorkflowDef) MarshalJSON() ([]byte, error) {
	type Alias struct {
		Description string     `json:"description"`
		ScopeType   string     `json:"scope_type"`
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
	scopeType := wf.ScopeType
	if scopeType == "" {
		scopeType = "ticket"
	}
	return json.Marshal(Alias{
		Description: wf.Description,
		ScopeType:   scopeType,
		Categories:  cats,
		Phases:      phases,
	})
}

// UnmarshalJSON deserializes WorkflowDef, parsing mixed-format phases
func (wf *WorkflowDef) UnmarshalJSON(data []byte) error {
	var raw struct {
		Description string            `json:"description"`
		ScopeType   string            `json:"scope_type"`
		Categories  []string          `json:"categories"`
		Phases      []json.RawMessage `json:"phases"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	wf.Description = raw.Description
	wf.ScopeType = raw.ScopeType
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
	ID      string   `json:"id"`
	Agent   string   `json:"agent"`
	Layer   int      `json:"layer"`
	Order   int      `json:"order,omitempty"`
	SkipFor []string `json:"skip_for,omitempty"`
}
