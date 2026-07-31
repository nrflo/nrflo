package apirun

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// FSSession is per-session state for the native fs builtins
// (tools_builtin/fs*.go): a read-before-edit/write tracking set plus a
// background-shell registry for bash's run_in_background. A ToolEnv is
// passed BY VALUE and shared across concurrent tool dispatch (cap 4), so this
// lives behind a pointer guarded by its own mutex. Every method is nil-safe —
// a nil *FSSession means "no session state wired" (console sessions, tests):
// callers check env.FS != nil separately to skip read-checks, and background-
// shell lookups simply report not-found.
type FSSession struct {
	mu      sync.Mutex
	readSet map[string]bool
	shells  map[string]*BgShell
	nextID  int
}

// NewFSSession returns an empty FSSession.
func NewFSSession() *FSSession {
	return &FSSession{
		readSet: make(map[string]bool),
		shells:  make(map[string]*BgShell),
	}
}

// MarkRead records abs (a symlink-resolved absolute path — resolveReadPath
// for read_file, resolveFSPath for write_file) as read (or written) this
// session, satisfying a later edit_file/write_file read-before-overwrite
// check. Both resolvers agree on the resolved form for a given file, so a
// read via either path matches WasRead on the write side. No-op on a nil
// receiver.
func (fs *FSSession) MarkRead(abs string) {
	if fs == nil {
		return
	}
	fs.mu.Lock()
	fs.readSet[abs] = true
	fs.mu.Unlock()
}

// WasRead reports whether abs has been read (or written) this session.
// Always false on a nil receiver — callers must check env.FS != nil
// separately to distinguish "no session state" (skip the check) from
// "session state present, not yet read" (enforce it).
func (fs *FSSession) WasRead(abs string) bool {
	if fs == nil {
		return false
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.readSet[abs]
}

// NewShellID returns a fresh, session-unique background-shell id. "" on a
// nil receiver.
func (fs *FSSession) NewShellID() string {
	if fs == nil {
		return ""
	}
	fs.mu.Lock()
	fs.nextID++
	id := fs.nextID
	fs.mu.Unlock()
	return fmt.Sprintf("bg_%d", id)
}

// AddShell registers sh under its ID. No-op on a nil receiver.
func (fs *FSSession) AddShell(sh *BgShell) {
	if fs == nil {
		return
	}
	fs.mu.Lock()
	fs.shells[sh.ID] = sh
	fs.mu.Unlock()
}

// GetShell looks up a registered background shell by ID.
func (fs *FSSession) GetShell(id string) (*BgShell, bool) {
	if fs == nil {
		return nil, false
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()
	sh, ok := fs.shells[id]
	return sh, ok
}

// KillAll kills every background shell registered on this session. Called
// when the owning agent session ends so no shell outlives it. No-op on a nil
// receiver.
func (fs *FSSession) KillAll() {
	if fs == nil {
		return
	}
	fs.mu.Lock()
	shells := make([]*BgShell, 0, len(fs.shells))
	for _, sh := range fs.shells {
		shells = append(shells, sh)
	}
	fs.mu.Unlock()
	for _, sh := range shells {
		sh.Kill()
	}
}

// bgShellBufferCap bounds a background shell's combined-output buffer so a
// long-running/noisy process cannot grow it unboundedly; oldest bytes are
// dropped first (a ring, not a hard cap on any single read).
const bgShellBufferCap = 1 << 20 // 1 MiB

// BgShell tracks one background bash invocation: the running/finished
// process, its combined stdout+stderr, and the read cursor bash_output
// advances. Safe for concurrent use.
type BgShell struct {
	ID        string
	Command   string
	StartedAt time.Time

	mu       sync.Mutex
	buf      bytes.Buffer
	readPos  int
	cmd      *exec.Cmd
	cancel   context.CancelFunc
	done     chan struct{}
	finished bool
	exitCode int
}

// NewBgShell constructs a BgShell not yet tracking a process; call Track once
// cmd.Start() has succeeded. cancel is the CancelFunc for the command's
// context — Kill calls it, which both terminates the process (via
// exec.CommandContext's Cancel) and unblocks the pending Wait in Track.
func NewBgShell(id, command string, startedAt time.Time, cancel context.CancelFunc) *BgShell {
	return &BgShell{
		ID:        id,
		Command:   command,
		StartedAt: startedAt,
		cancel:    cancel,
		done:      make(chan struct{}),
	}
}

// Write implements io.Writer so a BgShell can be wired directly as
// cmd.Stdout/cmd.Stderr; writes are mutex-guarded and the buffer is capped at
// bgShellBufferCap, dropping the oldest bytes first.
func (b *BgShell) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n, err := b.buf.Write(p)
	if over := b.buf.Len() - bgShellBufferCap; over > 0 {
		b.buf.Next(over)
		b.readPos -= over
		if b.readPos < 0 {
			b.readPos = 0
		}
	}
	return n, err
}

// Track starts a goroutine that waits on cmd and records its exit, then
// closes Done(). Must be called exactly once, after cmd.Start() succeeds.
func (b *BgShell) Track(cmd *exec.Cmd) {
	b.mu.Lock()
	b.cmd = cmd
	b.mu.Unlock()
	go func() {
		waitErr := cmd.Wait()
		b.mu.Lock()
		b.finished = true
		if waitErr != nil {
			if exitErr, ok := waitErr.(*exec.ExitError); ok {
				b.exitCode = exitErr.ExitCode()
			} else {
				b.exitCode = -1
			}
		}
		cancel := b.cancel
		b.mu.Unlock()
		// Release the timeout context now the process has exited, so its
		// timer/goroutine does not leak until the deadline. Kill early-returns
		// once finished, so cancel must be called here on normal completion.
		if cancel != nil {
			cancel()
		}
		close(b.done)
	}()
}

// Done returns a channel closed once the process has exited — the
// deterministic synchronization point tests use instead of a sleep.
func (b *BgShell) Done() <-chan struct{} {
	return b.done
}

// Kill terminates a still-running shell. No-op once finished or if Track was
// never called successfully.
func (b *BgShell) Kill() {
	b.mu.Lock()
	cancel := b.cancel
	finished := b.finished
	b.mu.Unlock()
	if finished || cancel == nil {
		return
	}
	cancel()
}

// BgShellSnapshot is one bash_output poll result.
type BgShellSnapshot struct {
	Status   string // "running" | "completed" | "failed"
	Output   string // new output since the last Snapshot call
	ExitCode int
}

// Snapshot returns output written since the last Snapshot call (all of it on
// the first call), plus status/exit code.
func (b *BgShell) Snapshot() BgShellSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	full := b.buf.String()
	pos := b.readPos
	if pos > len(full) {
		pos = len(full)
	}
	out := full[pos:]
	b.readPos = len(full)

	status := "running"
	if b.finished {
		status = "completed"
		if b.exitCode != 0 {
			status = "failed"
		}
	}
	return BgShellSnapshot{Status: status, Output: out, ExitCode: b.exitCode}
}
