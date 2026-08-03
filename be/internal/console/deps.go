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
	// StartConsoleWorkflow is StartWorkflow for console-initiated starts: the
	// run's origin is attributed to the launching console session.
	StartConsoleWorkflow(ctx context.Context, projectID, ticketID, workflowName, instructions, scopeType, consoleSessionID string) (string, error)
	StopByProject(projectID, workflowName, instanceID string) error
	RetryFailed(ctx context.Context, projectID, ticketID, workflowName, sessionID string) error
	RetryFailedProject(ctx context.Context, projectID, workflowName, sessionID, instanceID string) error
	// RunPlanner/ResumeAfterPlanApproval back the console-scoped revise_plan/
	// approve_plan handlers (tools_plan.go), which drive service.PlanService
	// directly (project-guarded via loadGuardedInstance) instead of through
	// the parent-ownership-guarded apirun.SubworkflowRunner path — a console
	// session has no parent workflow instance to own a child under. The
	// signature matches service.PlannerRunner, so an Orchestrator value
	// passes directly where that interface is expected.
	RunPlanner(ctx context.Context, instanceID string, in service.PlannerInput) (sessionID string, err error)
	ResumeAfterPlanApproval(ctx context.Context, instanceID string) error
	// ClaimPlanApprovalAtBoundary hands an approval to a live runLoop still
	// drafting inline at the plan boundary, returning true when claimed. The
	// approve_plan handler needs the claim result itself (to choose its
	// response text — a "note" vs. the parked "approved but resume failed"
	// case) rather than just ResumeAfterPlanApproval's error, since a
	// successful claim and a successful resume both return nil there.
	ClaimPlanApprovalAtBoundary(instanceID string) bool
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
	WaitBroker         *WaitBroker
	// Delegator lets a console session's delegate/get_delegation tools spawn
	// tier-resolved workers even though the session has no bound
	// WorkflowInstanceID — the implementation mints a hidden host instance
	// for the call. Nil when not wired (e.g. tests).
	Delegator apirun.Delegator
	// Consultant lets a console session's consult tool ask a named
	// consultant a question even though the session has no bound
	// WorkflowInstanceID — mirrors Delegator's hidden-host path (the
	// implementation resolves the consultant across the project's
	// agent_definitions rather than one caller-known workflow). Nil when not
	// wired (e.g. tests).
	Consultant apirun.ConsultantSpawner
}
