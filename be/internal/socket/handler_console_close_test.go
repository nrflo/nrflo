package socket

import (
	"encoding/json"
	"errors"
	"testing"

	"be/internal/console"
)

var errBoom = errors.New("boom")

func TestConsoleAttach_ReturnsExistingScopedBearer(t *testing.T) {
	env := newHandlerTestEnv(t)
	creator := &fakeConsoleChatCreator{}
	env.handler.consoleChat = creator
	params, _ := json.Marshal(map[string]string{"project": env.project, "session_id": "chat-live-1"})
	resp := env.handler.Handle(Request{ID: "attach-1", Method: "console.attach", Params: params})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	var result map[string]string
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result["token"] != "chat-token-1" || creator.attached != "chat-live-1" {
		t.Fatalf("result=%+v attached=%q", result, creator.attached)
	}
}

// TestConsoleClose_HappyPath forwards session_id/project to CloseAuthenticated
// and returns the session id on success.
func TestConsoleClose_HappyPath(t *testing.T) {
	env := newHandlerTestEnv(t)
	creator := &fakeConsoleChatCreator{}
	env.handler.consoleChat = creator
	params, _ := json.Marshal(map[string]string{"project": env.project, "session_id": "chat-live-1"})
	resp := env.handler.Handle(Request{ID: "close-1", Method: "console.close", Params: params})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	var result map[string]string
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result["session_id"] != "chat-live-1" {
		t.Fatalf("result = %+v", result)
	}
	if creator.closed != "chat-live-1" || creator.project != env.project {
		t.Fatalf("creator args closed=%q project=%q", creator.closed, creator.project)
	}
}

// TestConsoleClose_MissingSessionID validates before reaching the service.
func TestConsoleClose_MissingSessionID(t *testing.T) {
	env := newHandlerTestEnv(t)
	creator := &fakeConsoleChatCreator{}
	env.handler.consoleChat = creator
	params, _ := json.Marshal(map[string]string{"project": env.project})
	resp := env.handler.Handle(Request{ID: "close-2", Method: "console.close", Params: params})
	if resp.Error == nil || resp.Error.Code != ErrCodeValidation {
		t.Fatalf("expected validation error, got %+v", resp.Error)
	}
	if creator.closed != "" {
		t.Errorf("CloseAuthenticated must not be called without session_id, closed=%q", creator.closed)
	}
}

// TestConsoleClose_UnknownSession maps ErrChatSessionNotFound to a not-found response.
func TestConsoleClose_UnknownSession(t *testing.T) {
	env := newHandlerTestEnv(t)
	creator := &fakeConsoleChatCreator{closeErr: console.ErrChatSessionNotFound}
	env.handler.consoleChat = creator
	params, _ := json.Marshal(map[string]string{"project": env.project, "session_id": "no-such-session"})
	resp := env.handler.Handle(Request{ID: "close-3", Method: "console.close", Params: params})
	if resp.Error == nil || resp.Error.Code != ErrCodeNotFound {
		t.Fatalf("expected not-found error, got %+v", resp.Error)
	}
}

// TestConsoleClose_ProjectMismatch maps ErrChatProjectMismatch to a validation response.
func TestConsoleClose_ProjectMismatch(t *testing.T) {
	env := newHandlerTestEnv(t)
	creator := &fakeConsoleChatCreator{closeErr: console.ErrChatProjectMismatch}
	env.handler.consoleChat = creator
	params, _ := json.Marshal(map[string]string{"project": env.project, "session_id": "chat-live-1"})
	resp := env.handler.Handle(Request{ID: "close-4", Method: "console.close", Params: params})
	if resp.Error == nil || resp.Error.Code != ErrCodeValidation {
		t.Fatalf("expected validation error, got %+v", resp.Error)
	}
}

// TestConsoleClose_OtherError maps an unrecognized service error to internal.
func TestConsoleClose_OtherError(t *testing.T) {
	env := newHandlerTestEnv(t)
	creator := &fakeConsoleChatCreator{closeErr: errBoom}
	env.handler.consoleChat = creator
	params, _ := json.Marshal(map[string]string{"project": env.project, "session_id": "chat-live-1"})
	resp := env.handler.Handle(Request{ID: "close-5", Method: "console.close", Params: params})
	if resp.Error == nil || resp.Error.Code != ErrCodeInternal {
		t.Fatalf("expected internal error, got %+v", resp.Error)
	}
}

// TestConsoleClose_NilConsoleChatService returns an internal error rather
// than panicking when no ConsoleChatCreator is wired.
func TestConsoleClose_NilConsoleChatService(t *testing.T) {
	env := newHandlerTestEnv(t)
	env.handler.consoleChat = nil
	params, _ := json.Marshal(map[string]string{"project": env.project, "session_id": "chat-live-1"})
	resp := env.handler.Handle(Request{ID: "close-6", Method: "console.close", Params: params})
	if resp.Error == nil || resp.Error.Code != ErrCodeInternal {
		t.Fatalf("expected internal error, got %+v", resp.Error)
	}
}

var _ ConsoleChatCreator = (*fakeConsoleChatCreator)(nil)
