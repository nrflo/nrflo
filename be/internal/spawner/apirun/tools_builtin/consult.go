package tools_builtin

import (
	"context"
	"encoding/json"

	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

type consultHandler struct{}

func (consultHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "consult",
		Description: "Ask a named topic-expert consultant a question and receive an inline answer. The consultant runs synchronously and its response is returned directly so you can continue without interruption. Consultants are consultant-flagged agent definitions in your own workflow; your task prompt names the ones available to you — an unknown id fails with an error naming the workflow it was looked up in.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"consultant":{"type":"string","description":"ID of a consultant agent definition in this workflow"},
"question":{"type":"string","description":"The question or task to send to the consultant"}
},
"required":["consultant","question"],
"additionalProperties":false
}`),
	}
}

func (consultHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Consultant string `json:"consultant"`
		Question   string `json:"question"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if env.Consultant == nil {
		return missingService("consultant")
	}
	answer, err := env.Consultant.Consult(ctx, env.SessionID, args.Consultant, args.Question)
	if err != nil {
		return err.Error(), true, nil
	}
	return answer, false, nil
}
