// Package apirun implements the in-process tool-use loop that drives an
// API-mode agent through one or more turns. The runner is provider-agnostic:
// it consumes a provider.Provider for streaming, and reports messages /
// status back to the spawner via the small interfaces declared here.
//
// The package deliberately does NOT import the spawner package — the spawner
// supplies adapter values that satisfy these interfaces, avoiding the cycle
// (spawner -> apirun -> spawner).
package apirun

import "be/internal/spawner/apirun/provider"

// MessageSink receives streaming events from the runner. The sink is bound
// to a single agent process (the spawner adapter captures *processInfo);
// callers do not pass the process again per call.
type MessageSink interface {
	TrackMessage(content, category string)
	// TrackToolInvoke records a tool-invoke row carrying tool_use_id and the
	// raw tool input in its payload, so the trace timeline can pair it with
	// the result and the UI tool card can render the full invocation.
	TrackToolInvoke(content, category, toolUseID string, rawInput []byte)
	// CloseToolSpan stamps ended_at on the invoke row once the tool returns.
	CloseToolSpan(toolUseID string)
}

// ProcState is the small mutator surface the runner needs on the agent
// process. The spawner supplies an adapter wrapping *processInfo. The runner
// reads SessionID/ProjectID for AgentSvc and ErrorRecorder calls, and writes
// FinalStatus and ContextLeft so monitorAll observes them through the same
// fields the CLI backend uses.
type ProcState interface {
	SessionID() string
	ProjectID() string
	WorkflowInstanceID() string
	SetFinalStatus(string)
	SetContextLeft(int)
	SetCallbackLevel(int)
	// SetProviderHardFail flags a HARD (non-rate-limit) provider error so the
	// spawner's tier-fallback engine can advance to the next chain entry on
	// relaunch. Never called for RetryClassRateLimit — that stays in-band.
	SetProviderHardFail()
}

// AgentSvc persists context_left and broadcasts the corresponding WS event.
// In production this is service.AgentService.UpdateContextLeft.
type AgentSvc interface {
	UpdateContextLeft(sessionID string, pct int) (projectID, ticketID, workflowName string, err error)
}

// ErrorRecorder mirrors spawner.ErrorRecorder so the runner can record
// agent-level errors (auth, network, protocol) without depending on the
// spawner package.
type ErrorRecorder interface {
	RecordError(projectID, errorType, instanceID, message string) error
}

// LedgerObserver receives every content block the runner appends to the
// conversation and each turn's provider usage, so the spawner's external
// context ledger can track context blocks EXACTLY as they enter the
// conversation without apirun importing spawner. Nil-safe: Config.Observer
// is optional and every call site guards it, mirroring EventEmitter.emit.
type LedgerObserver interface {
	// OnMessage reports blocks newly appended under role ("user" | "assistant").
	// Callers pass only new appends, never pre-existing history, so a
	// Conversation's replayed turns are not double-counted.
	OnMessage(role string, blocks []provider.ContentBlock)
	// OnUsage reports one turn's provider-side token accounting.
	OnUsage(u provider.Usage)
}

// WatcherState summarizes the live conversation for a ContextWatcher policy
// decision: MessageCount is len(msgs) at the consult point, PctLeft/PctKnown
// mirror the last turn's reported context-left percentage (PctKnown is false
// before any turn has reported usage, or right after a compaction reset it).
type WatcherState struct {
	MessageCount int
	PctLeft      int
	PctKnown     bool
}

// CompactionPlan is a selective-GC decision returned by ContextWatcher.PlanGC:
// the applier keeps msgs[:KeepPrefixMsgs] and msgs[len(msgs)-KeepSuffixMsgs:]
// byte-identical and replaces everything between them with a single digest
// message carrying ReferenceDigest — pointers back to the evicted content.
type CompactionPlan struct {
	KeepPrefixMsgs  int
	KeepSuffixMsgs  int
	ReferenceDigest string
	PolicyName      string
	TokensEvicted   int
}

// ContextWatcher is a policy engine consulted at the runner's compaction
// checkpoints (mid-loop and pre-turn) to decide whether a selective GC should
// run right now, in place of the uniform maybeCompactInLoop/maybeCompact
// fallback. Nil-safe: Config.Watcher is optional and every call site guards
// it, mirroring LedgerObserver — apirun must not import spawner, so the
// spawner supplies an adapter over its context ledger + budget/idle/throttle
// policy.
type ContextWatcher interface {
	// PlanGC returns (plan, true) when a GC should run now; (zero, false)
	// tells the caller to fall back to its own uniform compaction check.
	PlanGC(state WatcherState) (CompactionPlan, bool)
}
