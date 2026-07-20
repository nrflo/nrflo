package apirun

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"be/internal/spawner/apirun/provider"

	"github.com/google/uuid"
)

type runnerSink struct {
	msgSink  MessageSink
	mu       sync.Mutex
	buf      strings.Builder
	thinkBuf strings.Builder
	// textItemID/thinkItemID name the segment currently accumulating in the
	// matching buffer, and rotate on every flush. Each id therefore maps 1:1
	// onto exactly one persisted row, which is what lets a live consumer key
	// its delta buffer by id and drop it once the row lands (chatStream.ts's
	// dedupe). A single id reused across flushes would concatenate every
	// segment of the session into one ever-growing live bubble.
	textItemID      string
	thinkItemID     string
	captureThinking bool
	stream          StreamHook
	toolNames       map[string]string
}

func newRunnerSink(msgSink MessageSink, captureThinking bool, stream StreamHook) *runnerSink {
	return &runnerSink{
		msgSink:         msgSink,
		captureThinking: captureThinking,
		stream:          stream,
		toolNames:       map[string]string{},
	}
}

func (s *runnerSink) OnTextDelta(text string) {
	if text == "" {
		return
	}
	s.mu.Lock()
	if s.textItemID == "" {
		s.textItemID = uuid.New().String()
	}
	itemID := s.textItemID
	s.buf.WriteString(text)
	var content string
	if s.buf.Len() >= 4096 {
		content = s.takeBufLocked()
	}
	s.mu.Unlock()

	// Stream before persisting: the consumer's buffer must exist by the time
	// the row it dedupes against arrives.
	if s.stream != nil {
		s.stream.OnTextDelta(itemID, text)
	}
	if content != "" {
		s.msgSink.TrackMessage(content, "text")
	}
}

func (s *runnerSink) OnThinkingDelta(text string) {
	if text == "" {
		return
	}
	s.mu.Lock()
	if s.thinkItemID == "" {
		s.thinkItemID = uuid.New().String()
	}
	itemID := s.thinkItemID
	s.thinkBuf.WriteString(text)
	var content string
	if s.thinkBuf.Len() >= 4096 {
		content = s.takeThinkBufLocked()
	}
	s.mu.Unlock()

	if s.stream != nil {
		s.stream.OnThinkingDelta(itemID, text)
	}
	if content != "" && s.captureThinking {
		s.msgSink.TrackMessage(content, "thinking")
	}
}

func (s *runnerSink) OnToolUseStart(id, name string) {
	s.flush()
	s.mu.Lock()
	s.toolNames[id] = name
	s.mu.Unlock()
}

func (s *runnerSink) OnToolUseInputDelta(id, partialJSON string) {
	// Discarded — the full input arrives on OnToolUseStop.
}

func (s *runnerSink) OnToolUseStop(id string, fullInput json.RawMessage) {
	s.flush()
	s.mu.Lock()
	name := s.toolNames[id]
	if name == "" {
		name = id
	}
	delete(s.toolNames, id)
	s.mu.Unlock()

	var compact bytes.Buffer
	compactStr := string(fullInput)
	if err := json.Compact(&compact, fullInput); err == nil {
		compactStr = compact.String()
	}
	if len(compactStr) > 2048 {
		compactStr = compactStr[:2048]
	}
	s.msgSink.TrackToolInvoke(fmt.Sprintf("[%s] %s", name, compactStr), toolCategory(name), id, fullInput)
}

func (s *runnerSink) OnUsage(u provider.Usage) {
	s.flush()
}

func (s *runnerSink) flush() {
	s.mu.Lock()
	thinkContent := s.takeThinkBufLocked()
	content := s.takeBufLocked()
	s.mu.Unlock()
	if thinkContent != "" && s.captureThinking {
		s.msgSink.TrackMessage(thinkContent, "thinking")
	}
	if content != "" {
		s.msgSink.TrackMessage(content, "text")
	}
}

func (s *runnerSink) takeBufLocked() string {
	s.textItemID = ""
	if s.buf.Len() == 0 {
		return ""
	}
	content := s.buf.String()
	s.buf.Reset()
	return content
}

func (s *runnerSink) takeThinkBufLocked() string {
	s.thinkItemID = ""
	if s.thinkBuf.Len() == 0 {
		return ""
	}
	content := s.thinkBuf.String()
	s.thinkBuf.Reset()
	return content
}

func (s *runnerSink) close() {
	s.flush()
}

func toolCategory(name string) string {
	switch name {
	case "Task", "Agent":
		return "subagent"
	case "Skill":
		return "skill"
	default:
		return "tool"
	}
}
