package socket

import (
	"testing"
)

// TestHandleGlobal_UnknownAction verifies unknown global actions return MethodNotFound.
func TestHandleGlobal_UnknownAction(t *testing.T) {
	env := newHandlerTestEnv(t)
	req := Request{
		ID:     "req-unknown",
		Method: "global.nonexistent_action",
		Params: []byte("{}"),
	}
	resp := env.handler.Handle(req)
	if resp.Error == nil {
		t.Fatal("expected error for unknown global action")
	}
	if resp.Error.Code != ErrCodeMethodNotFound {
		t.Errorf("code = %d, want %d (MethodNotFound)", resp.Error.Code, ErrCodeMethodNotFound)
	}
}
