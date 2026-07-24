package consoleui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestApplyStream_ConsoleChatYolo verifies a console_chat.yolo event sets
// m.detail.Yolo from the event's yolo field.
func TestApplyStream_ConsoleChatYolo(t *testing.T) {
	m := &model{detail: ChatDetail{SessionID: "s1"}, deltas: map[string]string{}}
	m.applyStream(streamUpdate{Events: []Event{
		event("console_chat.yolo", "s1", map[string]any{"yolo": true}),
	}})
	if !m.detail.Yolo {
		t.Error("detail.Yolo = false after console_chat.yolo{yolo:true}, want true")
	}

	m.applyStream(streamUpdate{Events: []Event{
		event("console_chat.yolo", "s1", map[string]any{"yolo": false}),
	}})
	if m.detail.Yolo {
		t.Error("detail.Yolo = true after console_chat.yolo{yolo:false}, want false")
	}
}

// TestHandleKey_CtrlY_TogglesYoloAndCallsClient verifies ctrl+y is handled,
// flips the requested state from the current m.detail.Yolo, and its
// returned command drives Client.SetYolo with that toggled value.
func TestHandleKey_CtrlY_TogglesYoloAndCallsClient(t *testing.T) {
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	m := &model{
		ctx:    context.Background(),
		client: NewClient(Config{BaseURL: srv.URL, Session: "sess-1"}),
		detail: ChatDetail{SessionID: "sess-1", Yolo: false},
	}

	cmd, handled := m.handleKey(ctrlYKeyMsg())
	if !handled {
		t.Fatal("handleKey(ctrl+y) handled = false, want true")
	}
	if cmd == nil {
		t.Fatal("handleKey(ctrl+y) returned a nil command")
	}
	msg := cmd()
	am, ok := msg.(actionMsg)
	if !ok {
		t.Fatalf("handleKey(ctrl+y) command produced %T, want actionMsg", msg)
	}
	if am.action != "yolo" {
		t.Errorf("actionMsg.action = %q, want yolo", am.action)
	}
	if am.err != nil {
		t.Errorf("actionMsg.err = %v, want nil", am.err)
	}
	// detail.Yolo was false, so the toggle must have POSTed (turn on).
	if gotMethod != http.MethodPost {
		t.Errorf("request method = %q, want POST (toggling false->true)", gotMethod)
	}
}

// ctrlYKeyMsg builds a tea.KeyPressMsg matching the "ctrl+y" case in
// handleKey (update.go matches on msg.String()).
func ctrlYKeyMsg() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl}
}
