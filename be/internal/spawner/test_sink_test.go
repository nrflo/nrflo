package spawner

import "sync"

// testSink records Sink method invocations for assertions. Shared fixture
// used by codex JSONL tail tests and any future sink-driven adapter tests.
type testSink struct {
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

func (s *testSink) RecordHookMessage(sessionID, content, category, payload string) (string, string, string, error) {
	s.mu.Lock()
	s.recordedMsgs = append(s.recordedMsgs, recordedMsg{content, category})
	s.mu.Unlock()
	return "proj", "t1", "feature", nil
}

func (s *testSink) UpdateContextLeft(sessionID string, pct int) (string, string, string, error) {
	s.mu.Lock()
	s.contextUpdates = append(s.contextUpdates, pct)
	s.mu.Unlock()
	return "proj", "t1", "feature", nil
}

func (s *testSink) BumpLastMessage(sessionID string) {
	s.mu.Lock()
	s.bumpCount++
	s.mu.Unlock()
}

func (s *testSink) SetLastMessage(sessionID, content string) {
	s.mu.Lock()
	s.lastMessages = append(s.lastMessages, content)
	s.mu.Unlock()
}

func (s *testSink) OnTurnComplete(sessionID string) {
	s.mu.Lock()
	s.turnCompletes++
	s.mu.Unlock()
}

func (s *testSink) BroadcastMessagesUpdated(projectID, ticketID, workflow, sessionID string) {
}

func (s *testSink) RecordError(projectID, errType, sessionID, msg string) {
	s.mu.Lock()
	s.errors = append(s.errors, msg)
	s.mu.Unlock()
}
