package consoleui

import (
	"context"
	"errors"
	"testing"
)

type messageReader struct {
	data []byte
	done bool
}

func (r *messageReader) ReadMessage() (int, []byte, error) {
	if r.done {
		return 0, nil, errors.New("closed")
	}
	r.done = true
	return 1, r.data, nil
}

func TestReadEvents_DecodesCoalescedWebSocketFrame(t *testing.T) {
	reader := &messageReader{data: []byte(
		`{"type":"console_chat.delta","session_id":"s1","data":{"item_id":"i1","text":"hi"}}` + "\n" +
			`{"type":"console_chat.turn","session_id":"s1","data":{"state":"idle"}}`,
	)}
	out := make(chan streamUpdate, 1)
	err := readEvents(context.Background(), reader, out)
	if err == nil {
		t.Fatal("expected terminal read error")
	}
	update := <-out
	if len(update.Events) != 2 || update.Events[0].Type != "console_chat.delta" || update.Events[1].Type != "console_chat.turn" {
		t.Fatalf("events = %+v", update.Events)
	}
}
