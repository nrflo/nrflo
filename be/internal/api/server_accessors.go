package api

import (
	"be/internal/orchestrator"
	"be/internal/spawner"
	"be/internal/ws"
)

// Accessors handing internal Server collaborators to code outside the api
// package (the spawner, the socket server, cli/serve.go's wiring).

// GetWSHub returns the WebSocket hub for external access (e.g., spawner)
func (s *Server) GetWSHub() *ws.Hub {
	return s.wsHub
}

// GetOrchestrator returns the orchestrator for external access (e.g., socket server).
func (s *Server) GetOrchestrator() *orchestrator.Orchestrator {
	return s.orchestrator
}

// ConsoleHub returns the sessionID->live console engine registry wired to the
// socket server's ConsoleHooks (see cli/serve.go).
func (s *Server) ConsoleHub() *spawner.ConsoleHub {
	return s.consoleHub
}
