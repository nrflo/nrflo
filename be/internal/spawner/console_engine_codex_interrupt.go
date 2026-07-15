package spawner

import (
	"context"
	"fmt"
)

// InterruptTurn asks app-server to stop the active turn while preserving the
// thread for later SendUserTurn calls.
func (e *codexEngine) InterruptTurn(ctx context.Context) error {
	e.mu.Lock()
	if !e.turnActive || e.turnID == "" {
		e.mu.Unlock()
		return ErrNoActiveTurn
	}
	client, threadID, turnID := e.client, e.threadID, e.turnID
	e.mu.Unlock()
	if client == nil {
		return ErrEngineStopped
	}
	if _, err := client.call(ctx, "turn/interrupt", map[string]string{"threadId": threadID, "turnId": turnID}); err != nil {
		return fmt.Errorf("console engine: turn/interrupt: %w", err)
	}
	return nil
}
