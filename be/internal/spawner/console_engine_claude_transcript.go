package spawner

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"time"
)

const (
	claudeTranscriptTailMaxBytes = 8 << 20   // 8 MiB defensive cap per flush
	claudeTranscriptTailMaxLines = 5000      // line cap per flush
	claudeTranscriptBodyCap      = 256 << 10 // per-row truncate
	claudeTranscriptTailInterval = 750 * time.Millisecond
)

// tailLoop periodically flushes the transcript so text/thinking events surface
// even between hook-triggered flushes (Stop, PreToolUse). Exits on ctx
// cancellation or Stop.
func (e *claudeEngine) tailLoop(ctx context.Context) {
	defer e.tailOnce.Do(func() { close(e.tailDone) })
	interval := e.tailInterval
	if interval <= 0 {
		interval = claudeTranscriptTailInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stopping:
			return
		case <-ticker.C:
			e.flushTranscript()
		}
	}
}

// flushTranscript reads any new complete lines from the session transcript
// since the last stored offset (byte-offset + only-complete-lines pattern
// from socket/transcript_thinking.go), emitting EventText (+ persisting via
// emitAgentText, category "text") for assistant text blocks and EventThinking
// (event-only, NO Sink row — socket's tailThinking stays the single writer of
// "thinking" rows) for thinking blocks. tool_use/tool_result blocks are
// skipped: those surface via the PreToolUse/PostToolUse hooks instead, so
// they are never duplicated here. Tolerates a missing/rotated file.
//
// flushMu serializes the whole read-process-advance sequence: flushes are
// driven by the tail ticker AND by hook-carrying socket goroutines
// (RequestApproval, NotifyTurnEnd), and two overlapping flushes would both
// start from the same offset and persist/emit every new line twice.
func (e *claudeEngine) flushTranscript() {
	e.flushMu.Lock()
	defer e.flushMu.Unlock()

	e.mu.Lock()
	spec := e.spec
	offset := e.transcriptOffset
	e.mu.Unlock()

	path := claudeTranscriptPath(spec.Env, spec.WorkDir, spec.effectiveCLISessionID())
	if path == "" {
		return
	}

	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return
	}
	if offset > fi.Size() {
		offset = 0 // rotated/truncated — restart from the beginning
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return
	}

	r := bufio.NewReaderSize(f, 1<<20)
	var consumed int64
	var lineCount int
	for consumed < claudeTranscriptTailMaxBytes && lineCount < claudeTranscriptTailMaxLines {
		line, readErr := r.ReadBytes('\n')
		if readErr != nil {
			break // partial line or EOF — left unconsumed for the next flush
		}
		consumed += int64(len(line))
		lineCount++
		e.processTranscriptLine(line)
	}

	e.mu.Lock()
	e.transcriptOffset = offset + consumed
	e.mu.Unlock()
}

// processTranscriptLine parses one JSONL transcript line and emits/persists
// its assistant text and thinking blocks. Non-assistant lines are ignored.
func (e *claudeEngine) processTranscriptLine(line []byte) {
	var entry struct {
		Type    string `json:"type"`
		Message struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &entry) != nil || entry.Type != "assistant" {
		return
	}
	e.mu.Lock()
	sessionID := e.spec.SessionID
	e.mu.Unlock()
	ingestClaudeTranscriptUsage(sessionID, line)

	var blocks []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		Thinking string `json:"thinking"`
	}
	if json.Unmarshal(entry.Message.Content, &blocks) != nil {
		// Bare-string content shape carries no thinking blocks.
		var s string
		if json.Unmarshal(entry.Message.Content, &s) == nil && s != "" {
			e.emitText(s)
		}
		return
	}

	for _, b := range blocks {
		switch b.Type {
		case "text":
			if b.Text != "" {
				e.emitText(b.Text)
			}
		case "thinking":
			if b.Thinking != "" {
				e.emitThinking(b.Thinking)
			}
			// tool_use / tool_result intentionally skipped — see doc comment.
		}
	}
}

func (e *claudeEngine) emitText(text string) {
	if len(text) > claudeTranscriptBodyCap {
		text = text[:claudeTranscriptBodyCap] + "\n…[truncated]"
	}
	e.mu.Lock()
	sessionID := e.spec.SessionID
	e.turnTextSeen = true
	e.mu.Unlock()
	if e.sink != nil {
		emitAgentText(sessionID, text, e.sink)
	}
	e.emit(EngineEvent{Type: EventText, SessionID: sessionID, Text: text})
}

func (e *claudeEngine) emitThinking(text string) {
	if len(text) > claudeTranscriptBodyCap {
		text = text[:claudeTranscriptBodyCap] + "\n…[truncated]"
	}
	e.mu.Lock()
	sessionID := e.spec.SessionID
	e.mu.Unlock()
	e.emit(EngineEvent{Type: EventThinking, SessionID: sessionID, Text: text})
}
