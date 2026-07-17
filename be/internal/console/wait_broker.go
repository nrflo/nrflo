package console

import (
	"strings"
	"sync"

	"be/internal/ws"
)

// WaitBroker bridges hub broadcasts to blocked workflow_wait calls.
// Hub.RegisterListener is pre-Run only, so the broker is registered ONCE at
// server startup and fans wakes out to per-call subscribers, keyed by
// lowercase project id. A wake is a hint, not a payload: the waiter
// recomputes its state digest and decides whether anything changed.
type WaitBroker struct {
	mu      sync.Mutex
	waiters map[string]map[chan struct{}]struct{}
}

var _ ws.Listener = (*WaitBroker)(nil)

func NewWaitBroker() *WaitBroker {
	return &WaitBroker{waiters: make(map[string]map[chan struct{}]struct{})}
}

// OnEvent implements ws.Listener: wakes every waiter subscribed to the
// event's project. Sends are non-blocking — each waiter channel has capacity
// 1 and an already-queued wake is enough.
func (b *WaitBroker) OnEvent(e *ws.Event) {
	if e == nil || e.ProjectID == "" {
		return
	}
	key := strings.ToLower(e.ProjectID)
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.waiters[key] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// Subscribe registers a waiter for projectID's events. The returned cancel
// must be called (defer it) to release the registration.
func (b *WaitBroker) Subscribe(projectID string) (<-chan struct{}, func()) {
	key := strings.ToLower(projectID)
	ch := make(chan struct{}, 1)
	b.mu.Lock()
	if b.waiters[key] == nil {
		b.waiters[key] = make(map[chan struct{}]struct{})
	}
	b.waiters[key][ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.waiters[key], ch)
		if len(b.waiters[key]) == 0 {
			delete(b.waiters, key)
		}
		b.mu.Unlock()
	}
}

// WaiterCount reports the number of subscribed waiters for projectID.
func (b *WaitBroker) WaiterCount(projectID string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.waiters[strings.ToLower(projectID)])
}
