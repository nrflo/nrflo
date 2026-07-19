package spawner

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"strings"
)

const (
	ledgerTranscriptTailMaxBytes = 8 << 20 // defensive cap per flush
	ledgerTranscriptTailMaxLines = 5000
)

// claudeTranscriptBlock is the union of content-block fields observed across
// a Claude transcript's assistant (text/thinking/tool_use) and user
// (tool_result) entries.
type claudeTranscriptBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	ID        string          `json:"id"`          // tool_use
	Name      string          `json:"name"`        // tool_use
	Input     json.RawMessage `json:"input"`       // tool_use
	ToolUseID string          `json:"tool_use_id"` // tool_result
	Content   json.RawMessage `json:"content"`     // tool_result: string or block array
}

// ingestClaudeTranscript tails path for sessionID — EXACT-ish accounting from
// the CLI's own transcript, since the api-mode Observer path is unavailable
// for a CLI process. Reads only new complete lines since the ledger's stored
// byte offset (same offset+only-complete-lines pattern as
// console_engine_claude_transcript.go); a missing/rotated file restarts at 0.
func (s *Spawner) ingestClaudeTranscript(sessionID, path string) {
	if path == "" {
		return
	}
	l := globalLedgerStore.get(sessionID)

	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return
	}
	offset := l.transcriptOffsetVal()
	if offset > fi.Size() {
		offset = 0
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return
	}

	r := bufio.NewReaderSize(f, 1<<20)
	var consumed int64
	var lineCount int
	for consumed < ledgerTranscriptTailMaxBytes && lineCount < ledgerTranscriptTailMaxLines {
		line, readErr := r.ReadBytes('\n')
		if readErr != nil {
			break // partial line or EOF — left unconsumed for the next tick
		}
		consumed += int64(len(line))
		lineCount++
		ingestClaudeTranscriptLine(l, line)
	}
	l.setTranscriptOffset(offset + consumed)
}

// ingestClaudeTranscriptLine parses one JSONL transcript line into ledger
// entries: assistant text/thinking/tool_use, or user tool_result. Any other
// entry type is ignored.
func ingestClaudeTranscriptLine(l *ledger, line []byte) {
	var entry struct {
		Type    string `json:"type"`
		Message struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &entry) != nil {
		return
	}
	switch entry.Type {
	case "assistant":
		ingestClaudeAssistantBlocks(l, entry.Message.Content)
	case "user":
		ingestClaudeUserBlocks(l, entry.Message.Content)
	}
}

func ingestClaudeAssistantBlocks(l *ledger, raw json.RawMessage) {
	var blocks []claudeTranscriptBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return
	}
	l.nextTurn()
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if b.Text != "" {
				l.append(LedgerKindDialog, estTokens(len(b.Text)), "", ledgerSHA([]byte(b.Text)), true)
			}
		case "thinking":
			if b.Thinking != "" {
				l.append(LedgerKindDialog, estTokens(len(b.Thinking)), "", ledgerSHA([]byte(b.Thinking)), true)
			}
		case "tool_use":
			path := extractPathHint(b.Input)
			l.recordToolMeta(b.ID, toolCallMeta{name: b.Name, path: path})
			source := b.Name
			if path != "" {
				l.markRef(path)
				source = path
			}
			l.append(LedgerKindToolUse, estTokens(len(b.Name)+len(b.Input)), source, ledgerSHA(b.Input), true)
		}
	}
}

func ingestClaudeUserBlocks(l *ledger, raw json.RawMessage) {
	var blocks []claudeTranscriptBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return
	}
	for _, b := range blocks {
		if b.Type != "tool_result" {
			continue
		}
		meta := l.lookupToolMeta(b.ToolUseID)
		text := claudeTranscriptContentText(b.Content)
		if isReadToolName(meta.name) && meta.path != "" {
			l.append(LedgerKindFileRead, estTokens(len(text)), meta.path, "", true)
			continue
		}
		l.append(LedgerKindToolResult, estTokens(len(text)), meta.name, ledgerSHA([]byte(text)), true)
	}
}

// claudeTranscriptContentText extracts the text of a tool_result's content,
// which is either a plain string or an array of {"type":"text","text":...}
// blocks.
func claudeTranscriptContentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var blocks []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) == nil {
		var sb strings.Builder
		for _, b := range blocks {
			if b.Text == "" {
				continue
			}
			if sb.Len() > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString(b.Text)
		}
		return sb.String()
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return ""
}

// tailClaudeLedgers runs updateLedgerFromTranscript for every running proc —
// one call per monitorAll tick, no new polling loop.
func (s *Spawner) tailClaudeLedgers(running []*processInfo) {
	for _, proc := range running {
		s.updateLedgerFromTranscript(proc)
	}
}

// updateLedgerFromTranscript tails proc's Claude session transcript into the
// shared context ledger store and, once the debounce window has elapsed,
// broadcasts an epoch summary. Gated to backends that resolve a transcript
// (Claude PTY only, via SupportsResume() — not an adapter name-check); called
// once per monitorAll tick, no new polling loop.
func (s *Spawner) updateLedgerFromTranscript(proc *processInfo) {
	if proc.backend == nil || !proc.backend.SupportsResume() {
		return
	}
	path := claudeTranscriptPath(proc.env, proc.workDir, proc.sessionID)
	s.ingestClaudeTranscript(proc.sessionID, path)
	s.broadcastLedgerEpoch(proc)
}
