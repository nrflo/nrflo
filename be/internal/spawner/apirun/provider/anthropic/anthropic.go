// Package anthropic implements provider.Provider on top of the official
// github.com/anthropics/anthropic-sdk-go SDK. Only streaming is supported —
// non-streaming has no callers and is intentionally not implemented.
//
// Cache breakpoints map onto Anthropic's 4-slot cache_control limit: a SYSTEM
// breakpoint marks the system block (which also caches the tool definitions,
// since tools render before system), a TOOLS breakpoint marks the last tool,
// and MESSAGE breakpoints slide along the conversation tail each turn. See
// cache.go for placement and lookback-window handling.
package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"

	"be/internal/spawner/apirun/provider"
)

// claudeCodeIdentitySystem is the system-prompt block Anthropic requires as
// the leading block when authenticating with an OAuth bearer token. Without
// it, premium models (sonnet/opus) return 429 rate_limit_error with no
// utilization headers; only haiku is exempt. Prepended in Run when
// authMethod==MethodOAuthBearer.
const claudeCodeIdentitySystem = "You are Claude Code, Anthropic's official CLI for Claude."

// New returns a provider.Provider backed by the Anthropic SDK using the given credentials.
// For MethodAPIKey, uses x-api-key auth. For MethodOAuthBearer, uses Authorization: Bearer
// with the oauth-2025-04-20 beta header. Optional opts are forwarded to the SDK client
// (used by tests to inject a fake http.Client).
func New(creds Credentials, opts ...option.RequestOption) provider.Provider {
	var authOpts []option.RequestOption
	switch creds.Method {
	case MethodOAuthBearer:
		authOpts = []option.RequestOption{
			option.WithAuthToken(creds.Value),
			option.WithHeaderAdd("anthropic-beta", "oauth-2025-04-20"),
		}
	default:
		authOpts = []option.RequestOption{option.WithAPIKey(creds.Value)}
	}
	all := append(authOpts, opts...)
	client := sdk.NewClient(all...)
	return &anthropicProvider{client: &client, authMethod: creds.Method}
}

// NewWithHTTPClient is a convenience constructor for tests: it injects the
// given *http.Client (fake transport) before applying credentials.
func NewWithHTTPClient(creds Credentials, hc *http.Client) provider.Provider {
	return New(creds, option.WithHTTPClient(hc))
}

type anthropicProvider struct {
	client     *sdk.Client
	authMethod AuthMethod
}

func (p *anthropicProvider) Name() string { return "anthropic" }

// MaxContext returns the model's input context window in tokens. The DB
// models.api_context is authoritative (see spawner_prepare); this is the
// fallback consulted only when a row carries no explicit value. Window values
// live in model.go (contextWindow); unknown models default to 200k.
func (p *anthropicProvider) MaxContext(model string) int {
	return contextWindow(model)
}

// Run executes a single streaming model turn. It blocks until the stream
// terminates (message_stop / error), invoking sink callbacks in stream order
// and returning the assembled FinalResponse.
func (p *anthropicProvider) Run(ctx context.Context, req provider.Request, sink provider.EventSink) (*provider.FinalResponse, error) {
	params, err := translateRequest(req)
	if err != nil {
		return nil, fmt.Errorf("translate anthropic request: %w", err)
	}
	if p.authMethod == MethodOAuthBearer {
		params.System = append(
			[]sdk.TextBlockParam{{Text: claudeCodeIdentitySystem}},
			params.System...,
		)
	}
	stream := p.client.Messages.NewStreaming(ctx, params)
	defer stream.Close()

	return decodeStream(stream, sink)
}

// blockAccumulator tracks per-index streaming state so we can assemble the
// FinalResponse from a sequence of deltas.
type blockAccumulator struct {
	kind      string // "text" | "tool_use" | "thinking" | "redacted_thinking"
	text      string
	toolID    string
	toolName  string
	toolInput string // accumulated partial JSON
	signature string // for thinking blocks
	data      string // for redacted_thinking blocks
}

