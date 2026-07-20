package socket

import (
	"encoding/json"
	"testing"

	"be/internal/console"
)

// TestConsoleChat_PlumbsProfile verifies a non-empty profile param forwards
// through to CreateAuthenticated.
func TestConsoleChat_PlumbsProfile(t *testing.T) {
	env := newHandlerTestEnv(t)
	creator := &fakeConsoleChatCreator{}
	env.handler.consoleChat = creator
	params, _ := json.Marshal(map[string]string{
		"project": env.project, "engine": "claude", "model": "opus-4-8", "profile": "t0-decider",
	})
	resp := env.handler.Handle(Request{ID: "chat-profile-1", Method: "console.chat", Params: params})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if creator.profile != "t0-decider" {
		t.Errorf("creator.profile = %q, want t0-decider", creator.profile)
	}
}

// TestConsoleChat_EmptyProfile verifies the default (unset) case forwards an
// empty string, not a placeholder.
func TestConsoleChat_EmptyProfile(t *testing.T) {
	env := newHandlerTestEnv(t)
	creator := &fakeConsoleChatCreator{}
	env.handler.consoleChat = creator
	params, _ := json.Marshal(map[string]string{
		"project": env.project, "engine": "codex", "model": "gpt-5.3-codex",
	})
	resp := env.handler.Handle(Request{ID: "chat-profile-2", Method: "console.chat", Params: params})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if creator.profile != "" {
		t.Errorf("creator.profile = %q, want empty", creator.profile)
	}
}

// TestConsoleChat_UnknownProfile_MapsToValidationError verifies
// console.ErrUnknownProfile bubbling out of CreateAuthenticated becomes a
// validation error response, not an internal error.
func TestConsoleChat_UnknownProfile_MapsToValidationError(t *testing.T) {
	env := newHandlerTestEnv(t)
	creator := &fakeConsoleChatCreator{createErr: console.ErrUnknownProfile}
	env.handler.consoleChat = creator
	params, _ := json.Marshal(map[string]string{
		"project": env.project, "engine": "claude", "model": "opus-4-8", "profile": "no-such-profile",
	})
	resp := env.handler.Handle(Request{ID: "chat-profile-3", Method: "console.chat", Params: params})
	if resp.Error == nil {
		t.Fatal("expected an error response for an unknown profile")
	}
	if resp.Error.Code != ErrCodeValidation {
		t.Errorf("error code = %v, want %v", resp.Error.Code, ErrCodeValidation)
	}
}
