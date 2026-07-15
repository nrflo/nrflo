package console

import (
	"context"
	"encoding/json"

	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

// workflowRunHandler implements workflow_run: ticket-scoped when ticket_id is
// set (mirrors api/handlers_orchestrate.go's ValidateRunnable guard),
// project-scoped otherwise (mirrors api/handlers_project_workflow.go).
type workflowRunHandler struct{ d Deps }

func (workflowRunHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "workflow_run",
		Description: "Start a workflow run. Returns {\"instance_id\":...}. Set ticket_id for a ticket-scoped workflow — prefer ticket_current (the session's ticket, from the git branch), else pick one with ticket_list — or omit it for a project-scoped one; workflow_list shows each definition's scope_type. A ticket-scoped definition requires ticket_id; a project-scoped one must omit it.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"workflow":{"type":"string"},
"instructions":{"type":"string"},
"ticket_id":{"type":"string","description":"When set, runs ticket-scoped against this ticket; otherwise runs project-scoped"}
},
"required":["workflow"],
"additionalProperties":false
}`),
	}
}

func (h workflowRunHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Workflow     string `json:"workflow"`
		Instructions string `json:"instructions"`
		TicketID     string `json:"ticket_id"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if args.Workflow == "" {
		return "workflow is required", true, nil
	}
	if h.d.Orch == nil {
		return missingService("orchestrator")
	}

	scopeType := "project"
	if args.TicketID != "" {
		scopeType = "ticket"
		if h.d.TicketSvc == nil {
			return missingService("ticket")
		}
		if err := h.d.TicketSvc.ValidateRunnable(env.ProjectID, args.TicketID); err != nil {
			return err.Error(), true, nil
		}
	}

	instanceID, err := h.d.Orch.StartWorkflow(ctx, env.ProjectID, args.TicketID, args.Workflow, args.Instructions, scopeType)
	if err != nil {
		return err.Error(), true, nil
	}
	out, err := json.Marshal(map[string]string{"instance_id": instanceID})
	if err != nil {
		return err.Error(), true, nil
	}
	return string(out), false, nil
}

// workflowStopHandler implements workflow_stop.
type workflowStopHandler struct{ d Deps }

func (workflowStopHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "workflow_stop",
		Description: "Stop a running workflow instance. ticket_id stops via the ticket-scoped path; omit it for a project-scoped instance.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"instance_id":{"type":"string"},
"ticket_id":{"type":"string"},
"workflow":{"type":"string"}
},
"required":["instance_id"],
"additionalProperties":false
}`),
	}
}

func (h workflowStopHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		InstanceID string `json:"instance_id"`
		TicketID   string `json:"ticket_id"`
		Workflow   string `json:"workflow"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if args.InstanceID == "" {
		return "instance_id is required", true, nil
	}
	if h.d.Orch == nil {
		return missingService("orchestrator")
	}
	if _, err := loadGuardedInstance(h.d, env.ProjectID, args.InstanceID); err != nil {
		return err.Error(), true, nil
	}

	var err error
	if args.TicketID != "" {
		err = h.d.Orch.StopByTicket(env.ProjectID, args.TicketID, args.Workflow, args.InstanceID)
	} else {
		err = h.d.Orch.StopByProject(env.ProjectID, args.Workflow, args.InstanceID)
	}
	if err != nil {
		return err.Error(), true, nil
	}
	return "ok", false, nil
}

// workflowRetryFailedHandler implements workflow_retry_failed.
type workflowRetryFailedHandler struct{ d Deps }

func (workflowRetryFailedHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "workflow_retry_failed",
		Description: "Retry a failed workflow from its failed layer. ticket_id retries via the ticket-scoped path; instance_id retries a project-scoped instance.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"workflow":{"type":"string"},
"session_id":{"type":"string"},
"ticket_id":{"type":"string"},
"instance_id":{"type":"string"}
},
"required":["workflow","session_id"],
"additionalProperties":false
}`),
	}
}

func (h workflowRetryFailedHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Workflow   string `json:"workflow"`
		SessionID  string `json:"session_id"`
		TicketID   string `json:"ticket_id"`
		InstanceID string `json:"instance_id"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if args.Workflow == "" || args.SessionID == "" {
		return "workflow and session_id are required", true, nil
	}
	if args.TicketID == "" && args.InstanceID == "" {
		return "either ticket_id or instance_id is required", true, nil
	}
	if h.d.Orch == nil {
		return missingService("orchestrator")
	}
	if args.InstanceID != "" {
		if _, err := loadGuardedInstance(h.d, env.ProjectID, args.InstanceID); err != nil {
			return err.Error(), true, nil
		}
	}

	var err error
	if args.TicketID != "" {
		err = h.d.Orch.RetryFailed(ctx, env.ProjectID, args.TicketID, args.Workflow, args.SessionID)
	} else {
		err = h.d.Orch.RetryFailedProject(ctx, env.ProjectID, args.Workflow, args.SessionID, args.InstanceID)
	}
	if err != nil {
		return err.Error(), true, nil
	}
	return "ok", false, nil
}
