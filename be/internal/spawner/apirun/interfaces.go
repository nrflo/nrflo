// Package apirun implements the in-process tool-use loop that drives an
// API-mode agent through one or more turns. The runner is provider-agnostic:
// it consumes a provider.Provider for streaming, and reports messages /
// status back to the spawner via the small interfaces declared here.
//
// The package deliberately does NOT import the spawner package — the spawner
// supplies adapter values that satisfy these interfaces, avoiding the cycle
// (spawner -> apirun -> spawner).
package apirun

// MessageSink receives streaming events from the runner. The sink is bound
// to a single agent process (the spawner adapter captures *processInfo);
// callers do not pass the process again per call.
type MessageSink interface {
	TrackMessage(content, category string)
}

// ProcState is the small mutator surface the runner needs on the agent
// process. The spawner supplies an adapter wrapping *processInfo. The runner
// reads SessionID/ProjectID for AgentSvc and ErrorRecorder calls, and writes
// FinalStatus and ContextLeft so monitorAll observes them through the same
// fields the CLI backend uses.
type ProcState interface {
	SessionID() string
	ProjectID() string
	WorkflowInstanceID() string
	SetFinalStatus(string)
	SetContextLeft(int)
	SetCallbackLevel(int)
}

// AgentSvc persists context_left and broadcasts the corresponding WS event.
// In production this is service.AgentService.UpdateContextLeft.
type AgentSvc interface {
	UpdateContextLeft(sessionID string, pct int) (projectID, ticketID, workflowName string, err error)
}

// ErrorRecorder mirrors spawner.ErrorRecorder so the runner can record
// agent-level errors (auth, network, protocol) without depending on the
// spawner package.
type ErrorRecorder interface {
	RecordError(projectID, errorType, instanceID, message string) error
}
