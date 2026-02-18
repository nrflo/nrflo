package pty

import (
	"fmt"
	"sync"
)

// Manager tracks active PTY sessions by session ID.
type Manager struct {
	mu       sync.Mutex
	sessions map[string]*Session
}

// NewManager creates a new PTY manager.
func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
	}
}

// Create spawns a new PTY session and tracks it. Returns an error if one
// already exists for the given session ID.
func (m *Manager) Create(sessionID, workDir string, env []string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s, ok := m.sessions[sessionID]; ok {
		// Session already exists — return it (allows reconnect).
		return s, nil
	}

	s, err := NewSession(sessionID, workDir, env)
	if err != nil {
		return nil, fmt.Errorf("create pty session: %w", err)
	}
	m.sessions[sessionID] = s

	// Auto-remove when process exits.
	go func() {
		<-s.Done()
		m.Remove(sessionID)
	}()

	return s, nil
}

// Get returns the active PTY session for the given session ID, or nil.
func (m *Manager) Get(sessionID string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[sessionID]
}

// Remove stops tracking the session (does not close it).
func (m *Manager) Remove(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, sessionID)
}

// CloseAll closes all active PTY sessions. Called on server shutdown.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.sessions = make(map[string]*Session)
	m.mu.Unlock()

	for _, s := range sessions {
		_ = s.Close()
	}
}
