package python

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Runner executes a Python script with the given input.
type Runner interface {
	Invoke(ctx context.Context, scriptPath string, input []byte, env []string, timeout time.Duration) ([]byte, error)
}

// Runtime wraps a Runner and a config directory for script resolution.
type Runtime struct {
	runner    Runner
	configDir string
}

// NewRuntime creates a Runtime with the given runner and config directory.
func NewRuntime(runner Runner, configDir string) *Runtime {
	return &Runtime{runner: runner, configDir: configDir}
}

// Invoke resolves scriptPath relative to configDir and calls the runner.
func (rt *Runtime) Invoke(ctx context.Context, scriptPath string, input []byte, env []string, timeout time.Duration) ([]byte, error) {
	abs, err := resolveScript(rt.configDir, scriptPath)
	if err != nil {
		return nil, err
	}
	return rt.runner.Invoke(ctx, abs, input, env, timeout)
}

// resolveScript returns an absolute path for scriptPath under configDir,
// rejecting empty, absolute, or parent-traversal paths.
func resolveScript(configDir, scriptPath string) (string, error) {
	if scriptPath == "" {
		return "", fmt.Errorf("script path must not be empty")
	}
	if filepath.IsAbs(scriptPath) {
		return "", fmt.Errorf("script path must be relative, got %q", scriptPath)
	}
	clean := filepath.Clean(scriptPath)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("script path must not traverse parent directories: %q", scriptPath)
	}
	return filepath.Join(configDir, clean), nil
}

// ScriptError is returned when a Python script exits with a non-zero code.
type ScriptError struct {
	ExitCode int
	Stderr   string
	Cause    error
}

func (e *ScriptError) Error() string {
	return fmt.Sprintf("script exited with code %d: %s", e.ExitCode, e.Stderr)
}

func (e *ScriptError) Unwrap() error { return e.Cause }

// ringBuffer captures the last N bytes of written data (stderr sink).
type ringBuffer struct {
	mu   sync.Mutex
	data []byte
	cap  int
}

func newRingBuffer(capacity int) *ringBuffer {
	return &ringBuffer{cap: capacity, data: make([]byte, 0, capacity)}
}

func (rb *ringBuffer) Write(p []byte) (int, error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.data = append(rb.data, p...)
	if len(rb.data) > rb.cap {
		rb.data = rb.data[len(rb.data)-rb.cap:]
	}
	return len(p), nil
}

func (rb *ringBuffer) String() string {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return string(rb.data)
}
