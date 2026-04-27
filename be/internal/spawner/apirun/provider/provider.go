// Package provider defines a provider-agnostic abstraction for executing model
// turns in API mode. Concrete implementations (anthropic, openai, bedrock, ...)
// translate between the SDK-specific event/message shapes and these neutral
// types so the runner (T3) and tool dispatcher (T4) stay provider-free.
package provider

import (
	"context"
	"encoding/json"
)

// Provider is the contract every API-mode adapter implements.
type Provider interface {
	Name() string
	MaxContext(model string) int
	Run(ctx context.Context, req Request, sink EventSink) (*FinalResponse, error)
}

// Request is one provider-neutral model turn.
type Request struct {
	System           string
	Messages         []Message
	Tools            []ToolSpec
	MaxTokens        int
	ToolChoice       string // "auto" for v1
	CacheBreakpoints []CacheBreakpoint
	Model            string
}

// Message is a single role-tagged entry in the conversation history.
type Message struct {
	Role    string // "user" | "assistant"
	Content []ContentBlock
}

// ContentBlock is a single block within a Message. Type determines which
// fields are populated.
type ContentBlock struct {
	Type      string // "text" | "tool_use" | "tool_result"
	Text      string
	ToolUseID string
	ToolName  string
	Input     json.RawMessage
	Output    string
	IsError   bool
}

// ToolSpec describes a single tool the model may invoke.
type ToolSpec struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

// CacheBreakpointTarget identifies which logical request region a cache
// breakpoint applies to.
type CacheBreakpointTarget string

const (
	CacheTargetSystem  CacheBreakpointTarget = "system"
	CacheTargetTools   CacheBreakpointTarget = "tools"
	CacheTargetMessage CacheBreakpointTarget = "message"
)

// CacheBreakpoint is a hint to the provider about where to attach a cache
// marker. Anthropic supports at most two breakpoints today; providers without
// caching may ignore this.
type CacheBreakpoint struct {
	Target CacheBreakpointTarget
}

// EventSink receives streaming callbacks from a provider while a turn is in
// flight. All callbacks must be invoked from the same goroutine that called
// Provider.Run (no locking required by the implementer).
type EventSink interface {
	OnTextDelta(text string)
	OnToolUseStart(id, name string)
	OnToolUseInputDelta(id, partialJSON string)
	OnToolUseStop(id string, fullInput json.RawMessage)
	OnUsage(u Usage) // delivered once at the end of the turn
}

// FinalResponse is the assembled result of a single Run() call. Content is the
// complete assistant message reconstructed from the streamed deltas — callers
// pass it back as the next turn's assistant Message.
type FinalResponse struct {
	StopReason string // "end_turn" | "tool_use" | "max_tokens" | "stop_sequence"
	Content    []ContentBlock
	Usage      Usage
}

// Usage is the token accounting reported by the provider for one turn.
type Usage struct {
	InputTokens         int
	OutputTokens        int
	CacheReadTokens     int
	CacheCreationTokens int
}
