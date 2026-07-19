package socket

// SetWorkflowRunner wires an optional orchestrator for observer trigger/retry methods.
func (s *Server) SetWorkflowRunner(r WorkflowOrchestrator) {
	s.handler.workflowRunner = r
}

// SetToolDispatcher wires the MCP tool dispatcher for api-via-cli sessions.
func (s *Server) SetToolDispatcher(d ToolDispatcher) {
	s.handler.toolDispatcher = d
}

// SetConsoleHooks wires the optional console-session hook router.
func (s *Server) SetConsoleHooks(h ConsoleHooks) {
	s.handler.consoleHooks = h
}

// SetConsoleChatCreator wires trusted local chat creation.
func (s *Server) SetConsoleChatCreator(c ConsoleChatCreator) {
	s.handler.consoleChat = c
}

// SetContextInjector wires the optional UserPromptSubmit context provider.
func (s *Server) SetContextInjector(i ContextInjector) {
	s.handler.contextInjector = i
}