// decodeStream drives a single Anthropic stream to completion, emitting sink
// callbacks and assembling the FinalResponse content/usage.
func decodeStream(stream *ssestream.Stream[sdk.MessageStreamEventUnion], sink provider.EventSink) (*provider.FinalResponse, error) {
	final := &provider.FinalResponse{}
	blocks := map[int64]*blockAccumulator{}

	for stream.Next() {
		ev := stream.Current()
		switch ev.Type {
		case "message_start":
			start := ev.AsMessageStart()
			final.Usage = mergeUsage(final.Usage, fromUsage(start.Message.Usage))

		case "content_block_start":
			cb := ev.ContentBlock
			acc := &blockAccumulator{kind: cb.Type}
			switch cb.Type {
			case "tool_use":
				acc.toolID = cb.ID
				acc.toolName = cb.Name
				sink.OnToolUseStart(cb.ID, cb.Name)
			case "text":
				// Initial text is usually empty, but keep any prefill.
				acc.text = cb.Text
			case "thinking":
				acc.text = cb.Thinking
				acc.signature = cb.Signature
			case "redacted_thinking":
				acc.data = cb.Data
			}
			blocks[ev.Index] = acc

		case "content_block_delta":
			acc, ok := blocks[ev.Index]
			if !ok {
				continue
			}
			d := ev.Delta
			switch d.Type {
			case "text_delta":
				acc.text += d.Text
				sink.OnTextDelta(d.Text)
			case "input_json_delta":
				acc.toolInput += d.PartialJSON
				sink.OnToolUseInputDelta(acc.toolID, d.PartialJSON)
			case "thinking_delta":
				acc.text += d.Thinking
				sink.OnThinkingDelta(d.Thinking)
			case "signature_delta":
				acc.signature += d.Signature
			}

		case "content_block_stop":
			acc, ok := blocks[ev.Index]
			if !ok {
				continue
			}
			block, err := finalizeBlock(acc, sink)
			if err != nil {
				return nil, err
			}
			final.Content = append(final.Content, block)
			delete(blocks, ev.Index)

		case "message_delta":
			md := ev.AsMessageDelta()
			if sr := string(md.Delta.StopReason); sr != "" {
				final.StopReason = sr
			}
			final.Usage = mergeDeltaUsage(final.Usage, md.Usage)

		case "message_stop":
			sink.OnUsage(final.Usage)
			return final, nil
		}
	}

	if err := stream.Err(); err != nil {
		return nil, err
	}
	// Stream ended without explicit message_stop — still report usage so the
	// runner can record what little we know.
	sink.OnUsage(final.Usage)
	return final, nil
}

func finalizeBlock(acc *blockAccumulator, sink provider.EventSink) (provider.ContentBlock, error) {
	switch acc.kind {
	case "tool_use":
		raw := acc.toolInput
		if raw == "" {
			raw = "{}"
		}
		// Validate that the accumulated fragments form valid JSON; surface a
		// clear error rather than passing malformed bytes downstream.
		if !json.Valid([]byte(raw)) {
			return provider.ContentBlock{}, fmt.Errorf("tool_use %s: invalid JSON input %q", acc.toolID, raw)
		}
		input := json.RawMessage(raw)
		sink.OnToolUseStop(acc.toolID, input)
		return provider.ContentBlock{
			Type:      "tool_use",
			ToolUseID: acc.toolID,
			ToolName:  acc.toolName,
			Input:     input,
		}, nil
	case "text":
		return provider.ContentBlock{Type: "text", Text: acc.text}, nil
	case "thinking":
		return provider.ContentBlock{Type: "thinking", Text: acc.text, Signature: acc.signature}, nil
	case "redacted_thinking":
		return provider.ContentBlock{Type: "redacted_thinking", Data: acc.data}, nil
	}
	return provider.ContentBlock{}, fmt.Errorf("unknown content block kind: %q", acc.kind)
}

func fromUsage(u sdk.Usage) provider.Usage {
	return provider.Usage{
		InputTokens:         int(u.InputTokens),
		OutputTokens:        int(u.OutputTokens),
		CacheReadTokens:     int(u.CacheReadInputTokens),
		CacheCreationTokens: int(u.CacheCreationInputTokens),
	}
}

func mergeUsage(a, b provider.Usage) provider.Usage {
	if b.InputTokens > 0 {
		a.InputTokens = b.InputTokens
	}
	if b.OutputTokens > 0 {
		a.OutputTokens = b.OutputTokens
	}
	if b.CacheReadTokens > 0 {
		a.CacheReadTokens = b.CacheReadTokens
	}
	if b.CacheCreationTokens > 0 {
		a.CacheCreationTokens = b.CacheCreationTokens
	}
	return a
}

func mergeDeltaUsage(a provider.Usage, d sdk.MessageDeltaUsage) provider.Usage {
	if d.InputTokens > 0 {
		a.InputTokens = int(d.InputTokens)
	}
	if d.OutputTokens > 0 {
		a.OutputTokens = int(d.OutputTokens)
	}
	if d.CacheReadInputTokens > 0 {
		a.CacheReadTokens = int(d.CacheReadInputTokens)
	}
	if d.CacheCreationInputTokens > 0 {
		a.CacheCreationTokens = int(d.CacheCreationInputTokens)
	}
	return a
}
