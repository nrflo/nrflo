package spawner

// Session activity signals. Every agent-liveness input the spawner has for a
// cli_interactive session arrives here, routed from the socket handler's
// record-event/agent.log path through the orchestrator's sessionID→*Spawner
// index. If that index does not hold the session, these all become silent
// no-ops and stall detection sees a healthy agent as frozen — see
// childSessionHooks in spawner_session.go.

// BumpLastMessage sends a non-blocking signal to monitorAll to update
// lastMessageTime and hasReceivedMessage for the matching proc. Used by the
// socket handler to reset stall detection when hook events arrive for
// interactive CLI agents. Silently dropped when channel is full.
func (s *Spawner) BumpLastMessage(sessionID string) {
	select {
	case s.bumpMessageCh <- sessionID:
	default:
	}
}

// SetLastMessage updates proc.lastMessage for the matching session so the
// status log line ("agent status ... last_msg=...") shows the most recent
// agent output. Interactive CLI mode otherwise leaves lastMessage empty
// because the PTY ferry drops bytes — hook events / SSE events feed content
// here directly. Also bumps lastMessageTime + hasReceivedMessage (same as
// BumpLastMessage) so stall/idle detection treats this as activity.
// No-op when the session is unknown or content is empty.
func (s *Spawner) SetLastMessage(sessionID, content string) {
	if content == "" {
		return
	}
	proc := s.lookupSessionProc(sessionID)
	if proc == nil {
		return
	}
	proc.messagesMutex.Lock()
	proc.lastMessage = content
	proc.lastMessageTime = s.config.Clock.Now()
	proc.hasReceivedMessage = true
	proc.messagesMutex.Unlock()
}

// dispatchBumpMessage finds the running proc matching a bumpMessageCh signal
// and updates its lastMessageTime/hasReceivedMessage so stall detection
// treats the hook event as activity. No-op if the session already finished.
func (s *Spawner) dispatchBumpMessage(running []*processInfo, sessionID string) {
	for _, proc := range running {
		if proc.sessionID == sessionID {
			proc.messagesMutex.Lock()
			proc.lastMessageTime = s.config.Clock.Now()
			proc.hasReceivedMessage = true
			proc.messagesMutex.Unlock()
			return
		}
	}
}

// TriggerIdleNudge sends a non-blocking signal to monitorAll to fire the
// existing idle-nudge machinery immediately for the matching proc — used by
// the socket handler when a Claude Notification hook indicates the agent is
// parked (idle-waiting or permission-prompt) instead of waiting out the
// wall-clock idle window. reason is "idle" or "permission" for the log
// marker. Silently dropped when channel is full.
func (s *Spawner) TriggerIdleNudge(sessionID, reason string) {
	select {
	case s.nudgeRequestCh <- nudgeRequest{sessionID: sessionID, reason: reason}:
	default:
	}
}
