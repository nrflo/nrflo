package openaichat

import (
	"encoding/json"
	"fmt"

	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"

	"be/internal/spawner/apirun/provider"
)

// translateRequest converts a provider-neutral Request into Chat Completions
// params. Text + tools only in v1 — multimodal tool results are a
// Responses-API concern (see openai/translate_media.go); local models rarely
// need OCR.
func translateRequest(req provider.Request) (openaisdk.ChatCompletionNewParams, error) {
	params := openaisdk.ChatCompletionNewParams{
		Model:               req.Model,
		MaxCompletionTokens: param.NewOpt(int64(req.MaxTokens)),
		StreamOptions:       openaisdk.ChatCompletionStreamOptionsParam{IncludeUsage: param.NewOpt(true)},
	}

	if req.ReasoningEffort != "" {
		params.ReasoningEffort = shared.ReasoningEffort(req.ReasoningEffort)
	}

	messages := make([]openaisdk.ChatCompletionMessageParamUnion, 0, len(req.Messages)+1)
	if req.System != "" {
		messages = append(messages, openaisdk.SystemMessage(req.System))
	}

	if len(req.Tools) > 0 {
		tools := make([]openaisdk.ChatCompletionToolUnionParam, 0, len(req.Tools))
		for _, t := range req.Tools {
			schema, err := decodeToolSchema(t.InputSchema)
			if err != nil {
				return params, fmt.Errorf("tool %s: %w", t.Name, err)
			}
			fn := shared.FunctionDefinitionParam{
				Name:       t.Name,
				Parameters: schema,
			}
			if t.Description != "" {
				fn.Description = param.NewOpt(t.Description)
			}
			tools = append(tools, openaisdk.ChatCompletionFunctionTool(fn))
		}
		params.Tools = tools
		params.ToolChoice = openaisdk.ChatCompletionToolChoiceOptionUnionParam{OfAuto: param.NewOpt("auto")}
	}

	msgs, err := translateMessages(req.Messages)
	if err != nil {
		return params, err
	}
	messages = append(messages, msgs...)
	params.Messages = messages
	return params, nil
}

func translateMessages(msgs []provider.Message) ([]openaisdk.ChatCompletionMessageParamUnion, error) {
	var out []openaisdk.ChatCompletionMessageParamUnion
	for _, m := range msgs {
		switch m.Role {
		case "assistant":
			item, err := translateAssistantMessage(m)
			if err != nil {
				return nil, err
			}
			if item != nil {
				out = append(out, *item)
			}
		default:
			items, err := translateUserMessage(m)
			if err != nil {
				return nil, err
			}
			out = append(out, items...)
		}
	}
	return out, nil
}

// translateAssistantMessage folds every block of one assistant turn into a
// single ChatCompletionAssistantMessageParam: text blocks concatenate into
// Content, tool_use blocks become ToolCalls.
func translateAssistantMessage(m provider.Message) (*openaisdk.ChatCompletionMessageParamUnion, error) {
	var text string
	var calls []openaisdk.ChatCompletionMessageToolCallUnionParam
	for _, b := range m.Content {
		switch b.Type {
		case "text":
			text += b.Text
		case "tool_use":
			args := "{}"
			if len(b.Input) > 0 {
				args = string(b.Input)
			}
			calls = append(calls, openaisdk.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &openaisdk.ChatCompletionMessageFunctionToolCallParam{
					ID: b.ToolUseID,
					Function: openaisdk.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name:      b.ToolName,
						Arguments: args,
					},
				},
			})
		case "thinking", "redacted_thinking":
			// Not representable on Chat Completions; dropped.
		default:
			return nil, fmt.Errorf("unsupported assistant content block type: %q", b.Type)
		}
	}
	if text == "" && len(calls) == 0 {
		return nil, nil
	}
	assistant := openaisdk.ChatCompletionAssistantMessageParam{ToolCalls: calls}
	if text != "" {
		assistant.Content = openaisdk.ChatCompletionAssistantMessageParamContentUnion{OfString: param.NewOpt(text)}
	}
	item := openaisdk.ChatCompletionMessageParamUnion{OfAssistant: &assistant}
	return &item, nil
}

// translateUserMessage emits one item per block: text -> UserMessage,
// tool_result -> ToolMessage.
func translateUserMessage(m provider.Message) ([]openaisdk.ChatCompletionMessageParamUnion, error) {
	var out []openaisdk.ChatCompletionMessageParamUnion
	for _, b := range m.Content {
		switch b.Type {
		case "text":
			out = append(out, openaisdk.UserMessage(b.Text))
		case "tool_result":
			text := b.Output
			if b.IsError {
				text = "Error: " + text
			}
			out = append(out, openaisdk.ToolMessage(text, b.ToolUseID))
		default:
			return nil, fmt.Errorf("unsupported content block type: %q", b.Type)
		}
	}
	return out, nil
}

func decodeToolSchema(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, fmt.Errorf("invalid tool input schema: %w", err)
	}
	return schema, nil
}
