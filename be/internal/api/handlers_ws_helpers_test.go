package api

import (
	"sync"
	"testing"
	"time"

	"be/internal/ws"
)

// wsRecorder captures WS events for handler tests.
type wsRecorder struct {
	mu     sync.Mutex
	events []*ws.Event
	ch     chan *ws.Event
}

func (r *wsRecorder) OnEvent(e *ws.Event) {
	r.mu.Lock()
	r.events = append(r.events, e)
	r.mu.Unlock()
	select {
	case r.ch <- e:
	default:
	}
}

// waitEvent polls until an event of the given type is seen or the timeout elapses.
func (r *wsRecorder) waitEvent(t *testing.T, eventType string) *ws.Event {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		for _, ev := range r.events {
			if ev.Type == eventType {
				r.mu.Unlock()
				return ev
			}
		}
		r.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for WS event %q", eventType)
	return nil
}
