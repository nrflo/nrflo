package tools_builtin

import (
	"context"
	"encoding/json"
	"fmt"

	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
	"be/internal/types"
	"be/internal/ws"
)

// findingsAddHandler implements findings_add.
type findingsAddHandler struct{}

func (findingsAddHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "findings_add",
		Description: "Set or overwrite a finding key on the current agent's session.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"key":{"type":"string","description":"Finding key"},
"value":{"type":"string","description":"Finding value (string or JSON-encoded value)"}
},
"required":["key","value"],
"additionalProperties":false
}`),
	}
}

func (findingsAddHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if env.Findings == nil {
		return missingService("findings")
	}
	bctx, err := env.Findings.Add(&types.FindingsAddRequest{
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
		"action":     "add",
	})
	return "ok", false, nil
}

// findingsAddBulkHandler implements findings_add_bulk.
type findingsAddBulkHandler struct{}

func (findingsAddBulkHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "findings_add_bulk",
		Description: "Set multiple findings on the current agent's session in one call.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"key_values":{"type":"object","additionalProperties":{"type":"string"},"description":"Map of key -> value (string or JSON-encoded value)"}
},
"required":["key_values"],
"additionalProperties":false
}`),
	}
}

func (findingsAddBulkHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		KeyValues map[string]string `json:"key_values"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if env.Findings == nil {
		return missingService("findings")
	}
	bctx, err := env.Findings.AddBulk(&types.FindingsAddBulkRequest{
		KeyValues:  args.KeyValues,
		SessionID:  env.SessionID,
		InstanceID: env.WorkflowInstanceID,
	})
	if err != nil {
		return err.Error(), true, nil
	}
	service.BroadcastFromCtx(env.WSHub, ws.EventFindingsUpdated, bctx, map[string]interface{}{
		"agent_type": bctx.AgentType,
		"action":     "add-bulk",
		"count":      len(args.KeyValues),
	})
	return "ok", false, nil
}

// findingsAppendHandler implements findings_append.
type findingsAppendHandler struct{}

func (findingsAppendHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "findings_append",
		Description: "Append a value to a finding key on the current agent's session.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"key":{"type":"string"},
"value":{"type":"string","description":"Value to append (string or JSON-encoded value)"}
},
"required":["key","value"],
"additionalProperties":false
}`),
	}
}

func (findingsAppendHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if env.Findings == nil {
		return missingService("findings")
	}
	bctx, err := env.Findings.Append(&types.FindingsAppendRequest{
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
		"action":     "append",
	})
	return "ok", false, nil
}

// findingsAppendBulkHandler implements findings_append_bulk.
type findingsAppendBulkHandler struct{}

func (findingsAppendBulkHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "findings_append_bulk",
		Description: "Append multiple values to findings on the current agent's session.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"key_values":{"type":"object","additionalProperties":{"type":"string"}}
},
"required":["key_values"],
"additionalProperties":false
}`),
	}
}

func (findingsAppendBulkHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		KeyValues map[string]string `json:"key_values"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if env.Findings == nil {
		return missingService("findings")
	}
	bctx, err := env.Findings.AppendBulk(&types.FindingsAppendBulkRequest{
		KeyValues:  args.KeyValues,
		SessionID:  env.SessionID,
		InstanceID: env.WorkflowInstanceID,
	})
	if err != nil {
		return err.Error(), true, nil
	}
	service.BroadcastFromCtx(env.WSHub, ws.EventFindingsUpdated, bctx, map[string]interface{}{
		"agent_type": bctx.AgentType,
		"action":     "append-bulk",
		"count":      len(args.KeyValues),
	})
	return "ok", false, nil
}

// findingsGetHandler implements findings_get.
type findingsGetHandler struct{}

func (findingsGetHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "findings_get",
		Description: "Read findings. Omit agent_type for own session, or pass another agent_type for cross-agent reads.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"agent_type":{"type":"string","description":"Cross-agent read target (omit for own session)"},
"key":{"type":"string","description":"Single finding key"},
"keys":{"type":"array","items":{"type":"string"},"description":"Multiple finding keys"},
"model":{"type":"string","description":"Optional specific model_id when reading cross-agent"}
},
"additionalProperties":false
}`),
	}
}

func (findingsGetHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		AgentType string   `json:"agent_type"`
		Key       string   `json:"key"`
		Keys      []string `json:"keys"`
		Model     string   `json:"model"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &args); err != nil {
			return invalidArgs(err)
		}
	}
	if env.Findings == nil {
		return missingService("findings")
	}
	res, err := env.Findings.Get(&types.FindingsGetRequest{
		AgentType:  args.AgentType,
		Key:        args.Key,
		Keys:       args.Keys,
		Model:      args.Model,
		SessionID:  env.SessionID,
		InstanceID: env.WorkflowInstanceID,
	})
	if err != nil {
		return err.Error(), true, nil
	}
	out, marshalErr := json.Marshal(res)
	if marshalErr != nil {
		return marshalErr.Error(), true, nil
	}
	return string(out), false, nil
}

// findingsDeleteHandler implements findings_delete.
type findingsDeleteHandler struct{}

func (findingsDeleteHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "findings_delete",
		Description: "Delete one or more finding keys from the current agent's session.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"keys":{"type":"array","items":{"type":"string"}}
},
"required":["keys"],
"additionalProperties":false
}`),
	}
}

func (findingsDeleteHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Keys []string `json:"keys"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if env.Findings == nil {
		return missingService("findings")
	}
	bctx, deleted, err := env.Findings.Delete(&types.FindingsDeleteRequest{
		Keys:       args.Keys,
		SessionID:  env.SessionID,
		InstanceID: env.WorkflowInstanceID,
	})
	if err != nil {
		return err.Error(), true, nil
	}
	service.BroadcastFromCtx(env.WSHub, ws.EventFindingsUpdated, bctx, map[string]interface{}{
		"agent_type": bctx.AgentType,
		"action":     "delete",
		"deleted":    deleted,
	})
	return fmt.Sprintf("deleted %d", deleted), false, nil
}

func invalidArgs(err error) (string, bool, error) {
	return fmt.Sprintf("invalid arguments: %s", err.Error()), true, nil
}

func missingService(name string) (string, bool, error) {
	return fmt.Sprintf("%s service not configured", name), true, nil
}
