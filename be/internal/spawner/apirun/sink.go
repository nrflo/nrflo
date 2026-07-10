package apirun

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"be/internal/spawner/apirun/provider"
)

type runnerSink struct {
	msgSink         MessageSink
	mu              sync.Mutex
	buf             strings.Builder
	thinkBuf        strings.Builder
	captureThinking bool
	toolNames       map[string]string
}

func newRunnerSink(msgSink MessageSink, captureThinking bool) *runnerSink {
	return &runnerSink{
		msgSink:         msgSink,
		captureThinking: captureThinking,
		toolNames:       map[string]string{},
	}
}

func (s *runnerSink) OnTextDelta(text string) {
	if text == "" {
		return
	}
	s.mu.Lock()
	s.buf.WriteString(text)
	if s.buf.Len() >= 4096 {
		content := s.takeBufLocked()
		s.mu.Unlock()
		if content != "" {
			s.msgSink.TrackMessage(content, "text")
		}
		return
	}
	s.mu.Unlock()
}

func (s *runnerSink) OnThinkingDelta(text string) {
	if text == "" {
		return
	}
	s.mu.Lock()
	s.thinkBuf.WriteString(text)
	if s.thinkBuf.Len() >= 4096 {
		content := s.takeThinkBufLocked()
		s.mu.Unlock()
		if content != "" && s.captureThinking {
			s.msgSink.TrackMessage(content, "thinking")
		}
		return
	}
	s.mu.Unlock()
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
	s.msgSink.TrackToolInvoke(fmt.Sprintf("[%s] %s", name, compactStr), toolCategory(name), id)
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
	if s.buf.Len() == 0 {
		return ""
	}
	content := s.buf.String()
	s.buf.Reset()
	return content
}

func (s *runnerSink) takeThinkBufLocked() string {
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
