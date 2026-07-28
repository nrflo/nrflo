package api

import (
	"context"
	"errors"
	"testing"

	"be/internal/console"
	"be/internal/model"
)

// TestShutdownCleanup_StopsConsoleChatEngines verifies shutdownCleanup calls
// ChatService.StopAll before the agent_sessions sweep, so no console-chat
// engine (PTY child, app-server child) outlives the server.
func TestShutdownCleanup_StopsConsoleChatEngines(t *testing.T) {
	srv := newShutdownTestServer(t)
	factory := &fakeEngineFactory{}
	srv.consoleChatEngineFunc = factory.factory
	seedConsoleProject(t, srv, "proj-sd-chat")
	adminID := createTestUser(t, srv, "sd-chat-admin@test.com", model.UserRoleAdmin, false)
	cookie := injectSession(t, srv, adminID)

	sid, eng := createChatSession(t, srv, factory, "proj-sd-chat", cookie)

	srv.shutdownCleanup(context.Background())

	if !eng.isStopped() {
		t.Error("console-chat engine was not stopped by shutdownCleanup")
	}
	if _, err := srv.consoleChat.SendMessage(sid, "after shutdown"); !errors.Is(err, console.ErrChatSessionNotFound) {
		t.Errorf("SendMessage after shutdownCleanup = %v, want ErrChatSessionNotFound (session removed)", err)
	}
}
