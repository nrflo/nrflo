package console

import "be/internal/service"

// RefineryLifecycle starts/stops a per-console-chat-session refinery sidecar.
// Satisfied by *refinery.Manager; declared here (not imported) so console
// never depends on the refinery package's internals — only this narrow
// lifecycle contract.
type RefineryLifecycle interface {
	Start(sessionID, projectID string)
	Stop(sessionID string)
}

// refineryEffective resolves whether the refinery sidecar should run for this
// chat: the per-create param wins outright, otherwise the global
// refinery_enabled setting decides (default off).
func (s *ChatService) refineryEffective(paramEnabled bool) bool {
	if paramEnabled {
		return true
	}
	val, _ := service.NewGlobalSettingsService(s.deps.Pool, s.deps.Clock).Get("refinery_enabled")
	return val == "true"
}
