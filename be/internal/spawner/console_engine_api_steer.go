package spawner

import (
	"context"
	"strings"
)

// SteerUserTurn accepts mid-turn user input for the api engine: the text is
// persisted as a user_input row immediately and buffered on e.steered; the
// running tool loop drains the buffer at its next tool-results boundary
// (apirun.Config.Steer), and the turn goroutine runs any leftover as a
// continuation turn before going idle (console_engine_api.go). Rejected with
// ErrNoActiveTurn when idle — the buffer would have no consumer.
func (e *apiConsoleEngine) SteerUserTurn(_ context.Context, text string) error {
	e.mu.Lock()
	if e.stopped {
		e.mu.Unlock()
		return ErrEngineStopped
	}
	if !e.turnActive {
		e.mu.Unlock()
		return ErrNoActiveTurn
	}
	e.steered = append(e.steered, text)
	spec := e.spec
	e.mu.Unlock()

	emitMessage(spec.SessionID, text, "user_input", e.sink)
	return nil
}

// takeSteeredLocked joins and clears the steered buffer. Caller holds e.mu.
func (e *apiConsoleEngine) takeSteeredLocked() string {
	text := strings.Join(e.steered, "\n\n")
	e.steered = nil
	return text
}

// apiEngineSteerSource adapts the engine's mu-guarded steered buffer to
// apirun.SteerSource for the runner's tool-boundary drain.
type apiEngineSteerSource struct{ e *apiConsoleEngine }

func (s apiEngineSteerSource) Drain() []string {
	s.e.mu.Lock()
	defer s.e.mu.Unlock()
	out := s.e.steered
	s.e.steered = nil
	return out
}
