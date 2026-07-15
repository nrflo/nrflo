package consoleui

import (
	"bytes"
	"context"
	"encoding/json"
	"time"
)

const streamRetryMax = 5 * time.Second

func (c *Client) Stream(ctx context.Context) <-chan streamUpdate {
	out := make(chan streamUpdate, 16)
	go func() {
		defer close(out)
		delay := 250 * time.Millisecond
		for ctx.Err() == nil {
			conn, err := c.dialEvents(ctx)
			if err != nil {
				emitStream(ctx, out, streamUpdate{Connected: boolPtr(false), Err: err})
				if !waitContext(ctx, delay) {
					return
				}
				delay = min(delay*2, streamRetryMax)
				continue
			}
			delay = 250 * time.Millisecond
			connectionDone := make(chan struct{})
			go func() {
				select {
				case <-ctx.Done():
					_ = conn.Close()
				case <-connectionDone:
				}
			}()
			emitStream(ctx, out, streamUpdate{Connected: boolPtr(true)})
			err = readEvents(ctx, conn, out)
			close(connectionDone)
			conn.Close()
			if ctx.Err() == nil {
				emitStream(ctx, out, streamUpdate{Connected: boolPtr(false), Err: err})
				if !waitContext(ctx, delay) {
					return
				}
				delay = min(delay*2, streamRetryMax)
			}
		}
	}()
	return out
}

func readEvents(ctx context.Context, conn interface {
	ReadMessage() (int, []byte, error)
}, out chan<- streamUpdate) error {
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		batch := make([]Event, 0, 8)
		for _, line := range bytes.Split(data, []byte{'\n'}) {
			var ev Event
			if len(bytes.TrimSpace(line)) > 0 && json.Unmarshal(line, &ev) == nil {
				batch = append(batch, ev)
			}
		}
		if len(batch) > 0 && !emitStream(ctx, out, streamUpdate{Events: batch}) {
			return ctx.Err()
		}
	}
}

func emitStream(ctx context.Context, out chan<- streamUpdate, update streamUpdate) bool {
	select {
	case out <- update:
		return true
	case <-ctx.Done():
		return false
	}
}

func waitContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func boolPtr(value bool) *bool { return &value }
