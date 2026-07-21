package api

import "net/http"

// registerSessionRoutes registers observer, console session, and console-chat
// routes. Split out of server.go to keep that file's line count under its
// baseline.
//
// Every {sid}-scoped route below is registered `protected`, not
// `projectAdmin`: requireProjectAdmin resolves project scope from {id} first,
// which would misinterpret the session id as a project id. Authorization is
// instead enforced in-handler (admin user, matching/global service
// principal, or the session's own bearer — authorizedForConsoleClose,
// handlers_console.go:80) — the last of which lets a session close/drive
// itself, something requireProjectAdmin could never satisfy since a bearer
// request never populates the user context.
//
// The tools catalogue/dispatch routes, and /console/skills, are `protected`
// for the same reason: authorization is enforced in-handler by
// requireConsoleSession (401 unless the bearer resolves to a kind=console or
// kind=console_chat agent_sessions row), falling back to admin-user or
// matching/global service-token semantics for non-console callers.
func (s *Server) registerSessionRoutes(protected, projectAdmin func(string, http.HandlerFunc)) {
	// Observer sessions
	protected("POST /api/v1/observers", s.handleLaunchObserver)
	protected("GET /api/v1/observers", s.handleListObservers)

	// Console sessions
	projectAdmin("POST /api/v1/console/sessions", s.handleCreateConsoleSession)
	protected("POST /api/v1/console/sessions/{sid}/close", s.handleCloseConsoleSession)

	// Console tools
	protected("GET /api/v1/console/tools", s.handleListConsoleTools)
	protected("POST /api/v1/console/tools/{name}/call", s.handleCallConsoleTool)

	// Console chats: server-managed console-chat sessions (kind='console_chat').
	projectAdmin("GET /api/v1/console/catalog", s.handleGetConsoleCatalog)
	protected("GET /api/v1/console/skills", s.handleListConsoleSkills)
	protected("GET /api/v1/console/history", s.handleGetConsoleHistory)
	projectAdmin("POST /api/v1/console/chats", s.handleCreateConsoleChat)
	projectAdmin("GET /api/v1/console/chats", s.handleListConsoleChats)
	protected("GET /api/v1/console/chats/{sid}", s.handleGetConsoleChat)
	protected("POST /api/v1/console/chats/{sid}/messages", s.handleConsoleChatMessage)
	protected("POST /api/v1/console/chats/{sid}/approvals/{aid}", s.handleConsoleChatApproval)
	protected("DELETE /api/v1/console/chats/{sid}/session-approvals/{tool}", s.handleRevokeConsoleChatSessionApproval)
	protected("POST /api/v1/console/chats/{sid}/interrupt", s.handleInterruptConsoleChat)
	protected("POST /api/v1/console/chats/{sid}/close", s.handleCloseConsoleChat)
	protected("GET /api/v1/console/chats/{sid}/messages", s.handleGetConsoleChatMessages)
	protected("POST /api/v1/console/chats/{sid}/switch-model", s.handleSwitchConsoleChatModel)
	protected("POST /api/v1/console/chats/{sid}/hands-sibling", s.handleOpenHandsSibling)
	protected("GET /api/v1/console/chats/{sid}/tools", s.handleConsoleChatTools)
	protected("POST /api/v1/console/chats/{sid}/invoke", s.handleConsoleChatInvoke)
}
