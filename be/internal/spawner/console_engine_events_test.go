package spawner

import (
	"strings"
	"testing"
	"time"
)

// TestCodexEngine_FullTurnFixture_DeltasAndCanonicalText replays the captured
// real turn (testdata/codex_appserver/full_turn.jsonl) through the engine and
// asserts the "user turn -> deltas -> completed text persisted" acceptance
// criterion against real data.
//
// The fixture's 28 item/agentMessage/delta frames concatenate to one
// continuous string, but item/completed fires TWO separate agentMessage items
// (verified against the fixture directly: msg_055ea9... "I'll say a brief
// hello... completion." and msg_0c4ae5... "Hello. All done." are distinct
// itemIds) — not one, so the Sink records exactly two category="text" rows
// whose concatenation equals the deltas' concatenation, not one row.
func TestCodexEngine_FullTurnFixture_DeltasAndCanonicalText(t *testing.T) {
	sink := &testSink{}
	eng, f := startTestCodexEngine(t, sink, EngineSpec{})

	for _, line := range rawFixtureLines(t, "full_turn.jsonl") {
		f.feed(line)
	}

	events := collectEventsUntil(t, eng.Events(), func(ev EngineEvent) bool {
		return ev.Type == EventTurnCompleted
	}, 2*time.Second)

	var deltas, texts []string
	var toolInvokes, toolResults, tokenUsages int
	var sawTurnStarted, sawTurnCompleted bool
	for _, ev := range events {
		switch ev.Type {
		case EventTextDelta:
			deltas = append(deltas, ev.Text)
		case EventText:
			texts = append(texts, ev.Text)
		case EventToolInvoke:
			toolInvokes++
		case EventToolResult:
			toolResults++
		case EventTokenUsage:
			tokenUsages++
		case EventTurnStarted:
			sawTurnStarted = true
		case EventTurnCompleted:
			sawTurnCompleted = true
		case EventError:
			t.Errorf("unexpected EventError on a clean fixture turn: %+v", ev)
		}
	}

	if len(deltas) != 28 {
		t.Errorf("EventTextDelta count = %d, want 28", len(deltas))
	}
	joinedDeltas := strings.Join(deltas, "")

	if len(texts) != 2 {
		t.Fatalf("EventText count = %d, want 2: %v", len(texts), texts)
	}
	if joined := strings.Join(texts, ""); joined != joinedDeltas {
		t.Errorf("joined EventText = %q, want it to equal joined deltas %q", joined, joinedDeltas)
	}

	if toolInvokes != 1 || toolResults != 1 {
		t.Errorf("tool invoke/result events = %d/%d, want 1/1", toolInvokes, toolResults)
	}
	if tokenUsages != 2 {
		t.Errorf("EventTokenUsage count = %d, want 2", tokenUsages)
	}
	if !sawTurnStarted || !sawTurnCompleted {
		t.Errorf("turn lifecycle events missing: started=%v completed=%v", sawTurnStarted, sawTurnCompleted)
	}

	if n := countCategory(sink, "text"); n != 2 {
		t.Errorf("Sink text rows = %d, want exactly 2", n)
	}
	sink.mu.Lock()
	var sinkTexts []string
	var toolRows []string
	for _, m := range sink.recordedMsgs {
		if m.category == "text" {
			sinkTexts = append(sinkTexts, m.content)
		}
		if m.category == "tool" {
			toolRows = append(toolRows, m.content)
		}
	}
	sink.mu.Unlock()
	if joined := strings.Join(sinkTexts, ""); joined != joinedDeltas {
		t.Errorf("Sink recorded text joined = %q, want %q", joined, joinedDeltas)
	}
	if len(toolRows) == 0 || !strings.Contains(toolRows[0], "echo hi") || !strings.Contains(toolRows[0], "exit 0") {
		t.Errorf("Sink tool row missing/mismatched: %v", toolRows)
	}
}

