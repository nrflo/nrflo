package spawner

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// newTranscriptTestEngine builds a bare claudeEngine (no PTY, no Start) wired
// only for flushTranscript: tests call the tailer's tick directly instead of
// waiting on tailLoop's ticker or a real Claude CLI transcript.
func newTranscriptTestEngine(sink Sink, cfgDir, workDir, sessionID string) *claudeEngine {
	e := &claudeEngine{
		sink:     sink,
		events:   make(chan EngineEvent, 256),
		stopping: make(chan struct{}),
	}
	e.spec = EngineSpec{SessionID: sessionID, WorkDir: workDir, Env: []string{"CLAUDE_CONFIG_DIR=" + cfgDir}}
	return e
}

// writeRawTranscript writes exact bytes (no implicit trailing newline), so
// tests can construct partial trailing lines deliberately.
func writeRawTranscript(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir transcript dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
}

// drainEvents collects every event currently buffered on e.events without
// blocking.
func drainEvents(e *claudeEngine) []EngineEvent {
	var out []EngineEvent
	for {
		select {
		case ev := <-e.events:
			out = append(out, ev)
		default:
			return out
		}
	}
}

func TestClaudeEngine_FlushTranscript_TextBlocksInOrder(t *testing.T) {
	cfg, workDir, sid := t.TempDir(), "/work/console-order", "sess-tail-order"
	sink := &testSink{}
	e := newTranscriptTestEngine(sink, cfg, workDir, sid)
	path := claudeTranscriptPath(e.spec.Env, workDir, sid)

	line1 := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"first"}]}}` + "\n"
	line2 := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"second"}]}}` + "\n"
	writeRawTranscript(t, path, line1+line2)

	e.flushTranscript()
	var texts []string
	for _, ev := range drainEvents(e) {
		if ev.Type == EventText {
			texts = append(texts, ev.Text)
		}
	}
	if len(texts) != 2 || texts[0] != "first" || texts[1] != "second" {
		t.Errorf("texts = %v, want [first second] in order", texts)
	}
	if n := countCategory(sink, "text"); n != 2 {
		t.Errorf("text rows = %d, want 2 (one per assistant text block)", n)
	}
}

func TestClaudeEngine_FlushTranscript_ThinkingEmitsEventOnlyNoRow(t *testing.T) {
	cfg, workDir, sid := t.TempDir(), "/work/console-think", "sess-tail-think"
	sink := &testSink{}
	e := newTranscriptTestEngine(sink, cfg, workDir, sid)
	path := claudeTranscriptPath(e.spec.Env, workDir, sid)
	writeRawTranscript(t, path, `{"type":"assistant","message":{"role":"assistant","content":[{"type":"thinking","thinking":"pondering"}]}}`+"\n")

	e.flushTranscript()
	events := drainEvents(e)
	var found bool
	for _, ev := range events {
		if ev.Type == EventThinking {
			found = true
			if ev.Text != "pondering" {
				t.Errorf("EventThinking.Text = %q, want %q", ev.Text, "pondering")
			}
		}
	}
	if !found {
		t.Fatalf("expected an EventThinking, got %+v", events)
	}
	if n := len(sink.recordedMsgs); n != 0 {
		t.Errorf("thinking blocks must not persist a Sink row (socket's tailThinking owns that), got %d rows", n)
	}
}

func TestClaudeEngine_FlushTranscript_ToolUseSkipped(t *testing.T) {
	cfg, workDir, sid := t.TempDir(), "/work/console-tool", "sess-tail-tool"
	sink := &testSink{}
	e := newTranscriptTestEngine(sink, cfg, workDir, sid)
	path := claudeTranscriptPath(e.spec.Env, workDir, sid)
	writeRawTranscript(t, path, `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash"}]}}`+"\n")

	e.flushTranscript()
	if events := drainEvents(e); len(events) != 0 {
		t.Errorf("tool_use block should emit no events (surfaces via PreToolUse hook instead), got %+v", events)
	}
	if n := len(sink.recordedMsgs); n != 0 {
		t.Errorf("tool_use block must not persist a Sink row, got %d", n)
	}
}

func TestClaudeEngine_FlushTranscript_BareStringContent(t *testing.T) {
	cfg, workDir, sid := t.TempDir(), "/work/console-bare", "sess-tail-bare"
	sink := &testSink{}
	e := newTranscriptTestEngine(sink, cfg, workDir, sid)
	path := claudeTranscriptPath(e.spec.Env, workDir, sid)
	writeRawTranscript(t, path, `{"type":"assistant","message":{"role":"assistant","content":"plain string reply"}}`+"\n")

	e.flushTranscript()
	events := drainEvents(e)
	if len(events) != 1 || events[0].Type != EventText || events[0].Text != "plain string reply" {
		t.Errorf("events = %+v, want one EventText %q", events, "plain string reply")
	}
}

