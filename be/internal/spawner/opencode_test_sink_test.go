package spawner

import "sync"

// opencodeTestSink records Sink method invocations for assertions. Shared
// fixture used by opencode poll tests, codex JSONL tail tests, and any
// future sink-driven adapter tests.
type opencodeTestSink struct {
	mu             sync.Mutex
	recordedMsgs   []recordedMsg
	bumpCount      int
	turnCompletes  int
	contextUpdates []int
	errors         []string
	lastMessages   []string
}

type recordedMsg struct {
	content  string
	category string
}

func (s *opencodeTestSink) RecordHookMessage(sessionID, content, category, payload string) (string, string, string, error) {
	s.mu.Lock()
	s.recordedMsgs = append(s.recordedMsgs, recordedMsg{content, category})
	s.mu.Unlock()
	return "proj", "t1", "feature", nil
}

func (s *opencodeTestSink) UpdateContextLeft(sessionID string, pct int) (string, string, string, error) {
	s.mu.Lock()
	s.contextUpdates = append(s.contextUpdates, pct)
	s.mu.Unlock()
	return "proj", "t1", "feature", nil
}

func (s *opencodeTestSink) BumpLastMessage(sessionID string) {
	s.mu.Lock()
	s.bumpCount++
	s.mu.Unlock()
}

func (s *opencodeTestSink) SetLastMessage(sessionID, content string) {
	s.mu.Lock()
	s.lastMessages = append(s.lastMessages, content)
	s.mu.Unlock()
}

func (s *opencodeTestSink) OnTurnComplete(sessionID string) {
	s.mu.Lock()
	s.turnCompletes++
	s.mu.Unlock()
}

func (s *opencodeTestSink) BroadcastMessagesUpdated(projectID, ticketID, workflow, sessionID string) {
}

func (s *opencodeTestSink) RecordError(projectID, errType, sessionID, msg string) {
	s.mu.Lock()
	s.errors = append(s.errors, msg)
	s.mu.Unlock()
}

// snapshotMessages returns a copy of recordedMsgs for assertion (avoids
// races between the calling test goroutine and any still-running adapter
// goroutines that may still hold the mutex briefly).
func (s *opencodeTestSink) snapshotMessages() []recordedMsg {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]recordedMsg(nil), s.recordedMsgs...)
}
