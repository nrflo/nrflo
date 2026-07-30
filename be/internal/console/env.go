package console

import "be/internal/spawner/apirun"

// NewToolEnv builds the console apirun.ToolEnv for one tool call: session +
// project identity only. WorkflowInstanceID/TicketID stay empty (a console
// session isn't bound to a run) and Findings stays nil (session-bound
// findings are a non-goal). ArtifactSvc is left nil ON PURPOSE:
// artifacts.workflow_instance_id is NOT NULL REFERENCES workflow_instances(id)
// with foreign_keys(1) on (see be/internal/db/migrations/000109_artifacts.up.sql
// and db/db.go's buildDSN), so a console session can never own an artifact row
// — any console-side AddFromAgent would FK-fail after already writing the
// blob. Leaving it nil makes web_fetch take its documented
// `env.ArtifactSvc == nil` branch (excerpt + "full content unavailable: no
// artifact store"). The console artifact_list/artifact_get tools hold their
// own ArtifactService from Deps and are unaffected.
func NewToolEnv(d Deps, sessionID, projectID string) apirun.ToolEnv {
	return apirun.ToolEnv{
		Pool:            d.Pool,
		WSHub:           d.WSHub,
		Clock:           d.Clock,
		SessionID:       sessionID,
		AgentType:       "console",
		ProjectID:       projectID,
		ProjectFindings: d.ProjectFindingsSvc,
		Ticket:          d.TicketSvc,
		Workflow:        d.WorkflowSvc,
		WorkflowControl: d.WorkflowControl,
		Delegator:       d.Delegator,
	}
}
