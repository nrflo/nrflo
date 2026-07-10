package service

import "time"

// MarkToolEnded closes a tool span opened by a PreToolUse row: stamps the
// current time as ended_at into the row's payload, matched by tool_use_id.
// Best-effort — a missing pre-row (old sessions, unknown id) is not an error.
func (s *AgentService) MarkToolEnded(sessionID, toolUseID string) error {
	if sessionID == "" || toolUseID == "" {
		return nil
	}
	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.msgRepo.SetToolEnded(sessionID, toolUseID, now)
	return err
}
