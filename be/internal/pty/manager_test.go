package pty

import (
	"sync"
	"testing"
	"time"
)

// Note: buildTestEnv() is defined in session_test.go (same package).

func TestManager_GetReturnsNilForUnknown(t *testing.T) {
	m := NewManager()
	if got := m.Get("nonexistent"); got != nil {
		t.Errorf("Get(unknown) = %v, want nil", got)
	}
}

func TestManager_Remove_NoPanic(t *testing.T) {
	m := NewManager()
	// Remove on a non-existent key must not panic.
	m.Remove("does-not-exist")
}

func TestManager_CloseAll_Empty(t *testing.T) {
	m := NewManager()
	// CloseAll on empty manager must not panic.
	m.CloseAll()
}

// stubSession returns a Session with only sessionID and a closed done channel.
// It has nil ptmx/cmd so it cannot be Closed via the real Close() path.
// Use only for tests that don't exercise Close().
func stubSession(id string) *Session {
	done := make(chan struct{})
	close(done)
	return &Session{
		sessionID: id,
		done:      done,
	}
}

func TestManager_TrackAndGet(t *testing.T) {
	m := NewManager()
	sess := stubSession("sess-1")

	m.mu.Lock()
	m.sessions["sess-1"] = sess
	m.mu.Unlock()

	got := m.Get("sess-1")
	if got != sess {
		t.Errorf("Get returned wrong session")
	}
}

func TestManager_Remove_DeletesEntry(t *testing.T) {
	m := NewManager()
	m.mu.Lock()
	m.sessions["sess-2"] = stubSession("sess-2")
	m.mu.Unlock()

	m.Remove("sess-2")

	if got := m.Get("sess-2"); got != nil {
		t.Errorf("Get after Remove = %v, want nil", got)
	}
}

// TestManager_CloseAll_ClearsMap verifies that CloseAll terminates real PTY
// sessions (blocking /bin/cat) and empties the tracking map.
func TestManager_CloseAll_ClearsMap(t *testing.T) {
	m := NewManager()

	// Spawn two real PTY sessions (/bin/cat blocks on stdin).
	env := buildTestEnv()
	s1, err := newTestSession("cat", nil, t.TempDir(), env)
	if err != nil {
		t.Fatalf("newTestSession s1: %v", err)
	}
	s2, err := newTestSession("cat", nil, t.TempDir(), env)
	if err != nil {
		_ = s1.Close()
		t.Fatalf("newTestSession s2: %v", err)
	}

	m.mu.Lock()
	m.sessions["s1"] = s1
	m.sessions["s2"] = s2
	m.mu.Unlock()

	m.CloseAll()

	m.mu.Lock()
	count := len(m.sessions)
	m.mu.Unlock()

	if count != 0 {
		t.Errorf("sessions map len = %d after CloseAll, want 0", count)
	}
}

// TestManager_ConcurrentGetRemove exercises concurrent access to verify no
// data races (run with -race).
func TestManager_ConcurrentGetRemove(t *testing.T) {
	m := NewManager()
	ids := []string{"c1", "c2", "c3", "c4", "c5"}

	m.mu.Lock()
	for _, id := range ids {
		m.sessions[id] = stubSession(id)
	}
	m.mu.Unlock()

	var wg sync.WaitGroup
	for _, id := range ids {
		id := id
		wg.Add(2)
		go func() {
			defer wg.Done()
			m.Get(id)
		}()
		go func() {
			defer wg.Done()
			m.Remove(id)
		}()
	}
	wg.Wait()
}

// TestManager_RegisterCommand_UsedByCreate verifies that a command registered
// via RegisterCommand is consumed and used when Create is called for the same
// session ID.
func TestManager_RegisterCommand_UsedByCreate(t *testing.T) {
	m := NewManager()

	// Register "echo" so Create won't attempt to exec claude.
	m.RegisterCommand("sess-echo", "echo", []string{"hello"})

	m.mu.Lock()
	pc, hasPending := m.pending["sess-echo"]
	m.mu.Unlock()
	if !hasPending {
		t.Fatal("RegisterCommand did not create a pending entry")
	}
	if pc.Command != "echo" {
		t.Errorf("pending.Command = %q, want %q", pc.Command, "echo")
	}

	// Create should use "echo hello" instead of "claude --resume".
	sess, err := m.Create("sess-echo", t.TempDir(), buildTestEnv())
	if err != nil {
		t.Fatalf("Create with registered command: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })

	// Pending entry must be consumed by Create.
	m.mu.Lock()
	_, stillPending := m.pending["sess-echo"]
	m.mu.Unlock()
	if stillPending {
		t.Error("pending entry still present after Create; expected it to be consumed")
	}

	// echo exits immediately; wait for Done().
	select {
	case <-sess.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("session did not exit within timeout")
	}
}

// TestManager_RegisterCommand_NotConsumedForOtherSession verifies that a
// pending command registered for session A is not consumed when Create is
// called for a different session B.
func TestManager_RegisterCommand_NotConsumedForOtherSession(t *testing.T) {
	m := NewManager()

	// Register a command for sess-A.
	m.RegisterCommand("sess-A", "echo", []string{"hi"})

	// Pre-inject a stub session for sess-B so Create returns it without exec.
	existing := stubSession("sess-B")
	m.mu.Lock()
	m.sessions["sess-B"] = existing
	m.mu.Unlock()

	got, err := m.Create("sess-B", "/tmp", buildTestEnv())
	if err != nil {
		t.Fatalf("Create sess-B: %v", err)
	}
	if got != existing {
		t.Error("Create returned different session than pre-injected")
	}

	// sess-A's pending entry must still be present.
	m.mu.Lock()
	_, ok := m.pending["sess-A"]
	m.mu.Unlock()
	if !ok {
		t.Error("pending for sess-A was incorrectly consumed when creating sess-B")
	}
}

// TestManager_Remove_CleansPending verifies that Remove deletes an orphan
// pending entry (RegisterCommand called but Create never called).
func TestManager_Remove_CleansPending(t *testing.T) {
	m := NewManager()

	m.RegisterCommand("sess-orphan", "echo", []string{"hi"})

	m.mu.Lock()
	_, ok := m.pending["sess-orphan"]
	m.mu.Unlock()
	if !ok {
		t.Fatal("RegisterCommand did not create a pending entry")
	}

	m.Remove("sess-orphan")

	m.mu.Lock()
	_, stillPending := m.pending["sess-orphan"]
	m.mu.Unlock()
	if stillPending {
		t.Error("pending entry still present after Remove; expected it to be cleaned up")
	}
}

// TestManager_CreateReturnsSameSessionOnDuplicate verifies that Create returns
// the existing session when one is already tracked for the given ID, without
// spawning a new process.
func TestManager_CreateReturnsSameSessionOnDuplicate(t *testing.T) {
	m := NewManager()
	existing := stubSession("dup-sess")

	m.mu.Lock()
	m.sessions["dup-sess"] = existing
	m.mu.Unlock()

	// Call Create — the manager must return the existing session without
	// attempting to call NewSession (which would try to exec claude).
	got, err := m.Create("dup-sess", "/tmp", []string{})
	if err != nil {
		t.Fatalf("Create on duplicate returned error: %v", err)
	}
	if got != existing {
		t.Errorf("Create on duplicate returned different session, want the existing one")
	}
}
