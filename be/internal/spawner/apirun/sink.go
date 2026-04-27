package apirun

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"be/internal/spawner/apirun/provider"
)

// Coalescing thresholds for streaming text deltas. Tuned for UI smoothness;
// revisit based on smoke-test feel.
const (
	textCoalesceIdleMs  = 200
	textCoalesceMaxChar = 80
)

// runnerSink implements provider.EventSink. Text deltas are coalesced into
// a buffer flushed on idle (200ms) or when the buffer exceeds 80 chars,
// matching CLI agents' message volume on the WS broadcast.
//
// All fields are guarded by mu. The provider contract guarantees callbacks
// are invoked from a single goroutine, but the idle flush runs from
// time.AfterFunc on a separate goroutine — so the lock is required.
type runnerSink struct {
	msgSink MessageSink

	mu      sync.Mutex
	buf     strings.Builder
	timer   *time.Timer
	closed  bool
}

func newRunnerSink(msgSink MessageSink) *runnerSink {
	return &runnerSink{msgSink: msgSink}
}

func (s *runnerSink) OnTextDelta(text string) {
	if text == "" {
		return
	}
	s.mu.Lock()
	s.buf.WriteString(text)
	if s.buf.Len() >= textCoalesceMaxChar {
		content := s.takeBufLocked()
		s.mu.Unlock()
		if content != "" {
			s.msgSink.TrackMessage(content, "text")
		}
		return
	}
	if s.timer == nil {
		s.timer = time.AfterFunc(textCoalesceIdleMs*time.Millisecond, s.idleFlush)
	} else {
		s.timer.Reset(textCoalesceIdleMs * time.Millisecond)
	}
	s.mu.Unlock()
}

func (s *runnerSink) OnToolUseStart(id, name string) {
	s.flush()
	s.msgSink.TrackMessage(fmt.Sprintf("[tool_use:start] id=%s name=%s", id, name), "tool_use_start")
}

func (s *runnerSink) OnToolUseInputDelta(id, partialJSON string) {
	// Discarded for v1 — the full input arrives on OnToolUseStop.
}

func (s *runnerSink) OnToolUseStop(id string, fullInput json.RawMessage) {
	s.flush()
	s.msgSink.TrackMessage(fmt.Sprintf("[tool_use:input] id=%s input=%s", id, string(fullInput)), "tool_use_input")
}

func (s *runnerSink) OnUsage(u provider.Usage) {
	s.flush()
}

// flush emits any buffered text and stops the idle timer. Safe to call
// multiple times.
func (s *runnerSink) flush() {
	s.mu.Lock()
	content := s.takeBufLocked()
	s.mu.Unlock()
	if content != "" {
		s.msgSink.TrackMessage(content, "text")
	}
}

// takeBufLocked returns the buffered text and resets the buffer + timer.
// Must be called with s.mu held.
func (s *runnerSink) takeBufLocked() string {
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	if s.buf.Len() == 0 {
		return ""
	}
	content := s.buf.String()
	s.buf.Reset()
	return content
}

func (s *runnerSink) idleFlush() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	content := s.takeBufLocked()
	s.mu.Unlock()
	if content != "" {
		s.msgSink.TrackMessage(content, "text")
	}
}

// close is called when the runner is done with this sink for the turn. It
// stops any pending timer to prevent late callbacks.
func (s *runnerSink) close() {
	s.mu.Lock()
	s.closed = true
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.mu.Unlock()
}
