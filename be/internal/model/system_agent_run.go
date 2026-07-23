package model

import (
	"encoding/json"
	"time"
)

// SystemAgentRun is one merged listing item for GET /api/v1/system-agent-runs:
// either a tier/system-agent session or a refinery fold, distinguished by
// Kind. Kind + CreatedAt are the merge keys used to interleave the two
// sources newest-first.
type SystemAgentRun struct {
	Kind                  string          `json:"kind"` // "agent_session" | "refinery_fold"
	SessionID             string          `json:"session_id"`
	AgentType             string          `json:"agent_type,omitempty"`
	Tier                  *int            `json:"tier,omitempty"`
	ResolvedProvider      string          `json:"resolved_provider,omitempty"`
	ResolvedExecutionMode string          `json:"resolved_execution_mode,omitempty"`
	ResolvedEffort        string          `json:"resolved_effort,omitempty"`
	ChainPosition         int             `json:"chain_position,omitempty"`
	FallbackFrom          json.RawMessage `json:"fallback_from,omitempty"`
	ModelID               string          `json:"model_id,omitempty"`
	TokensJSON            json.RawMessage `json:"tokens_json,omitempty"`
	CostEstimate          *float64        `json:"cost_estimate,omitempty"`
	Status                string          `json:"status,omitempty"`
	Result                string          `json:"result,omitempty"`
	WorkflowInstanceID    string          `json:"workflow_instance_id,omitempty"`
	TicketID              string          `json:"ticket_id,omitempty"`
	NodeID                string          `json:"node_id,omitempty"`
	ProjectID             string          `json:"project_id,omitempty"`
	PromptTokens          int             `json:"prompt_tokens,omitempty"`
	OutputTokens          int             `json:"output_tokens,omitempty"`
	Error                 string          `json:"error,omitempty"`
	FoldCount             int             `json:"fold_count,omitempty"`
	CreatedAt             time.Time       `json:"created_at"`
}
