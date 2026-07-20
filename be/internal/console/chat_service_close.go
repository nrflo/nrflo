package console

import (
	"context"
	"fmt"

	"be/internal/logger"
	"be/internal/repo"
	"be/internal/spawner"
)

// engineExited tears the session down after its engine's event channel closed
// (Stop, or the engine dying on its own). Idempotent: a Close-initiated exit
// finds the session already gone from the map and does nothing, so the row is
// never closed twice.
func (s *ChatService) engineExited(sid string) {
	s.mu.Lock()
	_, ok := s.sessions[sid]
	if ok {
		delete(s.sessions, sid)
	}
	s.mu.Unlock()
	if !ok {
		return
	}
	if s.deps.RefineryMgr != nil {
		s.deps.RefineryMgr.Stop(sid)
	}
	spawner.FinalizeSessionCost(sid)
	spawner.DropProactiveRestartState(sid)
	if _, err := repo.NewAgentSessionRepo(s.deps.Pool, s.deps.Clock).CloseConsoleChat(sid); err != nil {
		logger.Error(context.Background(), "console chat: close row after engine exit", "session_id", sid, "error", err)
	}
}

// Close stops the engine (its Events channel closing ends the event pump)
// and closes the DB row, killing its bearer token.
func (s *ChatService) Close(sid string) error {
	s.mu.Lock()
	sess, ok := s.sessions[sid]
	if ok {
		delete(s.sessions, sid)
	}
	s.mu.Unlock()
	if !ok {
		return ErrChatSessionNotFound
	}
	sess.getEngine().Stop()
	if s.deps.RefineryMgr != nil {
		s.deps.RefineryMgr.Stop(sid)
	}
	spawner.FinalizeSessionCost(sid)
	spawner.DropProactiveRestartState(sid)
	if _, err := repo.NewAgentSessionRepo(s.deps.Pool, s.deps.Clock).CloseConsoleChat(sid); err != nil {
		return fmt.Errorf("close console_chat session: %w", err)
	}
	return nil
}
