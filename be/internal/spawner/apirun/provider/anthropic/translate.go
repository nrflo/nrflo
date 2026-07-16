package anthropic

import (
	"encoding/json"
	"fmt"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	"be/internal/spawner/apirun/provider"
)

// thinkingBudget maps a reasoning effort string to a thinking token budget.
// Zero means thinking is disabled. Any non-zero budget enables extended thinking.
func thinkingBudget(effort string) int {
	switch effort {
	case "":
		return 0
	case "low":
		return 4096
	case "medium":
		return 8192
	case "high":
		return 16384
	case "xhigh":
		return 24576
	case "max":
		return 32768
	default:
		return 8192 // unknown non-empty → medium
	}
}

// translateRequest converts a provider-neutral Request into the SDK's
// MessageNewParams shape. Cache breakpoints are applied here so callers and
// tests can inspect the result without going through the network.
func translateRequest(req provider.Request) (sdk.MessageNewParams, error) {
	// Strip the "[1m]" context marker — it's a Claude Code convention and 404s
	// on the API. It selects the context window (see MaxContext), not the model.
	model := stripContextSuffix(req.Model)
	params := sdk.MessageNewParams{
		Model:     model,
		MaxTokens: int64(req.MaxTokens),
	}

	// Thinking is opt-in via reasoning_effort. Current 1M-native models use adaptive thinking
	// + the effort output-config (the enabled+budget shape is rejected with a 400
	// on Opus 4.7/4.8). Haiku 4.5 and older keep the enabled+budget path — they
	// have no adaptive mode and reject the effort parameter.
	if req.ReasoningEffort != "" {
		if isAdaptiveMillionModel(model) {
			params.Thinking = sdk.ThinkingConfigParamUnion{OfAdaptive: &sdk.ThinkingConfigAdaptiveParam{}}
			params.OutputConfig = sdk.OutputConfigParam{Effort: effortParam(req.ReasoningEffort)}
		} else if budget := thinkingBudget(req.ReasoningEffort); budget > 0 {
			params.Thinking = sdk.ThinkingConfigParamOfEnabled(int64(budget))
			minTokens := int64(budget + 4096)
			if params.MaxTokens < minTokens {
				params.MaxTokens = minTokens
			}
		}
	}

	wantSystemCache := false
	wantToolsCache := false
	wantMessageCache := false
	for _, b := range req.CacheBreakpoints {
		switch b.Target {
		case provider.CacheTargetSystem:
			wantSystemCache = true
		case provider.CacheTargetTools:
			wantToolsCache = true
		case provider.CacheTargetMessage:
			wantMessageCache = true
		}
	}

	if req.System != "" {
		sys := sdk.TextBlockParam{Text: req.System}
		if wantSystemCache {
			sys.CacheControl = ephemeralCacheControl()
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
			last.CacheControl = ephemeralCacheControl()
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
		if wantMessageCache {
			budget := maxCacheBreakpoints
			if wantSystemCache {
				budget--
			}
			if wantToolsCache {
				budget--
			}
			applyMessageCacheBreakpoints(msgs, budget)
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
		case "thinking":
			out = append(out, sdk.NewThinkingBlock(b.Signature, b.Text))
		case "redacted_thinking":
			out = append(out, sdk.NewRedactedThinkingBlock(b.Data))
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
			var content []sdk.ToolResultBlockParamContentUnion
			if b.Output != "" {
				content = append(content, sdk.ToolResultBlockParamContentUnion{
					OfText: &sdk.TextBlockParam{Text: b.Output},
				})
			}
			for _, m := range b.OutputMedia {
				mb, err := translateMediaBlock(m)
				if err != nil {
					return nil, fmt.Errorf("tool_result %s: %w", b.ToolUseID, err)
				}
				content = append(content, mb)
			}
			tr.Content = content
			out = append(out, sdk.ContentBlockParamUnion{OfToolResult: tr})
		default:
			return nil, fmt.Errorf("unsupported content block type: %q", b.Type)
		}
	}
	return out, nil
}

// translateMediaBlock maps a provider.MediaBlock into the SDK's tool_result
// content union (image or document base64 source).
func translateMediaBlock(m provider.MediaBlock) (sdk.ToolResultBlockParamContentUnion, error) {
	switch m.Kind {
	case "image":
		var mt sdk.Base64ImageSourceMediaType
		switch m.MediaType {
		case "image/jpeg":
			mt = sdk.Base64ImageSourceMediaTypeImageJPEG
		case "image/png":
			mt = sdk.Base64ImageSourceMediaTypeImagePNG
		case "image/gif":
			mt = sdk.Base64ImageSourceMediaTypeImageGIF
		case "image/webp":
			mt = sdk.Base64ImageSourceMediaTypeImageWebP
		default:
			return sdk.ToolResultBlockParamContentUnion{}, fmt.Errorf("unsupported image media type: %q", m.MediaType)
		}
		return sdk.ToolResultBlockParamContentUnion{
			OfImage: &sdk.ImageBlockParam{
				Source: sdk.ImageBlockParamSourceUnion{
					OfBase64: &sdk.Base64ImageSourceParam{Data: m.DataB64, MediaType: mt},
				},
			},
		}, nil
	case "document":
		if m.MediaType != "application/pdf" {
			return sdk.ToolResultBlockParamContentUnion{}, fmt.Errorf("unsupported document media type: %q", m.MediaType)
		}
		doc := &sdk.DocumentBlockParam{
			Source: sdk.DocumentBlockParamSourceUnion{
				OfBase64: &sdk.Base64PDFSourceParam{Data: m.DataB64},
			},
		}
		if m.Name != "" {
			doc.Title = param.NewOpt(m.Name)
		}
		return sdk.ToolResultBlockParamContentUnion{OfDocument: doc}, nil
	default:
		return sdk.ToolResultBlockParamContentUnion{}, fmt.Errorf("unsupported media kind: %q", m.Kind)
	}
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
