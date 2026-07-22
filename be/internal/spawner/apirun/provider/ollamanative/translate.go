package ollamanative

import (
	"encoding/json"
	"fmt"

	"be/internal/spawner/apirun/provider"
)

// chatRequest is the body shape for Ollama's native POST /api/chat.
type chatRequest struct {
	Model    string        `json:"model"`
	Stream   bool          `json:"stream"`
	Think    bool          `json:"think"`
	Messages []chatMessage `json:"messages"`
	Tools    []chatTool    `json:"tools,omitempty"`
	Options  *chatOptions  `json:"options,omitempty"`
}

type chatMessage struct {
	Role      string         `json:"role"`
	Content   string         `json:"content,omitempty"`
	ToolCalls []chatToolCall `json:"tool_calls,omitempty"`
}

type chatToolCall struct {
	Function chatToolCallFunction `json:"function"`
}

type chatToolCallFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type chatTool struct {
	Type     string       `json:"type"`
	Function chatToolFunc `json:"function"`
}

type chatToolFunc struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

type chatOptions struct {
	NumPredict int `json:"num_predict,omitempty"`
}

// translateRequest converts a provider-neutral Request into an Ollama native
// /api/chat body. think is the effort-derived switch: "" and "none" mean
// think:false, any other recognized level means think:true — see
// be/internal/service/model_reasoning.go for how "none" reaches api_efforts.
func translateRequest(req provider.Request) (chatRequest, error) {
	body := chatRequest{
		Model:  req.Model,
		Stream: true,
		Think:  req.ReasoningEffort != "" && req.ReasoningEffort != "none",
	}

	messages := make([]chatMessage, 0, len(req.Messages)+1)
	if req.System != "" {
		messages = append(messages, chatMessage{Role: "system", Content: req.System})
	}

	if len(req.Tools) > 0 {
		tools := make([]chatTool, 0, len(req.Tools))
		for _, t := range req.Tools {
			schema, err := decodeToolSchema(t.InputSchema)
			if err != nil {
				return body, fmt.Errorf("tool %s: %w", t.Name, err)
			}
			tools = append(tools, chatTool{
				Type: "function",
				Function: chatToolFunc{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  schema,
				},
			})
		}
		body.Tools = tools
	}

	msgs, err := translateMessages(req.Messages)
	if err != nil {
		return body, err
	}
	messages = append(messages, msgs...)
	body.Messages = messages

	if req.MaxTokens > 0 {
		body.Options = &chatOptions{NumPredict: req.MaxTokens}
	}
	return body, nil
}

func translateMessages(msgs []provider.Message) ([]chatMessage, error) {
	var out []chatMessage
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
// single chatMessage: text blocks concatenate into Content, tool_use blocks
// become ToolCalls (arguments taken verbatim from the stored Input bytes).
func translateAssistantMessage(m provider.Message) (*chatMessage, error) {
	var text string
	var calls []chatToolCall
	for _, b := range m.Content {
		switch b.Type {
		case "text":
			text += b.Text
		case "tool_use":
			args := b.Input
			if len(args) == 0 {
				args = json.RawMessage("{}")
			}
			calls = append(calls, chatToolCall{
				Function: chatToolCallFunction{Name: b.ToolName, Arguments: args},
			})
		case "thinking", "redacted_thinking":
			// Not representable on the native chat wire; dropped.
		default:
			return nil, fmt.Errorf("unsupported assistant content block type: %q", b.Type)
		}
	}
	if text == "" && len(calls) == 0 {
		return nil, nil
	}
	return &chatMessage{Role: "assistant", Content: text, ToolCalls: calls}, nil
}

// translateUserMessage emits one item per block: text -> user message,
// tool_result -> tool message.
func translateUserMessage(m provider.Message) ([]chatMessage, error) {
	var out []chatMessage
	for _, b := range m.Content {
		switch b.Type {
		case "text":
			out = append(out, chatMessage{Role: "user", Content: b.Text})
		case "tool_result":
			text := b.Output
			if b.IsError {
				text = "Error: " + text
			}
			out = append(out, chatMessage{Role: "tool", Content: text})
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
