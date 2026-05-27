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

// emitFindingsHandler implements emit_findings: validate a finding value against
// the workflow-scoped schema registered for its key, then store it. On a schema
// mismatch (or an unknown key) it returns an error result containing the
// validation message and a known-good example so the agent can fix and retry.
type emitFindingsHandler struct{}

func (emitFindingsHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "emit_findings",
		Description: "Validate and store a finding against the workflow's configured schema for the given key. The value must be a JSON array matching the key's schema. On a validation failure (or an unknown key) the call is rejected and the error result includes the required-structure example — fix the value and call again.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"key":{"type":"string","description":"Finding key (must have a schema configured for this workflow)"},
"value":{"type":"array","description":"Finding value as a JSON array; validated against the key's configured schema"}
},
"required":["key","value"],
"additionalProperties":false
}`),
	}
}

func (emitFindingsHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Key   string          `json:"key"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if env.Findings == nil {
		return missingService("findings")
	}
	bctx, err := env.Findings.Emit(&types.FindingsEmitRequest{
		Key:        args.Key,
		Value:      args.Value,
		SessionID:  env.SessionID,
		InstanceID: env.WorkflowInstanceID,
	})
	if err != nil {
		return err.Error(), true, nil
	}
	service.BroadcastFromCtx(env.WSHub, ws.EventFindingsUpdated, bctx, map[string]interface{}{
		"agent_type": bctx.AgentType,
		"key":        args.Key,
		"action":     "emit",
	})
	return "ok", false, nil
}
