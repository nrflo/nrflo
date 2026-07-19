package refinery

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"be/internal/clock"
	"be/internal/logger"
	"be/internal/ws"
)

// debounceFloor is the minimum gap enforced between non-immediate folds.
const debounceFloor = 30 * time.Second

// foldFunc runs one fold for sessionID/projectID over the buffered event
// lines since the last fold.
type foldFunc func(ctx context.Context, sessionID, projectID string, events []string)

// sidecar is the per-session goroutine: a trigger channel plus a
// clock.After debounce loop that coalesces triggers arriving within the
// floor into a single fold, firing immediately instead on a completion
// trigger. Event lines buffer independently of trigger delivery, so a
// dropped/coalesced signal never loses data — only fold timing.
type sidecar struct {
	sessionID string
	projectID string
	clk       clock.Clock
	fold      foldFunc

	mu       sync.Mutex
	buffered []string

	triggerCh chan bool // true = immediate
	cancel    context.CancelFunc
	ctx       context.Context
	done      chan struct{}
}

func newSidecar(sessionID, projectID string, clk clock.Clock, fold foldFunc) *sidecar {
	ctx, cancel := context.WithCancel(context.Background())
	return &sidecar{
		sessionID: sessionID,
		projectID: projectID,
		clk:       clk,
		fold:      fold,
		triggerCh: make(chan bool, 32),
		ctx:       ctx,
		cancel:    cancel,
		done:      make(chan struct{}),
	}
}

// push appends an event line (when non-empty) and signals the loop.
func (s *sidecar) push(line string, immediate bool) {
	if line != "" {
		s.mu.Lock()
		s.buffered = append(s.buffered, line)
		s.mu.Unlock()
	}
	select {
	case s.triggerCh <- immediate:
	case <-s.ctx.Done():
	}
}

func (s *sidecar) run() {
	go s.loop()
}

// stop cancels the loop and waits for it to exit, so Manager.Stop never
// returns while a fold is still mid-flight against a session about to close.
func (s *sidecar) stop() {
	s.cancel()
	<-s.done
}

func (s *sidecar) loop() {
	defer close(s.done)
	var timerCh <-chan time.Time
	for {
		select {
		case <-s.ctx.Done():
			return
		case immediate := <-s.triggerCh:
			if immediate {
				s.runFold()
				timerCh = nil
			} else if timerCh == nil {
				timerCh = s.clk.After(debounceFloor)
			}
		case <-timerCh:
			s.runFold()
			timerCh = nil
		}
	}
}

func (s *sidecar) runFold() {
	s.mu.Lock()
	events := s.buffered
	s.buffered = nil
	s.mu.Unlock()
	if len(events) == 0 {
		return
	}
	s.fold(s.ctx, s.sessionID, s.projectID, events)
}

// formatEventLine renders a WS event into one compact line for the fold
// buffer: type plus the raw data payload, trimmed to what a digest actually
// needs (never the full Event, whose Sequence/Timestamp churn is noise).
func formatEventLine(ev *ws.Event) string {
	compact := map[string]interface{}{
		"type": ev.Type,
	}
	if ev.Workflow != "" {
		compact["workflow"] = ev.Workflow
	}
	if ev.TicketID != "" {
		compact["ticket_id"] = ev.TicketID
	}
	if len(ev.Data) > 0 {
		compact["data"] = ev.Data
	}
	raw, err := json.Marshal(compact)
	if err != nil {
		logger.Warn(context.Background(), "refinery: marshal event line failed", "type", ev.Type, "error", err)
		return ev.Type
	}
	return string(raw)
}
