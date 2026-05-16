package ws

// Protocol v2 constants

const (
	// ProtocolVersion is the current protocol version for v2 events.
	ProtocolVersion = 2

	// Control event types
	EventSnapshotBegin  = "snapshot.begin"
	EventSnapshotChunk  = "snapshot.chunk"
	EventSnapshotEnd    = "snapshot.end"
	EventResyncRequired = "resync.required"
	EventHeartbeat      = "heartbeat"

	// Global event types (sent to all clients regardless of subscription)
	EventGlobalRunningAgents   = "global.running_agents"
	EventProjectEnvVarsUpdated = "project.env_vars_updated"
	EventServiceTokensUpdated  = "service_tokens.updated"

	// Spec import event types
	EventSpecImportStarted = "spec_import.started"
	EventSpecImportReady   = "spec_import.ready"
	EventSpecImportFailed  = "spec_import.failed"

	// SpecImportWorkflowID is the well-known workflow definition ID used to
	// run the spec-normalizer agent. Defined here so packages that should not
	// depend on `api` (e.g. orchestrator) can still recognize the workflow.
	SpecImportWorkflowID = "__spec_import__"
)

// Entity types used in snapshot chunks
const (
	EntityWorkflowState = "workflow_state"
	EntityAgentSessions = "agent_sessions"
	EntityFindings      = "findings"
	EntityTicketDetail  = "ticket_detail"
	EntityChainStatus   = "chain_status"
)
