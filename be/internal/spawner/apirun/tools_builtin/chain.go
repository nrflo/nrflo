package tools_builtin

import (
	"context"
	"encoding/json"

	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

type chainNextInstructionsHandler struct{}

func (chainNextInstructionsHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "chain_next_instructions",
		Description: "Set the instructions the next step in the current workflow chain run will receive. Call before finishing if this run is part of a chain.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"instructions":{"type":"string"}
},
"required":["instructions"],
"additionalProperties":false
}`),
	}
}

func (chainNextInstructionsHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Instructions string `json:"instructions"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if env.ChainRun == nil {
		return missingService("chain_run")
	}
	if err := env.ChainRun.SetNextStepInstructions(env.WorkflowInstanceID, args.Instructions); err != nil {
		return err.Error(), true, nil
	}
	return "ok", false, nil
}

type chainNextTicketHandler struct{}

func (chainNextTicketHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "chain_next_ticket",
		Description: "Set the ticket ID the next ticket-scope step in the current workflow chain run will operate on. Call before finishing if this run is part of a chain.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"ticket_id":{"type":"string"}
},
"required":["ticket_id"],
"additionalProperties":false
}`),
	}
}

func (chainNextTicketHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		TicketID string `json:"ticket_id"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if env.ChainRun == nil {
		return missingService("chain_run")
	}
	if err := env.ChainRun.SetNextStepTicket(env.WorkflowInstanceID, args.TicketID); err != nil {
		return err.Error(), true, nil
	}
	return "ok", false, nil
}
