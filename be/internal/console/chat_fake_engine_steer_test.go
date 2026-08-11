package console

import (
	"context"

	"be/internal/spawner"
)

// SteerUserTurn mirrors the claude/api engines' contract: only valid while a
// turn is active; steerErr is returned once instead of recording — the
// default fake supports steering so ChatService's steer-first path is
// exercised. steerUnsupported flips the codex behavior.
func (f *fakeConsoleEngine) SteerUserTurn(_ context.Context, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.steerUnsupported {
		return spawner.ErrSteeringUnsupported
	}
	if f.steerErr != nil {
		err := f.steerErr
		f.steerErr = nil
		return err
	}
	if !f.turnActive {
		return spawner.ErrNoActiveTurn
	}
	f.steers = append(f.steers, text)
	return nil
}

func (f *fakeConsoleEngine) steerTexts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.steers))
	copy(out, f.steers)
	return out
}

func (f *fakeConsoleEngine) setSteerUnsupported(on bool) {
	f.mu.Lock()
	f.steerUnsupported = on
	f.mu.Unlock()
}
