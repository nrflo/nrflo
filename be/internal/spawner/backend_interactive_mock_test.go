package spawner

import (
	"io"
	"sync"
)

// mockPtySession implements ptySessionIface for tests. Write records all bytes
// received; Close/Kill increment counters and close the done channel once.
// Read blocks on done channel until closed (returns io.EOF), but first drains
// any pre-loaded readChunks.
type mockPtySession struct {
	mu           sync.Mutex
	writtenBytes []byte
	closeCnt     int
	killCnt      int
	done         chan struct{}
	doneOnce     sync.Once
	exitCodeVal  int
	pidVal       int
	readChunks   []string
	readIdx      int
}

func newMockSession() *mockPtySession {
	return &mockPtySession{done: make(chan struct{})}
}

func (m *mockPtySession) Read(p []byte) (int, error) {
	m.mu.Lock()
	if m.readIdx < len(m.readChunks) {
		data := m.readChunks[m.readIdx]
		m.readIdx++
		m.mu.Unlock()
		return copy(p, data), nil
	}
	m.mu.Unlock()
	<-m.done
	return 0, io.EOF
}

func (m *mockPtySession) Write(p []byte) (int, error) {
	m.mu.Lock()
	m.writtenBytes = append(m.writtenBytes, p...)
	m.mu.Unlock()
	return len(p), nil
}

func (m *mockPtySession) Close() error {
	m.mu.Lock()
	m.closeCnt++
	m.mu.Unlock()
	m.doneOnce.Do(func() { close(m.done) })
	return nil
}

func (m *mockPtySession) Kill() error {
	m.mu.Lock()
	m.killCnt++
	m.mu.Unlock()
	m.doneOnce.Do(func() { close(m.done) })
	return nil
}

func (m *mockPtySession) Done() <-chan struct{} { return m.done }
func (m *mockPtySession) ExitCode() int         { return m.exitCodeVal }
func (m *mockPtySession) Pid() int               { return m.pidVal }

// mockPtyManager implements ptyManagerIface for tests.
type mockPtyManager struct {
	mu             sync.Mutex
	sessions       map[string]*mockPtySession
	registeredCmds map[string][]string
	createErr      error
	// lastEnv records the env slice passed to Create for each sessionID.
	// Populated only when Create succeeds.
	lastEnv map[string][]string
}

func newMockPtyManager() *mockPtyManager {
	return &mockPtyManager{
		sessions:       make(map[string]*mockPtySession),
		registeredCmds: make(map[string][]string),
		lastEnv:        make(map[string][]string),
	}
}

func (m *mockPtyManager) RegisterCommand(sessionID, cmd string, args []string) {
	m.mu.Lock()
	m.registeredCmds[sessionID] = append([]string{cmd}, args...)
	m.mu.Unlock()
}

func (m *mockPtyManager) Create(sessionID, _ string, env []string) (ptySessionIface, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	sess := newMockSession()
	m.mu.Lock()
	m.sessions[sessionID] = sess
	m.lastEnv[sessionID] = append([]string{}, env...)
	m.mu.Unlock()
	return sess, nil
}

func (m *mockPtyManager) Get(sessionID string) ptySessionIface {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[sessionID]; ok {
		return s
	}
	return nil
}
