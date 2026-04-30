package tools_builtin

import (
	"context"
	"encoding/json"

	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
	"be/internal/types"
	"be/internal/ws"
)

type agentFailHandler struct{}

func (agentFailHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "agent_fail",
		Description: "Mark the current agent as failed and stop execution. Optional reason explains the failure.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"reason":{"type":"string"}
},
"additionalProperties":false
}`),
	}
}

func (agentFailHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Reason string `json:"reason"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &args); err != nil {
			return invalidArgs(err)
		}
	}
	if env.Agent == nil {
		return missingService("agent")
	}
	bctx, err := env.Agent.Fail(&types.AgentRequest{
		Reason:     args.Reason,
		SessionID:  env.SessionID,
		InstanceID: env.WorkflowInstanceID,
	})
	if err != nil {
		return err.Error(), true, nil
	}
	service.BroadcastFromCtx(env.WSHub, ws.EventAgentCompleted, bctx, map[string]interface{}{
		"action":     "fail",
		"agent_type": bctx.AgentType,
		"session_id": bctx.SessionID,
		"model_id":   bctx.ModelID,
		"result":     "fail",
	})
	return "", false, apirun.TerminalSignal{Status: "FAIL", Reason: args.Reason}
}

type agentFinishedHandler struct{}

func (agentFinishedHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "agent_finished",
		Description: "Mark the current agent as successfully finished (pass) so the orchestrator advances to the next phase.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}
}

func (agentFinishedHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	if env.Agent == nil {
		return missingService("agent")
	}
	bctx, err := env.Agent.Finished(&types.AgentRequest{
		SessionID:  env.SessionID,
		InstanceID: env.WorkflowInstanceID,
	})
	if err != nil {
		return err.Error(), true, nil
	}
	service.BroadcastFromCtx(env.WSHub, ws.EventAgentCompleted, bctx, map[string]interface{}{
		"action":     "finished",
		"agent_type": bctx.AgentType,
		"session_id": bctx.SessionID,
		"model_id":   bctx.ModelID,
		"result":     "pass",
	})
	return "", false, apirun.TerminalSignal{Status: "PASS"}
}

type agentContinueHandler struct{}

func (agentContinueHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "agent_continue",
		Description: "Mark the current agent as needing context continuation. The orchestrator relaunches a fresh agent with previous-data findings.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}
}

func (agentContinueHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	if env.Agent == nil {
		return missingService("agent")
	}
	bctx, err := env.Agent.Continue(&types.AgentRequest{
		SessionID:  env.SessionID,
		InstanceID: env.WorkflowInstanceID,
	})
	if err != nil {
		return err.Error(), true, nil
	}
	service.BroadcastFromCtx(env.WSHub, ws.EventAgentContinued, bctx, map[string]interface{}{
		"action":     "continue",
		"agent_type": bctx.AgentType,
		"session_id": bctx.SessionID,
		"model_id":   bctx.ModelID,
	})
	return "", false, apirun.TerminalSignal{Status: "CONTINUE"}
}

type agentCallbackHandler struct{}

func (agentCallbackHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "agent_callback",
		Description: "Trigger a callback to re-run an earlier layer. Provide the target level (>=0).",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"level":{"type":"integer","minimum":0,"description":"Layer level to call back to"}
},
"required":["level"],
"additionalProperties":false
}`),
	}
}

func (agentCallbackHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Level int `json:"level"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if env.Agent == nil {
		return missingService("agent")
	}
	bctx, err := env.Agent.Callback(&types.AgentCallbackRequest{
		AgentRequest: types.AgentRequest{
			SessionID:  env.SessionID,
			InstanceID: env.WorkflowInstanceID,
		},
		Level: args.Level,
	})
	if err != nil {
		return err.Error(), true, nil
	}
	service.BroadcastFromCtx(env.WSHub, ws.EventAgentCompleted, bctx, map[string]interface{}{
		"action":     "callback",
		"agent_type": bctx.AgentType,
		"level":      args.Level,
		"model_id":   bctx.ModelID,
		"result":     "callback",
	})
	return "", false, apirun.TerminalSignal{Status: "CALLBACK", Level: args.Level}
}

type agentContextUpdateHandler struct{}

func (agentContextUpdateHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "agent_context_update",
		Description: "Update the agent's context_left percentage. Mostly redundant in API mode (set automatically each turn).",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"context_left":{"type":"integer","minimum":0,"maximum":100}
},
"required":["context_left"],
"additionalProperties":false
}`),
	}
}

func (agentContextUpdateHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		ContextLeft int `json:"context_left"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if env.Agent == nil {
		return missingService("agent")
	}
	projectID, ticketID, workflow, err := env.Agent.UpdateContextLeft(env.SessionID, args.ContextLeft)
	if err != nil {
		return err.Error(), true, nil
	}
	if projectID != "" {
		service.BroadcastFromCtx(env.WSHub, ws.EventAgentContextUpdated, service.BroadcastCtx{
			ProjectID: projectID,
			TicketID:  ticketID,
			Workflow:  workflow,
		}, map[string]interface{}{
			"session_id":   env.SessionID,
			"context_left": args.ContextLeft,
		})
	}
	return "ok", false, nil
}
