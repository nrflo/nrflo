package spawner

import "testing"

// TestChildSessionHooks_ForwardsToParent pins the contract that the one-off
// child Spawners (context_save.go, consult_run.go, delegate.go) must keep: a
// child session has to reach the parent's OnSessionRegister/OnSessionUnregister,
// because those callbacks are what put it into the orchestrator's
// sessionID→*Spawner index. That index serves the MCP bridge's tools/list and
// routes record-event heartbeats to the proc, so dropping the parent callback
// is silent — the child gets an empty tool list and no heartbeat, then a
// healthy agent is killed on a loop by start-stall detection. Observed in
// production on the planner and again on the context-saver.
func TestChildSessionHooks_ForwardsToParent(t *testing.T) {
	t.Parallel()

	var parentRegistered, parentUnregistered, captured string
	var parentGotSpawner *Spawner
	parent := New(Config{
		OnSessionRegister: func(sessionID string, sp *Spawner) {
			parentRegistered = sessionID
			parentGotSpawner = sp
		},
		OnSessionUnregister: func(sessionID string) { parentUnregistered = sessionID },
	})

	var capturedFrom *Spawner
	register, unregister := parent.childSessionHooks(func(sessionID string, child *Spawner) {
		captured = sessionID
		capturedFrom = child
	})

	child := New(Config{})
	register("sess-1", child)
	if unregister == nil {
		t.Fatal("childSessionHooks returned a nil unregister while the parent has one")
	}
	unregister("sess-1")

	if captured != "sess-1" {
		t.Errorf("child capture = %q, want sess-1", captured)
	}
	if capturedFrom != child {
		t.Error("capture received the wrong *Spawner; callers filter on it to ignore grandchild registrations (nested delegate fanout)")
	}
	if parentRegistered != "sess-1" {
		t.Errorf("parent OnSessionRegister got %q, want sess-1 — the child session never reaches the orchestrator index", parentRegistered)
	}
	if parentUnregistered != "sess-1" {
		t.Errorf("parent OnSessionUnregister got %q, want sess-1 — the index entry leaks for the process lifetime", parentUnregistered)
	}
	if parentGotSpawner != child {
		t.Error("parent OnSessionRegister received the wrong *Spawner; the index would resolve tools/heartbeats to the parent, not the child that owns the proc")
	}
}

// TestChildSessionHooks_NilSafe covers the run-less callers: a parent with no
// callbacks of its own (every one-off child Spawner built outside an
// orchestrator run) must not panic, and the child's own sid capture must still
// fire so consult/delegate can resolve their session id.
func TestChildSessionHooks_NilSafe(t *testing.T) {
	t.Parallel()

	var captured string
	parent := New(Config{}) // no OnSessionRegister/OnSessionUnregister

	register, unregister := parent.childSessionHooks(func(sessionID string, _ *Spawner) { captured = sessionID })
	register("sess-2", New(Config{}))
	if unregister != nil {
		unregister("sess-2")
	}
	if captured != "sess-2" {
		t.Errorf("child capture = %q, want sess-2", captured)
	}

	// A child with no capture of its own (context-saver) still forwards.
	var parentRegistered string
	withParent := New(Config{
		OnSessionRegister: func(sessionID string, _ *Spawner) { parentRegistered = sessionID },
	})
	registerNoCapture, _ := withParent.childSessionHooks(nil)
	registerNoCapture("sess-3", New(Config{}))
	if parentRegistered != "sess-3" {
		t.Errorf("parent OnSessionRegister got %q, want sess-3 (nil captureSID must not skip forwarding)", parentRegistered)
	}
}
