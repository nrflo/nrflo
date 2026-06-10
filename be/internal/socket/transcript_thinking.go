package socket

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"

	"be/internal/logger"
	"be/internal/service"
	"be/internal/ws"
)

const (
	maxThinkingBytes    = 256 << 10 // 256 KiB per-row truncate
	maxTailBytesPerCall = 8 << 20   // 8 MiB defensive cap per call
	maxTailLinesPerCall = 5000      // line cap per call
	readerBufSize       = 1 << 20   // 1 MiB bufio reader
)

// extractThinking parses a single JSONL line and returns any non-empty thinking
// strings from an assistant message's content array. Returns nil when the line
// is not an assistant turn or contains no thinking blocks.
func extractThinking(line []byte) []string {
	var m map[string]interface{}
	if err := json.Unmarshal(line, &m); err != nil {
		return nil
	}
	if m["type"] != "assistant" {
		return nil
	}
	msg, ok := m["message"].(map[string]interface{})
	if !ok {
		return nil
	}
	content, ok := msg["content"].([]interface{})
	if !ok {
		return nil
	}
	var out []string
	for _, elem := range content {
		block, ok := elem.(map[string]interface{})
		if !ok {
			continue
		}
		if block["type"] != "thinking" {
			continue
		}
		text, _ := block["thinking"].(string)
		if text != "" {
			out = append(out, text)
		}
	}
	return out
}

// tailThinking reads new complete lines from transcriptPath since the last stored
// offset, extracts thinking blocks, and inserts one agent_messages row per block
// with category="thinking". All errors are logged at INFO and swallowed.
func (h *Handler) tailThinking(ctx context.Context, sessionID, transcriptPath string) {
	if transcriptPath == "" {
		return
	}

	projectID, err := h.agentSvc.GetSessionProjectID(sessionID)
	if err != nil {
		logger.Info(ctx, "tailThinking: GetSessionProjectID error (best-effort)", "error", err, "session_id", sessionID)
		return
	}
	if projectID == "" {
		return
	}

	enabled, err := h.globalSettingsSvc.GetCaptureThinkingEnabled(projectID)
	if err != nil {
		logger.Info(ctx, "tailThinking: GetCaptureThinkingEnabled error (best-effort)", "error", err, "session_id", sessionID)
		return
	}
	if !enabled {
		return
	}

	h.thinkingMu.Lock()
	defer h.thinkingMu.Unlock()

	f, err := os.Open(transcriptPath)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Info(ctx, "tailThinking: open error (best-effort)", "error", err, "path", transcriptPath)
		}
		return
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		logger.Info(ctx, "tailThinking: stat error (best-effort)", "error", err, "path", transcriptPath)
		return
	}

	offset := h.thinkingOffsets[sessionID]
	if offset > fi.Size() {
		// File rotated or shrunk — restart from beginning
		offset = 0
	}

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		logger.Info(ctx, "tailThinking: seek error (best-effort)", "error", err, "path", transcriptPath)
		return
	}

	r := bufio.NewReaderSize(f, readerBufSize)
	var (
		consumed   int64
		lineCount  int
		thinkTexts []string
	)

	for consumed < maxTailBytesPerCall && lineCount < maxTailLinesPerCall {

		line, readErr := r.ReadBytes('\n')
		if readErr != nil {
			// Partial line or EOF — leave unconsumed for next call
			break
		}
		// Complete line (terminated with '\n')
		consumed += int64(len(line))
		lineCount++
		texts := extractThinking(line)
		thinkTexts = append(thinkTexts, texts...)
	}

	newOffset := offset + consumed
	h.thinkingOffsets[sessionID] = newOffset

	if len(thinkTexts) == 0 {
		return
	}

	var (
		insertedRows                          int
		lastProjectID, ticketID, workflowName string
	)
	for _, text := range thinkTexts {
		if len(text) > maxThinkingBytes {
			text = text[:maxThinkingBytes] + "\n…[truncated]"
		}
		pid, tid, wfName, insertErr := h.agentSvc.RecordHookMessage(sessionID, text, "thinking", "")
		if insertErr != nil {
			logger.Info(ctx, "tailThinking: RecordHookMessage error (best-effort)", "error", insertErr, "session_id", sessionID)
			continue
		}
		insertedRows++
		if pid != "" {
			lastProjectID = pid
			ticketID = tid
			workflowName = wfName
		}
	}

	if insertedRows > 0 && lastProjectID != "" {
		service.BroadcastFromCtx(h.wsHub, ws.EventMessagesUpdated, service.BroadcastCtx{
			ProjectID: lastProjectID,
			TicketID:  ticketID,
			Workflow:  workflowName,
		}, map[string]interface{}{
			"session_id": sessionID,
		})
		if h.signaler != nil {
			if sigErr := h.signaler.BumpLastMessage(lastProjectID, ticketID, workflowName, sessionID); sigErr != nil {
				logger.Info(ctx, "tailThinking: BumpLastMessage error (best-effort)", "error", sigErr)
			}
		}
	}
}
