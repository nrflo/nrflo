package openai

import (
	"encoding/json"
	"fmt"

	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/packages/ssestream"
	"github.com/openai/openai-go/responses"
	"github.com/openai/openai-go/shared"

	"be/internal/spawner/apirun/provider"
)

// translateRequest converts a provider-neutral Request into the Responses API params.
func translateRequest(req provider.Request) (responses.ResponseNewParams, error) {
	params := responses.ResponseNewParams{
		Model:           req.Model,
		MaxOutputTokens: param.NewOpt(int64(req.MaxTokens)),
		Store:           param.NewOpt(false),
		ToolChoice:      responses.ResponseNewParamsToolChoiceUnion{OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsAuto)},
	}

	if req.System != "" {
		params.Instructions = param.NewOpt(req.System)
	}

	if req.ReasoningEffort != "" {
		params.Reasoning = shared.ReasoningParam{Effort: shared.ReasoningEffort(req.ReasoningEffort)}
	}

	if len(req.Tools) > 0 {
		tools := make([]responses.ToolUnionParam, 0, len(req.Tools))
		for _, t := range req.Tools {
			schema, err := decodeToolSchema(t.InputSchema)
			if err != nil {
				return params, fmt.Errorf("tool %s: %w", t.Name, err)
			}
			tp := responses.FunctionToolParam{
				Name:       t.Name,
				Parameters: schema,
				Strict:     param.NewOpt(false),
			}
			if t.Description != "" {
				tp.Description = param.NewOpt(t.Description)
			}
			tools = append(tools, responses.ToolUnionParam{OfFunction: &tp})
		}
		params.Tools = tools
	}

	if len(req.Messages) > 0 {
		items, err := translateMessages(req.Messages)
		if err != nil {
			return params, err
		}
		params.Input = responses.ResponseNewParamsInputUnion{OfInputItemList: responses.ResponseInputParam(items)}
	}

	return params, nil
}

func translateMessages(msgs []provider.Message) ([]responses.ResponseInputItemUnionParam, error) {
	var items []responses.ResponseInputItemUnionParam
	for _, m := range msgs {
		role := responses.EasyInputMessageRole(m.Role)
		for _, b := range m.Content {
			switch b.Type {
			case "text":
				items = append(items, responses.ResponseInputItemUnionParam{
					OfMessage: &responses.EasyInputMessageParam{
						Role:    role,
						Content: responses.EasyInputMessageContentUnionParam{OfString: param.NewOpt(b.Text)},
					},
				})
			case "tool_use":
				args := "{}"
				if len(b.Input) > 0 {
					args = string(b.Input)
				}
				items = append(items, responses.ResponseInputItemParamOfFunctionCall(args, b.ToolUseID, b.ToolName))
			case "tool_result":
				out := b.Output
				if b.IsError {
					out = "Error: " + out
				}
				items = append(items, responses.ResponseInputItemParamOfFunctionCallOutput(b.ToolUseID, out))
				if len(b.OutputMedia) > 0 {
					item, err := mediaFollowupItem(b)
					if err != nil {
						return nil, err
					}
					items = append(items, item)
				}
			default:
				return nil, fmt.Errorf("unsupported content block type: %q", b.Type)
			}
		}
	}
	return items, nil
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

// itemAcc tracks per-output-index streaming state.
type itemAcc struct {
	kind    string // "message" | "function_call"
	text    string
	callID  string
	name    string
	args    string
	stopped bool // OnToolUseStop already called
}

// decodeStream drives a Responses API stream to completion, emitting sink
// callbacks and assembling the FinalResponse.
func decodeStream(stream *ssestream.Stream[responses.ResponseStreamEventUnion], sink provider.EventSink) (*provider.FinalResponse, error) {
	final := &provider.FinalResponse{}
	accs := map[int64]*itemAcc{}

	for stream.Next() {
		ev := stream.Current()
		switch ev.Type {
		case "response.output_item.added":
			acc := &itemAcc{kind: ev.Item.Type}
			if ev.Item.Type == "function_call" {
				acc.callID = ev.Item.CallID
				acc.name = ev.Item.Name
				sink.OnToolUseStart(ev.Item.CallID, ev.Item.Name)
			}
			accs[ev.OutputIndex] = acc

		case "response.output_text.delta":
			if acc, ok := accs[ev.OutputIndex]; ok {
				acc.text += ev.Text
			}
			sink.OnTextDelta(ev.Text)

		case "response.function_call_arguments.delta":
			// The incremental chunk lives in the union's "delta" field
			// (ResponseFunctionCallArgumentsDeltaEvent.Delta); "arguments" is
			// only populated on the ".done" variant.
			if acc, ok := accs[ev.OutputIndex]; ok {
				acc.args += ev.Delta.OfString
				sink.OnToolUseInputDelta(acc.callID, ev.Delta.OfString)
			}

		case "response.function_call_arguments.done":
			acc, ok := accs[ev.OutputIndex]
			if !ok || acc.stopped {
				continue
			}
			raw := acc.args
			if raw == "" {
				raw = "{}"
			}
			if !json.Valid([]byte(raw)) {
				return nil, fmt.Errorf("function_call %s: invalid JSON input %q", acc.callID, raw)
			}
			sink.OnToolUseStop(acc.callID, json.RawMessage(raw))
			acc.stopped = true

		case "response.output_item.done":
			acc, ok := accs[ev.OutputIndex]
			if !ok {
				continue
			}
			block, err := finalizeItemAcc(acc, sink)
			if err != nil {
				return nil, err
			}
			if block != nil {
				final.Content = append(final.Content, *block)
			}
			delete(accs, ev.OutputIndex)

		case "response.completed":
			final.Usage = fromResponseUsage(ev.Response.Usage)
			final.StopReason = resolveStopReason(ev.Response, final.Content)
			sink.OnUsage(final.Usage)
			return final, nil
		}
	}

	if err := stream.Err(); err != nil {
		return nil, err
	}
	sink.OnUsage(final.Usage)
	return final, nil
}

func finalizeItemAcc(acc *itemAcc, sink provider.EventSink) (*provider.ContentBlock, error) {
	switch acc.kind {
	case "function_call":
		raw := acc.args
		if raw == "" {
			raw = "{}"
		}
		if !json.Valid([]byte(raw)) {
			return nil, fmt.Errorf("function_call %s: invalid JSON input %q", acc.callID, raw)
		}
		if !acc.stopped {
			sink.OnToolUseStop(acc.callID, json.RawMessage(raw))
		}
		return &provider.ContentBlock{
			Type:      "tool_use",
			ToolUseID: acc.callID,
			ToolName:  acc.name,
			Input:     json.RawMessage(raw),
		}, nil
	case "message":
		if acc.text == "" {
			return nil, nil
		}
		return &provider.ContentBlock{Type: "text", Text: acc.text}, nil
	default:
		return nil, nil
	}
}

func fromResponseUsage(u responses.ResponseUsage) provider.Usage {
	return provider.Usage{
		InputTokens:  int(u.InputTokens),
		OutputTokens: int(u.OutputTokens),
	}
}

func resolveStopReason(resp responses.Response, content []provider.ContentBlock) string {
	for _, b := range content {
		if b.Type == "tool_use" {
			return "tool_use"
		}
	}
	if resp.Status == responses.ResponseStatusIncomplete || resp.IncompleteDetails.Reason == "max_output_tokens" {
		return "max_tokens"
	}
	return "end_turn"
}
