package types

// TraceResponse is the span tree for one workflow instance, assembled at read
// time from agent_sessions, agent_messages, findings_history, and child
// workflow_instances. Served by GET /api/v1/workflow-instances/{iid}/trace.
type TraceResponse struct {
	InstanceID  string          `json:"instance_id"`
	ProjectID   string          `json:"project_id"`
	TicketID    string          `json:"ticket_id,omitempty"`
	Workflow    string          `json:"workflow"`
	Status      string          `json:"status"`
	StartedAt   string          `json:"started_at"`
	EndedAt     *string         `json:"ended_at,omitempty"`
	Layers      []TraceLayer    `json:"layers"`
	Lanes       []TraceLane     `json:"lanes"`
	Children    []TraceChildRun `json:"children"`
	RootMarkers []TraceMarker   `json:"root_markers,omitempty"`
	Truncated   bool            `json:"truncated"`
}

// TraceLayer is the time band covered by one execution layer.
type TraceLayer struct {
	Layer     int      `json:"layer"`
	Phases    []string `json:"phases"`
	StartedAt *string  `json:"started_at,omitempty"`
	EndedAt   *string  `json:"ended_at,omitempty"`
}

// TraceLane is one agent's timeline row: the relaunch chain rooted at
// lane_id (COALESCE(ancestor_session_id, id)) rendered as ordered segments.
type TraceLane struct {
	LaneID    string         `json:"lane_id"`
	Phase     string         `json:"phase"`
	Layer     int            `json:"layer"` // -1 when the phase is absent from the current workflow def
	AgentType string         `json:"agent_type"`
	ModelID   string         `json:"model_id,omitempty"`
	Status    string         `json:"status"`
	Result    string         `json:"result,omitempty"`
	Segments  []TraceSegment `json:"segments"`
	Restarts  []TraceRestart `json:"restarts,omitempty"`
	Markers   []TraceMarker  `json:"markers"`
}

// TraceSegment is one agent_sessions row within a lane.
type TraceSegment struct {
	SessionID string  `json:"session_id"`
	Status    string  `json:"status"`
	Result    string  `json:"result,omitempty"`
	StartedAt *string `json:"started_at,omitempty"`
	EndedAt   *string `json:"ended_at,omitempty"`
}

// TraceRestart mirrors service.RestartDetail (types must not import service).
type TraceRestart struct {
	Reason       string  `json:"reason"`
	DurationSec  float64 `json:"duration_sec"`
	ContextLeft  *int64  `json:"context_left,omitempty"`
	MessageCount int     `json:"message_count"`
}

// TraceMarker is a point event on a lane (or on the instance root when the
// event cannot be attributed to a session).
type TraceMarker struct {
	Type      string `json:"type"` // tool|subagent|skill|user_input|error|thinking|finding
	At        string `json:"at"`
	SessionID string `json:"session_id,omitempty"`
	Label     string `json:"label"`
}

// TraceChildRun references a sub-workflow launched by this instance.
type TraceChildRun struct {
	InstanceID      string  `json:"instance_id"`
	Workflow        string  `json:"workflow"`
	Status          string  `json:"status"`
	StartedAt       string  `json:"started_at"`
	EndedAt         *string `json:"ended_at,omitempty"`
	ParentSessionID string  `json:"parent_session_id,omitempty"`
}
