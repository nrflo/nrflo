// Package console builds the server-owned tool profile and dispatcher for
// console agent_sessions (kind='console'): session-independent, no
// WorkflowInstanceID, driven over HTTP by GET/POST /api/v1/console/tools. It
// also owns kind='console_chat' session lifecycle (ChatService, chat_*.go):
// a server-managed spawner.ConsoleEngine per session, reached by the same
// tool profile through the `agent mcp-external` bridge.
package console

import (
	"context"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/service"
	"be/internal/spawner/apirun"
)

// Orchestrator is the narrow slice of *orchestrator.Orchestrator the console
// tool handlers need. Defined as an interface (not the concrete type) to keep
// this package free of an orchestrator import and to make tests fakeable —
// the same pattern as apirun.WorkflowController. All methods are already
// implemented by *orchestrator.Orchestrator (orchestrator_observer_adapter.go,
// orchestrator_stoppage.go).
type Orchestrator interface {
	StartWorkflow(ctx context.Context, projectID, ticketID, workflowName, instructions, scopeType string) (string, error)
	StartWorkflowWithContext(ctx context.Context, projectID, ticketID, workflowName, instructions, externalContext, scopeType string) (string, error)
	StopByTicket(projectID, ticketID, workflowName, instanceID string) error
	StopByProject(projectID, workflowName, instanceID string) error
	RetryFailed(ctx context.Context, projectID, ticketID, workflowName, sessionID string) error
	RetryFailedProject(ctx context.Context, projectID, workflowName, sessionID, instanceID string) error
}

// Deps bundles the injected dependencies every console tool handler needs.
// Console handlers are structs holding Deps (unlike the empty-struct
// tools_builtin handlers), so nothing console-only pollutes apirun.ToolEnv.
type Deps struct {
	Pool               *db.Pool
	Clock              clock.Clock
	WSHub              service.WSHub
	DataPath           string
	Orch               Orchestrator
	WorkflowSvc        *service.WorkflowService
	TicketSvc          *service.TicketService
	ProjectFindingsSvc *service.ProjectFindingsService
	ArtifactSvc        *service.ArtifactService
	WorkflowControl    apirun.WorkflowController
}
