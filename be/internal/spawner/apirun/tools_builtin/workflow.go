package tools_builtin

import (
	"context"
	"encoding/json"

	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
	"be/internal/ws"
)

type workflowContinueHandler struct{}

func (workflowContinueHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "workflow_continue",
		Description: "Resume a paused (waiting) workflow instance from its pause point.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"instance_id":{"type":"string"},
"instructions":{"type":"string"}
},
"required":["instance_id"],
"additionalProperties":false
}`),
	}
}

func (workflowContinueHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		InstanceID   string `json:"instance_id"`
		Instructions string `json:"instructions"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if env.WorkflowControl == nil {
		return missingService("workflow_control")
	}
	if err := env.WorkflowControl.ContinueWorkflow(ctx, env.ProjectID, args.InstanceID, args.Instructions); err != nil {
		return err.Error(), true, nil
	}
	return "ok", false, nil
}

type workflowFailHandler struct{}

func (workflowFailHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "workflow_fail",
		Description: "Fail a workflow instance with a given reason.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"instance_id":{"type":"string"},
"reason":{"type":"string"}
},
"required":["instance_id","reason"],
"additionalProperties":false
}`),
	}
}

func (workflowFailHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		InstanceID string `json:"instance_id"`
		Reason     string `json:"reason"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if env.WorkflowControl == nil {
		return missingService("workflow_control")
	}
	if err := env.WorkflowControl.FailWorkflow(ctx, env.ProjectID, args.InstanceID, args.Reason); err != nil {
		return err.Error(), true, nil
	}
	return "ok", false, nil
}

type workflowSkipHandler struct{}

func (workflowSkipHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "workflow_skip",
		Description: "Add a skip tag to the current workflow instance (must be one of the workflow's groups).",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"tag":{"type":"string"}
},
"required":["tag"],
"additionalProperties":false
}`),
	}
}

func (workflowSkipHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Tag string `json:"tag"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if env.Workflow == nil {
		return missingService("workflow")
	}
	projectID, ticketID, workflow, err := env.Workflow.AddSkipTag(env.WorkflowInstanceID, args.Tag)
	if err != nil {
		return err.Error(), true, nil
	}
	service.BroadcastFromCtx(env.WSHub, ws.EventSkipTagAdded, service.BroadcastCtx{
		ProjectID: projectID,
		TicketID:  ticketID,
		Workflow:  workflow,
	}, map[string]interface{}{
		"instance_id": env.WorkflowInstanceID,
		"tag":         args.Tag,
	})
	return "ok", false, nil
}
