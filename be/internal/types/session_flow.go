package types

// SessionListItem is one row in the Sessions tab listing, served by
// GET /api/v1/sessions (project-scoped) and GET /api/v1/sessions/global.
type SessionListItem struct {
	SessionID          string   `json:"session_id"`
	ProjectID          string   `json:"project_id"`
	Kind               string   `json:"kind"`
	AgentType          string   `json:"agent_type,omitempty"`
	ModelID            string   `json:"model_id,omitempty"`
	Status             string   `json:"status"`
	Result             string   `json:"result,omitempty"`
	WorkflowInstanceID string   `json:"workflow_instance_id,omitempty"`
	Workflow           string   `json:"workflow,omitempty"`
	TicketID           string   `json:"ticket_id,omitempty"`
	StartedAt          string   `json:"started_at,omitempty"`
	EndedAt            string   `json:"ended_at,omitempty"`
	CostEstimate       *float64 `json:"cost_estimate,omitempty"`
}

// SessionListResponse wraps SessionListItem for both the project and global
// listing endpoints.
type SessionListResponse struct {
	Sessions []SessionListItem `json:"sessions"`
}

// SessionFlowNode is one session in the flow graph's transitive closure
// rooted at a session id.
type SessionFlowNode struct {
	SessionID          string `json:"session_id"`
	Kind               string `json:"kind"`
	AgentType          string `json:"agent_type,omitempty"`
	Status             string `json:"status"`
	Result             string `json:"result,omitempty"`
	WorkflowInstanceID string `json:"workflow_instance_id,omitempty"`
	ModelID            string `json:"model_id,omitempty"`
	StartedAt          string `json:"started_at,omitempty"`
	EndedAt            string `json:"ended_at,omitempty"`
	ContextLeft        *int   `json:"context_left,omitempty"`
	// Depth is hops from the root session (0 = root).
	Depth int `json:"depth"`
}

// SessionFlowEdgeKind discriminates why two sessions are connected in the
// flow graph.
const (
	SessionFlowEdgeDelegate    = "delegate"
	SessionFlowEdgeConsult     = "consult"
	SessionFlowEdgeSubworkflow = "subworkflow"
	SessionFlowEdgeSibling     = "sibling"
	SessionFlowEdgeOrigin      = "origin"
)

// SessionFlowEdge is one directed edge in the flow graph (caller -> worker/
// child/sibling/origin).
type SessionFlowEdge struct {
	FromSessionID string `json:"from_session_id"`
	ToSessionID   string `json:"to_session_id"`
	Kind          string `json:"kind"`
}

// SessionFlowResponse is the read-time transitive closure over delegations/
// consults/sub-workflow children/console siblings and origin attribution,
// served by GET /api/v1/sessions/{sid}/flow. Mirrors types.TraceResponse's
// role for the trace subsystem, session-rooted instead of instance-rooted.
type SessionFlowResponse struct {
	RootSessionID string            `json:"root_session_id"`
	Nodes         []SessionFlowNode `json:"nodes"`
	Edges         []SessionFlowEdge `json:"edges"`
	// Truncated is true when BuildSessionFlow hit its depth/node cap before
	// exhausting the graph.
	Truncated bool `json:"truncated"`
}

// ToolCallDistributionEntry is one tool's success/error call counts over a
// flow's session set.
type ToolCallDistributionEntry struct {
	ToolName string `json:"tool_name"`
	Success  int    `json:"success"`
	Error    int    `json:"error"`
}

// SessionStatsResponse is the tool-call distribution plus cost/token rollup
// over a session-flow's node set, served by GET /api/v1/sessions/{sid}/stats.
type SessionStatsResponse struct {
	RootSessionID string                      `json:"root_session_id"`
	ToolCalls     []ToolCallDistributionEntry `json:"tool_calls"`
	// Self* is the root session alone; Subtree* sums every node in the flow
	// exactly once (a shared child counted once, not per caller edge).
	SelfCostUSD    float64 `json:"self_cost_usd"`
	SubtreeCostUSD float64 `json:"subtree_cost_usd"`
	SelfTokens     int64   `json:"self_tokens"`
	SubtreeTokens  int64   `json:"subtree_tokens"`
}
