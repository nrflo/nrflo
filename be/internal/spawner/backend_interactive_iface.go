package spawner

// ptySessionIface abstracts *pty.Session so tests can inject a mock PTY
// session (extracted from backend_interactive_helpers.go for the filesize
// ratchet).
type ptySessionIface interface {
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
	Resize(rows, cols uint16) error
	Close() error
	Kill() error
	Done() <-chan struct{}
	Pid() int
}