func TestClaudeEngine_FlushTranscript_PartialLineNotConsumedUntilNewline(t *testing.T) {
	cfg, workDir, sid := t.TempDir(), "/work/console-partial", "sess-tail-partial"
	sink := &testSink{}
	e := newTranscriptTestEngine(sink, cfg, workDir, sid)
	path := claudeTranscriptPath(e.spec.Env, workDir, sid)

	complete := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"done line"}]}}` + "\n"
	partial := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"unfinished`
	writeRawTranscript(t, path, complete+partial)

	e.flushTranscript()
	events := drainEvents(e)
	if len(events) != 1 || events[0].Text != "done line" {
		t.Fatalf("first flush events = %+v, want exactly the complete line", events)
	}
	if e.transcriptOffset != int64(len(complete)) {
		t.Errorf("offset = %d, want %d (partial trailing line left unconsumed)", e.transcriptOffset, len(complete))
	}

	// Complete the trailing line and flush again — it must now surface.
	full := complete + partial + `"}]}}` + "\n"
	writeRawTranscript(t, path, full)
	e.flushTranscript()
	events2 := drainEvents(e)
	if len(events2) != 1 || events2[0].Text != "unfinished" {
		t.Fatalf("second flush events = %+v, want the now-completed line", events2)
	}
}

func TestClaudeEngine_FlushTranscript_RotatedFileRestartsAtZero(t *testing.T) {
	cfg, workDir, sid := t.TempDir(), "/work/console-rotate", "sess-tail-rotate"
	sink := &testSink{}
	e := newTranscriptTestEngine(sink, cfg, workDir, sid)
	path := claudeTranscriptPath(e.spec.Env, workDir, sid)

	line1 := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"before rotation, a much longer line of content"}]}}` + "\n"
	writeRawTranscript(t, path, line1)
	e.flushTranscript()
	_ = drainEvents(e)
	if e.transcriptOffset == 0 {
		t.Fatal("offset should have advanced past the first flush")
	}

	// Rotate: replace with shorter content — offset now exceeds the new size.
	rotated := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"after rotation"}]}}` + "\n"
	if len(rotated) >= len(line1) {
		t.Fatalf("test fixture invariant broken: rotated content must be shorter than line1")
	}
	writeRawTranscript(t, path, rotated)

	e.flushTranscript()
	events := drainEvents(e)
	if len(events) != 1 || events[0].Text != "after rotation" {
		t.Fatalf("post-rotation events = %+v, want the rotated content reprocessed from offset 0", events)
	}
}

func TestClaudeEngine_FlushTranscript_MissingFileNoOp(t *testing.T) {
	sink := &testSink{}
	e := newTranscriptTestEngine(sink, t.TempDir(), "/work/console-missing", "sess-tail-missing")

	e.flushTranscript() // must not panic
	if events := drainEvents(e); len(events) != 0 {
		t.Errorf("missing transcript file should emit nothing, got %+v", events)
	}
	if len(sink.recordedMsgs) != 0 {
		t.Errorf("missing transcript file should record nothing")
	}
}

// TestClaudeEngine_FlushTranscript_ConcurrentFlushesNoDuplicates guards the
// serialization: flushes are driven by the tail ticker AND by the socket
// goroutines carrying the Stop / PreToolUse hooks. Unserialized, two
// overlapping flushes both start from the same offset, so every new assistant
// line is emitted twice and persisted twice as a 'text' row.
func TestClaudeEngine_FlushTranscript_ConcurrentFlushesNoDuplicates(t *testing.T) {
	cfg, workDir, sid := t.TempDir(), "/work/console-concurrent", "sess-tail-concurrent"
	sink := &testSink{}
	e := newTranscriptTestEngine(sink, cfg, workDir, sid)
	path := claudeTranscriptPath(e.spec.Env, workDir, sid)

	const lines = 20
	var content string
	for i := 0; i < lines; i++ {
		content += `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"chunk"}]}}` + "\n"
	}
	writeRawTranscript(t, path, content)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.flushTranscript()
		}()
	}
	wg.Wait()

	var texts int
	for _, ev := range drainEvents(e) {
		if ev.Type == EventText {
			texts++
		}
	}
	if texts != lines {
		t.Errorf("EventText count = %d, want %d (each transcript line emitted exactly once)", texts, lines)
	}
	if n := countCategory(sink, "text"); n != lines {
		t.Errorf("text rows = %d, want %d (each transcript line persisted exactly once)", n, lines)
	}
}
