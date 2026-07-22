package ollamanative

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"be/internal/spawner/apirun/provider"
)

// toolCallAcc tracks one buffered tool call: Ollama's native wire has no
// streaming deltas for tool calls (each arrives complete, with no id or
// index), so ids are synthesized sequentially in encounter order.
type toolCallAcc struct {
	id   string
	name string
	args json.RawMessage
}

// chatChunk is one NDJSON line from Ollama's native /api/chat stream.
type chatChunk struct {
	Message struct {
		Content   string `json:"content"`
		ToolCalls []struct {
			Function struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	} `json:"message"`
	Done            bool   `json:"done"`
	DoneReason      string `json:"done_reason"`
	PromptEvalCount int    `json:"prompt_eval_count"`
	EvalCount       int    `json:"eval_count"`
}

// decodeStream reads NDJSON line-by-line from an Ollama native /api/chat
// response, emitting sink callbacks and assembling the FinalResponse. Lines
// can exceed the default bufio.Scanner 64KB token limit (large tool-call
// arguments), so the scanner buffer is enlarged.
func decodeStream(r io.Reader, sink provider.EventSink) (*provider.FinalResponse, error) {
	final := &provider.FinalResponse{}
	var text string
	var calls []*toolCallAcc
	doneReason := ""

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var chunk chatChunk
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			return nil, fmt.Errorf("decode ollamanative chunk: %w", err)
		}

		if chunk.Message.Content != "" {
			text += chunk.Message.Content
			sink.OnTextDelta(chunk.Message.Content)
		}

		for _, tc := range chunk.Message.ToolCalls {
			args := tc.Function.Arguments
			if len(args) == 0 {
				args = json.RawMessage("{}")
			}
			id := fmt.Sprintf("call_%d", len(calls))
			acc := &toolCallAcc{id: id, name: tc.Function.Name, args: args}
			calls = append(calls, acc)
			sink.OnToolUseStart(acc.id, acc.name)
			sink.OnToolUseInputDelta(acc.id, string(acc.args))
		}

		if chunk.Done {
			doneReason = chunk.DoneReason
			final.Usage = provider.Usage{
				InputTokens:  chunk.PromptEvalCount,
				OutputTokens: chunk.EvalCount,
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read ollamanative stream: %w", err)
	}

	if text != "" {
		final.Content = append(final.Content, provider.ContentBlock{Type: "text", Text: text})
	}
	for _, acc := range calls {
		if !json.Valid(acc.args) {
			return nil, fmt.Errorf("tool_call %s: invalid JSON input %q", acc.id, string(acc.args))
		}
		sink.OnToolUseStop(acc.id, acc.args)
		final.Content = append(final.Content, provider.ContentBlock{
			Type:      "tool_use",
			ToolUseID: acc.id,
			ToolName:  acc.name,
			Input:     acc.args,
		})
	}

	if len(calls) > 0 {
		final.StopReason = "tool_use"
	} else {
		final.StopReason = mapDoneReason(doneReason)
	}
	sink.OnUsage(final.Usage)
	return final, nil
}

// mapDoneReason maps Ollama's done_reason to the provider-neutral StopReason
// vocabulary.
func mapDoneReason(reason string) string {
	if reason == "length" {
		return "max_tokens"
	}
	return "end_turn"
}
