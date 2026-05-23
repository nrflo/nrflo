package spawner

import (
	"bytes"
	"io"
	"os"
)

// Shared helpers for file-tailing context trackers. They translate raw tailer
// output into Sink calls and handle byte-offset incremental reads.

// emitAgentText records an agent_message body as a "text" agent_messages row
// and broadcasts the update. Empty bodies are dropped.
func emitAgentText(sessionID, body string, sink Sink) {
	if body == "" {
		return
	}
	emitMessage(sessionID, body, "text", sink)
}

// emitMessage is the common path: RecordHookMessage + BroadcastMessagesUpdated
// + BumpLastMessage + SetLastMessage.
func emitMessage(sessionID, body, category string, sink Sink) {
	projectID, ticketID, workflow, err := sink.RecordHookMessage(sessionID, body, category, "")
	if err != nil {
		return
	}
	sink.BroadcastMessagesUpdated(projectID, ticketID, workflow, sessionID)
	sink.BumpLastMessage(sessionID)
	// Surface a short preview in the periodic "agent status" log line.
	preview := body
	if len(preview) > 120 {
		preview = preview[:120]
	}
	sink.SetLastMessage(sessionID, preview)
}

// readNewLines reads from path starting at startOffset, calls fn for each
// complete newline-delimited line found, and returns the new offset. Partial
// trailing lines (no terminating newline yet) are NOT consumed — their bytes
// stay above startOffset on the next call so we re-read them once the rest
// arrives. Best-effort: read errors return startOffset unchanged.
func readNewLines(path string, startOffset int64, fn func(line []byte)) int64 {
	f, err := os.Open(path)
	if err != nil {
		return startOffset
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return startOffset
	}
	size := stat.Size()
	if size < startOffset {
		// File rotated/truncated. Reset.
		startOffset = 0
	}
	if size == startOffset {
		return startOffset
	}
	if startOffset > 0 {
		if _, err := f.Seek(startOffset, io.SeekStart); err != nil {
			return startOffset
		}
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return startOffset
	}

	// Walk full lines; keep any trailing partial as "unread" so the next
	// poll picks up the complete line.
	consumed := int64(0)
	for {
		idx := bytes.IndexByte(data[consumed:], '\n')
		if idx < 0 {
			break
		}
		line := data[consumed : consumed+int64(idx)]
		if len(line) > 0 {
			fn(line)
		}
		consumed += int64(idx) + 1
	}
	return startOffset + consumed
}
