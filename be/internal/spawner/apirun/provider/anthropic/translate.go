package anthropic

import (
	"encoding/json"
	"fmt"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	"be/internal/spawner/apirun/provider"
)

// translateRequest converts a provider-neutral Request into the SDK's
// MessageNewParams shape. Cache breakpoints are applied here so callers and
// tests can inspect the result without going through the network.
func translateRequest(req provider.Request) (sdk.MessageNewParams, error) {
	params := sdk.MessageNewParams{
		Model:     sdk.Model(req.Model),
		MaxTokens: int64(req.MaxTokens),
	}

	wantSystemCache := false
	wantToolsCache := false
	for _, b := range req.CacheBreakpoints {
		switch b.Target {
		case provider.CacheTargetSystem:
			wantSystemCache = true
		case provider.CacheTargetTools:
			wantToolsCache = true
		}
	}

	if req.System != "" {
		sys := sdk.TextBlockParam{Text: req.System}
		if wantSystemCache {
			sys.CacheControl = sdk.CacheControlEphemeralParam{}
		}
		params.System = []sdk.TextBlockParam{sys}
	}

	if len(req.Tools) > 0 {
		tools := make([]sdk.ToolUnionParam, 0, len(req.Tools))
		for _, t := range req.Tools {
			tp := sdk.ToolParam{Name: t.Name}
			if t.Description != "" {
				tp.Description = param.NewOpt(t.Description)
			}
			if len(t.InputSchema) > 0 {
				schema, err := decodeToolInputSchema(t.InputSchema)
				if err != nil {
					return params, fmt.Errorf("tool %s: %w", t.Name, err)
				}
				tp.InputSchema = schema
			}
			tools = append(tools, sdk.ToolUnionParam{OfTool: &tp})
		}
		// Anthropic only honors cache_control on the LAST tool; setting it on
		// any earlier tool wastes a breakpoint slot.
		if wantToolsCache {
			last := tools[len(tools)-1].OfTool
			last.CacheControl = sdk.CacheControlEphemeralParam{}
		}
		params.Tools = tools
	}

	if len(req.Messages) > 0 {
		msgs := make([]sdk.MessageParam, 0, len(req.Messages))
		for _, m := range req.Messages {
			content, err := translateContentBlocks(m.Content)
			if err != nil {
				return params, err
			}
			msgs = append(msgs, sdk.MessageParam{
				Role:    sdk.MessageParamRole(m.Role),
				Content: content,
			})
		}
		params.Messages = msgs
	}

	switch req.ToolChoice {
	case "", "auto":
		params.ToolChoice = sdk.ToolChoiceUnionParam{OfAuto: &sdk.ToolChoiceAutoParam{}}
	default:
		return params, fmt.Errorf("unsupported tool_choice: %q", req.ToolChoice)
	}

	return params, nil
}

func translateContentBlocks(blocks []provider.ContentBlock) ([]sdk.ContentBlockParamUnion, error) {
	out := make([]sdk.ContentBlockParamUnion, 0, len(blocks))
	for _, b := range blocks {
		switch b.Type {
		case "text":
			out = append(out, sdk.ContentBlockParamUnion{
				OfText: &sdk.TextBlockParam{Text: b.Text},
			})
		case "tool_use":
			var input any
			if len(b.Input) > 0 {
				if err := json.Unmarshal(b.Input, &input); err != nil {
					return nil, fmt.Errorf("tool_use %s: invalid input JSON: %w", b.ToolUseID, err)
				}
			} else {
				input = map[string]any{}
			}
			out = append(out, sdk.ContentBlockParamUnion{
				OfToolUse: &sdk.ToolUseBlockParam{
					ID:    b.ToolUseID,
					Name:  b.ToolName,
					Input: input,
				},
			})
		case "tool_result":
			tr := &sdk.ToolResultBlockParam{
				ToolUseID: b.ToolUseID,
			}
			if b.IsError {
				tr.IsError = param.NewOpt(true)
			}
			if b.Output != "" {
				tr.Content = []sdk.ToolResultBlockParamContentUnion{
					{OfText: &sdk.TextBlockParam{Text: b.Output}},
				}
			}
			out = append(out, sdk.ContentBlockParamUnion{OfToolResult: tr})
		default:
			return nil, fmt.Errorf("unsupported content block type: %q", b.Type)
		}
	}
	return out, nil
}

// decodeToolInputSchema parses the raw JSON schema from ToolSpec into the
// SDK's structured ToolInputSchemaParam. The SDK requires Properties / Required
// to be populated as Go values; we marshal-then-unmarshal via a generic map.
func decodeToolInputSchema(raw json.RawMessage) (sdk.ToolInputSchemaParam, error) {
	var schema sdk.ToolInputSchemaParam
	var generic struct {
		Properties any            `json:"properties"`
		Required   []string       `json:"required"`
		Extra      map[string]any `json:"-"`
	}
	if err := json.Unmarshal(raw, &generic); err != nil {
		return schema, fmt.Errorf("invalid tool input schema: %w", err)
	}
	schema.Properties = generic.Properties
	schema.Required = generic.Required

	// Preserve extra fields (e.g. additionalProperties) for the API.
	var all map[string]any
	if err := json.Unmarshal(raw, &all); err == nil {
		extras := map[string]any{}
		for k, v := range all {
			switch k {
			case "type", "properties", "required":
				continue
			}
			extras[k] = v
		}
		if len(extras) > 0 {
			schema.ExtraFields = extras
		}
	}
	return schema, nil
}
