package pty

import (
	"sync"
	"testing"
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
	s1, err := newSessionWithCmd("cat", []string{"cat"}, t.TempDir(), env)
	if err != nil {
		t.Fatalf("newSessionWithCmd s1: %v", err)
	}
	s2, err := newSessionWithCmd("cat", []string{"cat"}, t.TempDir(), env)
	if err != nil {
		_ = s1.Close()
		t.Fatalf("newSessionWithCmd s2: %v", err)
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