// TestCodexEngine_ThinkingDeltas asserts item/reasoning/textDelta and
// item/reasoning/summaryTextDelta both yield EventThinking (live-only, no
// Sink row — matches the delta contract for text deltas).
func TestCodexEngine_ThinkingDeltas(t *testing.T) {
	sink := &testSink{}
	eng, f := startTestCodexEngine(t, sink, EngineSpec{})

	f.feed(`{"method":"item/reasoning/textDelta","params":{"itemId":"r1","delta":"pondering"}}`)
	ev := waitForEventType(t, eng.Events(), EventThinking, 2*time.Second)
	if ev.Text != "pondering" {
		t.Errorf("textDelta event = %+v, want text=pondering", ev)
	}

	f.feed(`{"method":"item/reasoning/summaryTextDelta","params":{"itemId":"r1","delta":"summary bit"}}`)
	ev = waitForEventType(t, eng.Events(), EventThinking, 2*time.Second)
	if ev.Text != "summary bit" {
		t.Errorf("summaryTextDelta event = %+v, want text=%q", ev, "summary bit")
	}

	if n := len(sink.recordedMsgs); n != 0 {
		t.Errorf("thinking deltas must not persist, got %d Sink rows", n)
	}
}

// TestCodexEngine_CompletedReasoning_ArrayShape asserts the REAL wire shape of
// a completed reasoning item: ReasoningThreadItem.content/.summary are arrays
// of plain strings in both codex protocol generations (`codex app-server
// generate-json-schema`, codex-cli 0.144.1), never a bare string and never a
// {type,text} block. content wins over summary, blocks join with newlines, and
// `text` (an agentMessage field) is never used.
func TestCodexEngine_CompletedReasoning_ArrayShape(t *testing.T) {
	sink := &testSink{}
	eng, f := startTestCodexEngine(t, sink, EngineSpec{})

	f.feed(`{"method":"item/completed","params":{"item":{"type":"reasoning","id":"r1","summary":["ignored"],"content":["first","second"],"text":"must not be used"}}}`)
	ev := waitForEventType(t, eng.Events(), EventThinking, 2*time.Second)
	if ev.Text != "first\nsecond" {
		t.Errorf("completed reasoning event = %+v, want text=%q (joined content)", ev, "first\nsecond")
	}

	f.feed(`{"method":"item/completed","params":{"item":{"type":"reasoning","id":"r2","summary":["summary only"],"content":[]}}}`)
	ev = waitForEventType(t, eng.Events(), EventThinking, 2*time.Second)
	if ev.Text != "summary only" {
		t.Errorf("completed reasoning fallback = %+v, want text=%q (summary when content empty)", ev, "summary only")
	}

	if n := len(sink.recordedMsgs); n != 0 {
		t.Errorf("completed reasoning must not persist, got %d Sink rows", n)
	}
}

// TestCodexEngine_CompletedReasoning_EmptyArrays asserts the shape codex
// actually emits when reasoning summaries are off (testdata/codex_appserver/
// full_turn.jsonl carries content:[] summary:[]): no EventThinking, and — the
// part that used to break — the empty arrays must not make json.Unmarshal fail
// and take the whole item down with it.
func TestCodexEngine_CompletedReasoning_EmptyArrays(t *testing.T) {
	sink := &testSink{}
	eng, f := startTestCodexEngine(t, sink, EngineSpec{})

	f.feed(`{"method":"item/completed","params":{"item":{"type":"reasoning","id":"r1","summary":[],"content":[]}}}`)
	// A following agentMessage proves the dispatcher survived the reasoning item.
	f.feed(`{"method":"item/completed","params":{"item":{"type":"agentMessage","id":"a1","text":"after"}}}`)

	ev := waitForEventType(t, eng.Events(), EventText, 2*time.Second)
	if ev.Text != "after" {
		t.Errorf("agentMessage after empty reasoning = %+v, want text=%q", ev, "after")
	}
}

// TestCodexEngine_Tools_CommandExecution asserts a completed commandExecution
// item yields EventToolInvoke + EventToolResult plus the existing "[Bash] …"
// Sink tool row.
func TestCodexEngine_Tools_CommandExecution(t *testing.T) {
	sink := &testSink{}
	eng, f := startTestCodexEngine(t, sink, EngineSpec{})

	f.feed(`{"method":"item/completed","params":{"item":{"type":"commandExecution","id":"c1","command":"echo hi","aggregatedOutput":"hi\n","exitCode":0}}}`)

	invoke := waitForEventType(t, eng.Events(), EventToolInvoke, 2*time.Second)
	if invoke.ToolName != "Bash" || invoke.ToolInput["command"] != "echo hi" {
		t.Errorf("invoke event = %+v, want ToolName=Bash command=echo hi", invoke)
	}
	result := waitForEventType(t, eng.Events(), EventToolResult, 2*time.Second)
	if result.ToolName != "Bash" || result.IsError {
		t.Errorf("result event = %+v, want ToolName=Bash isError=false", result)
	}

	sink.mu.Lock()
	var toolRows []string
	for _, m := range sink.recordedMsgs {
		if m.category == "tool" {
			toolRows = append(toolRows, m.content)
		}
	}
	sink.mu.Unlock()
	if len(toolRows) != 1 || !strings.Contains(toolRows[0], "[Bash]") || !strings.Contains(toolRows[0], "echo hi") {
		t.Errorf("Sink tool rows = %v, want one [Bash] echo hi row", toolRows)
	}
}

