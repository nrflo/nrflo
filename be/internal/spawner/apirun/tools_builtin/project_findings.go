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

func projectBroadcastCtx(env apirun.ToolEnv) service.BroadcastCtx {
	return service.BroadcastCtx{
		ProjectID: env.ProjectID,
		TicketID:  env.TicketID,
		Workflow:  env.WorkflowName,
		AgentType: env.AgentType,
		SessionID: env.SessionID,
	}
}

type projectFindingsAddHandler struct{}

func (projectFindingsAddHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "project_findings_add",
		Description: "Set or overwrite a project-level finding key.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"key":{"type":"string"},
"value":{"type":"string","description":"String or JSON-encoded value"}
},
"required":["key","value"],
"additionalProperties":false
}`),
	}
}

func (projectFindingsAddHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if env.ProjectFindings == nil {
		return missingService("project_findings")
	}
	if err := env.ProjectFindings.Add(env.ProjectID, &types.ProjectFindingsAddRequest{Key: args.Key, Value: args.Value}); err != nil {
		return err.Error(), true, nil
	}
	service.BroadcastFromCtx(env.WSHub, ws.EventProjectFindingsUpdated, projectBroadcastCtx(env), map[string]interface{}{
		"key":    args.Key,
		"action": "add",
	})
	return "ok", false, nil
}

type projectFindingsAddBulkHandler struct{}

func (projectFindingsAddBulkHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "project_findings_add_bulk",
		Description: "Set multiple project-level findings in one call.",
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

func (projectFindingsAddBulkHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		KeyValues map[string]string `json:"key_values"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if env.ProjectFindings == nil {
		return missingService("project_findings")
	}
	if err := env.ProjectFindings.AddBulk(env.ProjectID, &types.ProjectFindingsAddBulkRequest{KeyValues: args.KeyValues}); err != nil {
		return err.Error(), true, nil
	}
	service.BroadcastFromCtx(env.WSHub, ws.EventProjectFindingsUpdated, projectBroadcastCtx(env), map[string]interface{}{
		"action": "add-bulk",
		"count":  len(args.KeyValues),
	})
	return "ok", false, nil
}

type projectFindingsAppendHandler struct{}

func (projectFindingsAppendHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "project_findings_append",
		Description: "Append a value to a project-level finding.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"key":{"type":"string"},
"value":{"type":"string"}
},
"required":["key","value"],
"additionalProperties":false
}`),
	}
}

func (projectFindingsAppendHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if env.ProjectFindings == nil {
		return missingService("project_findings")
	}
	if err := env.ProjectFindings.Append(env.ProjectID, &types.ProjectFindingsAppendRequest{Key: args.Key, Value: args.Value}); err != nil {
		return err.Error(), true, nil
	}
	service.BroadcastFromCtx(env.WSHub, ws.EventProjectFindingsUpdated, projectBroadcastCtx(env), map[string]interface{}{
		"key":    args.Key,
		"action": "append",
	})
	return "ok", false, nil
}

type projectFindingsAppendBulkHandler struct{}

func (projectFindingsAppendBulkHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "project_findings_append_bulk",
		Description: "Append multiple values to project-level findings.",
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

func (projectFindingsAppendBulkHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		KeyValues map[string]string `json:"key_values"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if env.ProjectFindings == nil {
		return missingService("project_findings")
	}
	if err := env.ProjectFindings.AppendBulk(env.ProjectID, &types.ProjectFindingsAppendBulkRequest{KeyValues: args.KeyValues}); err != nil {
		return err.Error(), true, nil
	}
	service.BroadcastFromCtx(env.WSHub, ws.EventProjectFindingsUpdated, projectBroadcastCtx(env), map[string]interface{}{
		"action": "append-bulk",
		"count":  len(args.KeyValues),
	})
	return "ok", false, nil
}

type projectFindingsGetHandler struct{}

func (projectFindingsGetHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "project_findings_get",
		Description: "Read project-level findings (single key, multiple keys, or all).",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"key":{"type":"string"},
"keys":{"type":"array","items":{"type":"string"}}
},
"additionalProperties":false
}`),
	}
}

func (projectFindingsGetHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Key  string   `json:"key"`
		Keys []string `json:"keys"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &args); err != nil {
			return invalidArgs(err)
		}
	}
	if env.ProjectFindings == nil {
		return missingService("project_findings")
	}
	res, err := env.ProjectFindings.Get(env.ProjectID, &types.ProjectFindingsGetRequest{Key: args.Key, Keys: args.Keys})
	if err != nil {
		return err.Error(), true, nil
	}
	out, marshalErr := json.Marshal(res)
	if marshalErr != nil {
		return marshalErr.Error(), true, nil
	}
	return string(out), false, nil
}

type projectFindingsDeleteHandler struct{}

func (projectFindingsDeleteHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "project_findings_delete",
		Description: "Delete one or more project-level finding keys.",
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

func (projectFindingsDeleteHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Keys []string `json:"keys"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if env.ProjectFindings == nil {
		return missingService("project_findings")
	}
	deleted, err := env.ProjectFindings.Delete(env.ProjectID, &types.ProjectFindingsDeleteRequest{Keys: args.Keys})
	if err != nil {
		return err.Error(), true, nil
	}
	service.BroadcastFromCtx(env.WSHub, ws.EventProjectFindingsUpdated, projectBroadcastCtx(env), map[string]interface{}{
		"action":  "delete",
		"deleted": deleted,
	})
	return fmt.Sprintf("deleted %d", len(deleted)), false, nil
}
