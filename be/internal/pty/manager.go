package pty

import (
	"fmt"
	"strings"
	"sync"
)

// Launch is a registered command+args+env+dir for a PTY session. Argv, env
// overrides, and working directory all come from the adapter that owns the
// session (spawner.CLIAdapter for managed take-control resumes; the
// orchestrator's CLIAdapter-driven interactive/plan pre-step).
type Launch struct {
	Command string
	Args    []string
	Env     []string // overrides applied on top of Create's env, filter-then-append per key
	Dir     string   // when non-empty, wins over Create's workDir argument
}

// Manager tracks active PTY sessions by session ID.
type Manager struct {
	mu       sync.Mutex
	sessions map[string]*Session
	pending  map[string]*Launch
}

// NewManager creates a new PTY manager.
func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
		pending:  make(map[string]*Launch),
	}
}

// RegisterLaunch pre-registers a Launch for a session ID. When Create() is
// called for this session ID, the registered launch will be used.
func (m *Manager) RegisterLaunch(sessionID string, l Launch) {
	m.mu.Lock()
	defer m.mu.Unlock()
	launch := l
	m.pending[sessionID] = &launch
}

// PendingLaunch returns the launch registered for a session ID that Create()
// has not yet consumed.
func (m *Manager) PendingLaunch(sessionID string) (Launch, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.pending[sessionID]
	if !ok {
		return Launch{}, false
	}
	return *l, true
}

// Create spawns a new PTY session and tracks it. Returns an error if one
// already exists for the given session ID, or if no launch has been
// registered for a brand-new session.
func (m *Manager) Create(sessionID, workDir string, env []string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s, ok := m.sessions[sessionID]; ok {
		// Session already exists — return it (allows reconnect).
		return s, nil
	}

	pc, ok := m.pending[sessionID]
	if !ok {
		return nil, fmt.Errorf("no PTY launch registered for session %s", sessionID)
	}
	delete(m.pending, sessionID)

	dir := workDir
	if pc.Dir != "" {
		dir = pc.Dir
	}

	s, err := NewSession(sessionID, dir, mergeLaunchEnv(env, pc.Env), pc.Command, pc.Args)
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

// mergeLaunchEnv returns base with each overrides entry applied: any existing
// base entry sharing the override's key is dropped (filter-then-append per
// key), then the override is appended. Preserves base's order for untouched
// keys and overrides' order among themselves.
func mergeLaunchEnv(base, overrides []string) []string {
	if len(overrides) == 0 {
		return base
	}
	out := append([]string{}, base...)
	for _, ov := range overrides {
		key := ov
		if i := strings.IndexByte(ov, '='); i >= 0 {
			key = ov[:i+1]
		}
		filtered := out[:0:0]
		for _, e := range out {
			if strings.HasPrefix(e, key) {
				continue
			}
			filtered = append(filtered, e)
		}
		out = append(filtered, ov)
	}
	return out
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
	delete(m.pending, sessionID)
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
