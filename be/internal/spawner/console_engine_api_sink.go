package spawner

import (
	"encoding/json"
	"sync"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
	"be/internal/spawner/apirun"
)

// apiEngineSink adapts spawner.Sink to apirun.MessageSink for the api console
// engine — no processInfo pending-buffer exists here, so every call persists
// straight through the sink (emitMessage/emitMessageWithPayload), the same
// path a human console session's tool cards render from.
type apiEngineSink struct {
	sessionID string
	sink      Sink
	pool      *db.Pool
	clock     clock.Clock
}

func (s *apiEngineSink) TrackMessage(content, category string) {
	emitMessage(s.sessionID, content, category, s.sink)
}

// TrackToolInvoke records a tool-invoke row carrying tool_use_id and the raw
// tool input in its payload — the exact shape output_tool_span.go writes for
// autonomous agents — so the FE's tool-card pairing (chatStream.ts) works
// unchanged.
func (s *apiEngineSink) TrackToolInvoke(content, category, toolUseID string, rawInput []byte) {
	payload := BuildToolInvokePayload(toolUseID, rawInput)
	emitMessageWithPayload(s.sessionID, content, category, payload, s.sink)
}

// CloseToolSpan stamps ended_at on the invoke row directly in the DB — unlike
// the autonomous path there is no in-memory pending buffer to check first.
func (s *apiEngineSink) CloseToolSpan(toolUseID string) {
	if toolUseID == "" || s.pool == nil {
		return
	}
	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
	_, _ = repo.NewAgentMessageRepo(s.pool, s.clock).SetToolEnded(s.sessionID, toolUseID, now)
}

// apiEngineProcState adapts the api console engine to apirun.ProcState.
// SetFinalStatus/SetCallbackLevel are record-only: a chat session has no
// processInfo and no workflow to fail, so there is nothing to act on beyond
// storing the value the runner set. SetContextLeft is the one field that
// drives an observable effect — it emits EventTokenUsage.
type apiEngineProcState struct {
	e *apiConsoleEngine
}

func (p *apiEngineProcState) SessionID() string          { return p.e.spec.SessionID }
func (p *apiEngineProcState) ProjectID() string          { return p.e.spec.ProjectID }
func (p *apiEngineProcState) WorkflowInstanceID() string { return "" }

func (p *apiEngineProcState) SetFinalStatus(status string) {
	p.e.mu.Lock()
	p.e.lastTurnStatus = status
	p.e.mu.Unlock()
}

func (p *apiEngineProcState) SetContextLeft(pct int) {
	p.e.emit(EngineEvent{Type: EventTokenUsage, SessionID: p.e.spec.SessionID, ContextLeftPct: pct})
}

func (p *apiEngineProcState) SetCallbackLevel(level int) {
	p.e.mu.Lock()
	p.e.lastCallbackLevel = level
	p.e.mu.Unlock()
}

// SetProviderHardFail is a no-op: console chat engines hold no processInfo
// and never advance a tier fallback chain (there is no monitorAll for them).
func (p *apiEngineProcState) SetProviderHardFail() {}

// apiEngineStream adapts the api console engine to apirun.StreamHook: raw
// text/thinking deltas stream to the console immediately, ahead of the
// runner sink's ~4KB buffered persistence. ItemID carries the sink's
// per-segment id (one id per persisted row), which is what the FE keys its
// live delta buffer by — codex gets the same guarantee from the app-server's
// own item ids (codex_appserver_events_engine.go). Tool span hooks map onto
// EventToolInvoke/EventToolResult so a live console can show the in-flight
// tool; toolNames pairs the end hook (id-only) back to its name.
type apiEngineStream struct {
	e         *apiConsoleEngine
	mu        sync.Mutex
	toolNames map[string]string
}

func (s *apiEngineStream) OnTextDelta(itemID, text string) {
	s.e.emit(EngineEvent{Type: EventTextDelta, SessionID: s.e.spec.SessionID, ItemID: itemID, Text: text})
}

func (s *apiEngineStream) OnThinkingDelta(itemID, text string) {
	s.e.emit(EngineEvent{Type: EventThinking, SessionID: s.e.spec.SessionID, ItemID: itemID, Text: text})
}

func (s *apiEngineStream) OnToolStart(toolUseID, name string, input json.RawMessage) {
	s.mu.Lock()
	if s.toolNames == nil {
		s.toolNames = map[string]string{}
	}
	s.toolNames[toolUseID] = name
	s.mu.Unlock()
	var parsed map[string]any
	_ = json.Unmarshal(input, &parsed)
	s.e.emit(EngineEvent{Type: EventToolInvoke, SessionID: s.e.spec.SessionID, ToolName: name, ToolInput: parsed})
}

func (s *apiEngineStream) OnToolEnd(toolUseID string, isError bool) {
	s.mu.Lock()
	name := s.toolNames[toolUseID]
	delete(s.toolNames, toolUseID)
	s.mu.Unlock()
	s.e.emit(EngineEvent{Type: EventToolResult, SessionID: s.e.spec.SessionID, ToolName: name, IsError: isError})
}

var _ apirun.MessageSink = (*apiEngineSink)(nil)
var _ apirun.ProcState = (*apiEngineProcState)(nil)
var _ apirun.StreamHook = (*apiEngineStream)(nil)
