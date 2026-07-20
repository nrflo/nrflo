package spawner

import (
	"encoding/json"
	"time"

	"be/internal/repo"
)

// TrackToolInvoke queues a tool-invoke message whose payload carries the
// tool_use_id and the raw tool input, so CloseToolSpan can stamp ended_at
// when the tool returns and the trace timeline / UI tool card can render the
// full invocation.
func (s *Spawner) TrackToolInvoke(proc *processInfo, msg, category, toolUseID string, rawInput []byte) {
	if toolUseID == "" {
		s.TrackMessage(proc, msg, category)
		return
	}
	payload := BuildToolInvokePayload(toolUseID, rawInput)
	if payload == "" {
		s.TrackMessage(proc, msg, category)
		return
	}
	proc.messagesMutex.Lock()
	proc.pendingMessages = append(proc.pendingMessages, repo.MessageEntry{Content: msg, Category: category, Payload: payload})
	proc.lastMessage = msg
	proc.messagesDirty = true
	proc.lastMessageTime = s.config.Clock.Now()
	proc.hasReceivedMessage = true
	proc.messagesMutex.Unlock()
	proc.appendRecent(msg)
}

// CloseToolSpan stamps ended_at on the invoke row for toolUseID. Fast tools
// are usually still in the pending buffer (stamped in memory before the next
// flush); slower ones have already been flushed, so fall back to the DB row.
func (s *Spawner) CloseToolSpan(proc *processInfo, toolUseID string) {
	if toolUseID == "" {
		return
	}
	now := s.config.Clock.Now().UTC().Format(time.RFC3339Nano)

	proc.messagesMutex.Lock()
	for i := range proc.pendingMessages {
		if stampPendingToolEnd(&proc.pendingMessages[i], toolUseID, now) {
			proc.messagesMutex.Unlock()
			return
		}
	}
	proc.messagesMutex.Unlock()

	pool := s.pool()
	if pool == nil {
		return
	}
	_, _ = repo.NewAgentMessageRepo(pool, s.config.Clock).SetToolEnded(proc.sessionID, toolUseID, now) // best-effort
}

// stampPendingToolEnd sets ended_at inside an in-memory entry's payload when
// it matches toolUseID and is not yet closed. Uses map[string]any (not
// map[string]string) because the payload now also carries a nested "input"
// object, which a map[string]string unmarshal would reject.
func stampPendingToolEnd(entry *repo.MessageEntry, toolUseID, endedAt string) bool {
	if entry.Payload == "" {
		return false
	}
	var p map[string]any
	if json.Unmarshal([]byte(entry.Payload), &p) != nil {
		return false
	}
	id, _ := p["tool_use_id"].(string)
	existingEnded, _ := p["ended_at"].(string)
	if id != toolUseID || existingEnded != "" {
		return false
	}
	p["ended_at"] = endedAt
	if b, err := json.Marshal(p); err == nil {
		entry.Payload = string(b)
		return true
	}
	return false
}
