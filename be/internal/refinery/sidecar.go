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

// flushReq is a synchronous fold request handed to the loop goroutine via
// flushCh; done is closed once the fold (or skip) completes.
type flushReq struct {
	ctx  context.Context
	done chan struct{}
}

// sidecar is the per-session goroutine: a trigger channel plus a
// clock.After debounce loop that coalesces triggers arriving within the
// floor into a single fold, firing immediately instead on a completion
// trigger. Event lines buffer independently of trigger delivery, so a
// dropped/coalesced signal never loses data — only fold timing. push's
// trigger send is non-blocking (drops on a full channel): a drop can only
// happen while >=1 trigger is already queued, so a fold is always still
// pending — only fold timing is affected, never data.
type sidecar struct {
	sessionID string
	projectID string
	clk       clock.Clock
	fold      foldFunc

	mu       sync.Mutex
	buffered []string

	triggerCh chan bool // true = immediate
	flushCh   chan flushReq
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
		flushCh:   make(chan flushReq),
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
	default:
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

// flush requests a synchronous fold on the loop goroutine and blocks until it
// completes (or ctx/the sidecar's own ctx ends first). Running the fold on
// the loop goroutine — rather than calling foldNow directly from the caller —
// is what serializes a flush against the debounce loop's own fold: the
// console fold path (fold.go) reads/writes refinery_digests with no slot
// lock, so two folds running concurrently could silently clobber each other.
func (s *sidecar) flush(ctx context.Context) {
	req := flushReq{ctx: ctx, done: make(chan struct{})}
	select {
	case s.flushCh <- req:
	case <-ctx.Done():
		return
	case <-s.ctx.Done():
		return
	}
	select {
	case <-req.done:
	case <-ctx.Done():
	}
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
				s.foldNow(s.ctx)
				timerCh = nil
			} else if timerCh == nil {
				timerCh = s.clk.After(debounceFloor)
			}
		case <-timerCh:
			s.foldNow(s.ctx)
			timerCh = nil
		case req := <-s.flushCh:
			// A caller whose ctx already expired must not produce a doomed
			// provider.Run + a bogus refinery_runs status=failed row.
			if req.ctx.Err() == nil {
				if s.foldNow(req.ctx) {
					timerCh = nil
				}
			}
			close(req.done)
		}
	}
}

// foldNow drains the buffered event lines under s.mu and always calls
// s.fold — each fold implementation owns its own emptiness check (Rule 6),
// since an autonomous fold ignores the buffer entirely while a console fold
// must also consider its own agent_messages delta.
func (s *sidecar) foldNow(ctx context.Context) bool {
	s.mu.Lock()
	events := s.buffered
	s.buffered = nil
	s.mu.Unlock()
	s.fold(ctx, s.sessionID, s.projectID, events)
	return true
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
