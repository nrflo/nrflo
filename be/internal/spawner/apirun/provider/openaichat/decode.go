package openaichat

import (
	"encoding/json"
	"fmt"

	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/ssestream"

	"be/internal/spawner/apirun/provider"
)

// toolCallAcc tracks per-index streaming tool-call assembly (Chat Completions
// deltas key tool calls by array index, not a stable id — the id/name arrive
// on the first delta for that index, arguments accumulate across deltas).
type toolCallAcc struct {
	id      string
	name    string
	args    string
	started bool
}

// decodeStream drives a Chat Completions stream to completion, emitting sink
// callbacks and assembling the FinalResponse.
func decodeStream(stream *ssestream.Stream[openaisdk.ChatCompletionChunk], sink provider.EventSink) (*provider.FinalResponse, error) {
	final := &provider.FinalResponse{}
	var text string
	calls := map[int64]*toolCallAcc{}
	var order []int64
	stopReason := "end_turn"

	for stream.Next() {
		chunk := stream.Current()
		if len(chunk.Choices) > 0 {
			choice := chunk.Choices[0]
			delta := choice.Delta

			if delta.Content != "" {
				text += delta.Content
				sink.OnTextDelta(delta.Content)
			}

			for _, tc := range delta.ToolCalls {
				acc, ok := calls[tc.Index]
				if !ok {
					acc = &toolCallAcc{}
					calls[tc.Index] = acc
					order = append(order, tc.Index)
				}
				if tc.ID != "" {
					acc.id = tc.ID
				}
				if tc.Function.Name != "" {
					acc.name += tc.Function.Name
				}
				if !acc.started && acc.id != "" && acc.name != "" {
					sink.OnToolUseStart(acc.id, acc.name)
					acc.started = true
				}
				if tc.Function.Arguments != "" {
					acc.args += tc.Function.Arguments
					if acc.started {
						sink.OnToolUseInputDelta(acc.id, tc.Function.Arguments)
					}
				}
			}

			if choice.FinishReason != "" {
				stopReason = mapFinishReason(choice.FinishReason)
			}
		}

		if chunk.Usage.TotalTokens > 0 {
			final.Usage = provider.Usage{
				InputTokens:  int(chunk.Usage.PromptTokens),
				OutputTokens: int(chunk.Usage.CompletionTokens),
			}
		}
	}

	if err := stream.Err(); err != nil {
		return nil, err
	}

	if text != "" {
		final.Content = append(final.Content, provider.ContentBlock{Type: "text", Text: text})
	}
	for _, idx := range order {
		acc := calls[idx]
		raw := acc.args
		if raw == "" {
			raw = "{}"
		}
		if !json.Valid([]byte(raw)) {
			return nil, fmt.Errorf("tool_call %s: invalid JSON input %q", acc.id, raw)
		}
		if !acc.started {
			sink.OnToolUseStart(acc.id, acc.name)
		}
		sink.OnToolUseStop(acc.id, json.RawMessage(raw))
		final.Content = append(final.Content, provider.ContentBlock{
			Type:      "tool_use",
			ToolUseID: acc.id,
			ToolName:  acc.name,
			Input:     json.RawMessage(raw),
		})
	}

	final.StopReason = stopReason
	sink.OnUsage(final.Usage)
	return final, nil
}

// mapFinishReason maps a Chat Completions finish_reason to the provider-neutral
// StopReason vocabulary.
func mapFinishReason(reason string) string {
	switch reason {
	case "tool_calls":
		return "tool_use"
	case "length":
		return "max_tokens"
	case "content_filter":
		return "refusal"
	default:
		return "end_turn"
	}
}
