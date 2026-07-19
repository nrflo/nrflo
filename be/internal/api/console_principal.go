package api

import (
	"net/http"
	"strings"

	"be/internal/console"
	"be/internal/model"
	"be/internal/service"
)

// requireConsoleSession resolves the bearer-authenticated agent session for
// this request and reports whether it is a console session (kind='console'
// or kind='console_chat' — the `agent mcp-external` bridge running inside a
// chat engine adopts a chat session the same way a human console session
// does). getAgentSession is nil both for non-bearer/cookie requests and for a
// closed session's token (GetByToken's status filter already excludes it,
// auth_middleware.go:80-90), so this single check yields the required 401 in
// both cases — mirroring the close route's authorizedForConsoleClose
// precedent (handlers_console.go:80).
func requireConsoleSession(r *http.Request) (*model.AgentSession, bool) {
	sess := getAgentSession(r)
	if sess == nil {
		return nil, false
	}
	if sess.Kind != model.AgentSessionKindConsole && sess.Kind != model.AgentSessionKindConsoleChat {
		return nil, false
	}
	return sess, true
}

// consoleToolProject resolves the project a console tool call acts on: the
// session's own project by default. An X-Project/?project= override is
// honored only for a global-scope console session (session project ==
// service.GlobalProjectID) — a project-scoped session's auth middleware
// already forces X-Project to match sess.ProjectID, so there is nothing to
// override for it.
func consoleToolProject(r *http.Request, sess *model.AgentSession) string {
	if strings.EqualFold(sess.ProjectID, service.GlobalProjectID) {
		if p := getProjectID(r); p != "" {
			return p
		}
	}
	return sess.ProjectID
}

// consoleDeps builds console.Deps from the server's shared infrastructure.
// Stateless and cheap to build per request — same style as
// availableAgentTools building tools_builtin.Builtins() per call
// (handlers_available_tools.go:33). d.Orch/d.WorkflowControl stay nil when
// s.orchestrator is nil (test servers) rather than wrapping a nil
// *orchestrator.Orchestrator in a non-nil interface value.
func (s *Server) consoleDeps() console.Deps {
	d := console.Deps{
		Pool:               s.pool,
		Clock:              s.clock,
		WSHub:              s.wsHub,
		DataPath:           s.dataPath,
		WorkflowSvc:        s.workflowService(),
		TicketSvc:          s.ticketService(),
		ProjectFindingsSvc: service.NewProjectFindingsService(s.pool, s.clock),
		ArtifactSvc:        service.NewArtifactService(s.pool, s.clock, s.wsHub, s.dataPath),
		WaitBroker:         s.waitBroker,
	}
	if s.orchestrator != nil {
		d.Orch = s.orchestrator
		d.WorkflowControl = s.orchestrator.APIWorkflowControl(s.pool)
		d.Delegator = s.orchestrator.APIDelegator(s.pool)
	}
	return d
}