// TestCodexEngine_Tools_McpToolCall asserts a completed mcpToolCall item
// yields invoke + result events with the name normalized to
// mcp__<server>__<tool> (emitMcpToolCall, codex_appserver_events.go:121).
func TestCodexEngine_Tools_McpToolCall(t *testing.T) {
	sink := &testSink{}
	eng, f := startTestCodexEngine(t, sink, EngineSpec{})

	f.feed(`{"method":"item/completed","params":{"item":{"type":"mcpToolCall","id":"i1","server":"nrflo","tool":"emit_findings","arguments":{"key":"summary"},"result":{"content":[{"type":"text","text":"ok: 1 finding"}]}}}}`)

	invoke := waitForEventType(t, eng.Events(), EventToolInvoke, 2*time.Second)
	if invoke.ToolName != "mcp__nrflo__emit_findings" {
		t.Errorf("invoke event ToolName = %q, want mcp__nrflo__emit_findings", invoke.ToolName)
	}
	result := waitForEventType(t, eng.Events(), EventToolResult, 2*time.Second)
	if result.ToolName != "mcp__nrflo__emit_findings" || result.Text != "ok: 1 finding" || result.IsError {
		t.Errorf("result event = %+v, want ok text, isError=false", result)
	}
}

// TestCodexEngine_TokenUsage asserts thread/tokenUsage/updated yields
// EventTokenUsage with the ComputeContextLeftPct-derived percentage and also
// updates the Sink.
func TestCodexEngine_TokenUsage(t *testing.T) {
	sink := &testSink{}
	eng, f := startTestCodexEngine(t, sink, EngineSpec{})

	f.feed(`{"method":"thread/tokenUsage/updated","params":{"tokenUsage":{"total":{"inputTokens":27279},"last":{"inputTokens":9115},"modelContextWindow":258400}}}`)

	ev := waitForEventType(t, eng.Events(), EventTokenUsage, 2*time.Second)
	want := ComputeContextLeftPct(9115, 258400)
	if ev.ContextLeftPct != want {
		t.Errorf("EventTokenUsage pct = %d, want %d (from last, not cumulative total)", ev.ContextLeftPct, want)
	}
	if len(sink.contextUpdates) != 1 || sink.contextUpdates[0] != want {
		t.Errorf("Sink.contextUpdates = %v, want [%d]", sink.contextUpdates, want)
	}
	if ev.Usage == nil {
		t.Fatal("EventTokenUsage.Usage = nil, want populated from thread/tokenUsage/updated `last`")
	}
	if ev.Usage.InputTokens != 9115 {
		t.Errorf("Usage.InputTokens = %d, want 9115 (last.inputTokens, not total's 27279)", ev.Usage.InputTokens)
	}
	if ev.Usage.ContextWindow != 258400 {
		t.Errorf("Usage.ContextWindow = %d, want 258400", ev.Usage.ContextWindow)
	}
}

// TestCodexEngine_StopUnblocksFullEventBuffer guards the deadlock a blocking
// emit used to cause: a consumer that stops draining Events() (the natural
// `for range Events() { … Stop() }` shape) parks runLoop in a send on the full
// 256-event buffer, so it never re-enters its select to see ctx.Done, never
// closes loopDone, and Stop() — which waits on loopDone — hangs forever.
func TestCodexEngine_StopUnblocksFullEventBuffer(t *testing.T) {
	eng, f := startTestCodexEngine(t, &testSink{}, EngineSpec{})

	// Overfill the buffer without ever reading Events().
	for i := 0; i < cap(eng.events)+50; i++ {
		f.feed(`{"method":"item/agentMessage/delta","params":{"itemId":"m1","delta":"x"}}`)
	}

	stopped := make(chan struct{})
	go func() { eng.Stop(); close(stopped) }()

	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() deadlocked with a full events buffer and no consumer draining")
	}
}
