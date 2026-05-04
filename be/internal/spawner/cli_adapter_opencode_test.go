package spawner

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// PrepareInteractive
// =============================================================================

func TestOpencodeAdapter_PrepareInteractive_AllocatesNonZeroPort(t *testing.T) {
	t.Parallel()
	a := &OpencodeAdapter{}
	extras, cleanup, err := a.PrepareInteractive(InteractivePrepOptions{WorkDir: "/tmp"})
	defer cleanup()
	if err != nil {
		t.Fatalf("PrepareInteractive() error: %v", err)
	}
	if extras.Port == 0 {
		t.Error("PrepareInteractive() Port = 0, want non-zero")
	}
}

func TestOpencodeAdapter_PrepareInteractive_PortIsFree(t *testing.T) {
	t.Parallel()
	a := &OpencodeAdapter{}
	extras, cleanup, err := a.PrepareInteractive(InteractivePrepOptions{WorkDir: "/tmp"})
	defer cleanup()
	if err != nil {
		t.Fatalf("PrepareInteractive() error: %v", err)
	}
	// The listener was released; we should be able to bind the port again.
	ln, bindErr := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", extras.Port))
	if bindErr != nil {
		t.Errorf("port %d not free after PrepareInteractive: %v", extras.Port, bindErr)
		return
	}
	ln.Close()
}

// =============================================================================
// BuildInteractiveCommand argv shape
// =============================================================================

func TestOpencodeAdapter_BuildInteractiveCommand_WorkDirFirstPositional(t *testing.T) {
	t.Parallel()
	a := &OpencodeAdapter{}
	opts := InteractiveSpawnOptions{
		Model:   "openai/gpt-5.4",
		WorkDir: "/projects/myapp",
		Port:    9000,
	}
	cmd := a.BuildInteractiveCommand(opts)
	// Args[0] is the executable; Args[1] must be workDir.
	if len(cmd.Args) < 2 {
		t.Fatalf("cmd.Args too short (%d), want >= 2", len(cmd.Args))
	}
	if cmd.Args[1] != "/projects/myapp" {
		t.Errorf("Args[1] = %q, want /projects/myapp; full args: %v", cmd.Args[1], cmd.Args)
	}
}

func TestOpencodeAdapter_BuildInteractiveCommand_HasPortAndHostname(t *testing.T) {
	t.Parallel()
	a := &OpencodeAdapter{}
	opts := InteractiveSpawnOptions{
		Model:   "openai/gpt-5.4",
		WorkDir: "/tmp",
		Port:    54321,
	}
	args := strings.Join(a.BuildInteractiveCommand(opts).Args, " ")
	if !strings.Contains(args, "--port 54321") {
		t.Errorf("BuildInteractiveCommand missing --port 54321: %s", args)
	}
	if !strings.Contains(args, "--hostname 127.0.0.1") {
		t.Errorf("BuildInteractiveCommand missing --hostname 127.0.0.1: %s", args)
	}
}

func TestOpencodeAdapter_BuildInteractiveCommand_PortValueIsNumeric(t *testing.T) {
	t.Parallel()
	a := &OpencodeAdapter{}
	opts := InteractiveSpawnOptions{Model: "openai/gpt-5.4", WorkDir: "/tmp", Port: 12345}
	cmd := a.BuildInteractiveCommand(opts)
	portIdx := -1
	for i, arg := range cmd.Args {
		if arg == "--port" {
			portIdx = i
			break
		}
	}
	if portIdx < 0 || portIdx+1 >= len(cmd.Args) {
		t.Fatalf("--port flag not found or has no value; args: %v", cmd.Args)
	}
	if _, err := strconv.Atoi(cmd.Args[portIdx+1]); err != nil {
		t.Errorf("--port value %q is not numeric: %v", cmd.Args[portIdx+1], err)
	}
}

func TestOpencodeAdapter_BuildInteractiveCommand_CmdDirMatchesWorkDir(t *testing.T) {
	t.Parallel()
	a := &OpencodeAdapter{}
	opts := InteractiveSpawnOptions{Model: "openai/gpt-5.4", WorkDir: "/work/myproject", Port: 8080}
	if cmd := a.BuildInteractiveCommand(opts); cmd.Dir != "/work/myproject" {
		t.Errorf("cmd.Dir = %q, want /work/myproject", cmd.Dir)
	}
}

func TestOpencodeAdapter_BuildInteractiveCommand_TERMAdded(t *testing.T) {
	t.Parallel()
	a := &OpencodeAdapter{}
	opts := InteractiveSpawnOptions{
		Model:   "openai/gpt-5.4",
		WorkDir: "/tmp",
		Port:    8080,
		Env:     []string{"HOME=/root"},
	}
	cmd := a.BuildInteractiveCommand(opts)
	for _, e := range cmd.Env {
		if e == "TERM=xterm-256color" {
			return // found
		}
	}
	t.Errorf("TERM=xterm-256color not added when absent; env: %v", cmd.Env)
}

func TestOpencodeAdapter_BuildInteractiveCommand_TERMNotDuplicated(t *testing.T) {
	t.Parallel()
	a := &OpencodeAdapter{}
	opts := InteractiveSpawnOptions{
		Model:   "openai/gpt-5.4",
		WorkDir: "/tmp",
		Port:    8080,
		Env:     []string{"TERM=xterm-256color", "HOME=/root"},
	}
	cmd := a.BuildInteractiveCommand(opts)
	count := 0
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "TERM=") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("TERM appears %d times in Env (want 1): %v", count, cmd.Env)
	}
}

// =============================================================================
// PostInteractiveStart: cleanup stops the SSE consumer goroutine
// =============================================================================

// TestOpencodeAdapter_PostInteractiveStart_CleanupStopsGoroutine starts the SSE
// consumer against a test HTTP server, calls cleanup(), and verifies the server
// handler exits (request cancelled) within 200ms.
func TestOpencodeAdapter_PostInteractiveStart_CleanupStopsGoroutine(t *testing.T) {
	t.Parallel()
	// handlerConnected signals when the server receives the SSE request.
	handlerConnected := make(chan struct{}, 1)
	handlerDone := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		select {
		case handlerConnected <- struct{}{}:
		default:
		}
		<-r.Context().Done() // block until client disconnects
		close(handlerDone)
	}))
	t.Cleanup(srv.Close)

	// Extract port from httptest server address.
	addr := srv.Listener.Addr().String()
	portStr := addr[strings.LastIndex(addr, ":")+1:]
	port, _ := strconv.Atoi(portStr)

	a := &OpencodeAdapter{}
	cleanup, err := a.PostInteractiveStart(context.Background(), PostInteractiveStartOptions{
		SessionID: "sess-oc-cleanup",
		WorkDir:   "/workdir",
		Port:      port,
		Sink:      &opencodeTestSink{},
	})
	if err != nil {
		t.Fatalf("PostInteractiveStart() error: %v", err)
	}

	// Wait until the consumer has actually connected to the server.
	select {
	case <-handlerConnected:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("SSE consumer did not connect to test server within 500ms")
	}

	cleanup()

	select {
	case <-handlerDone:
		// goroutine stopped — success
	case <-time.After(200 * time.Millisecond):
		t.Error("SSE consumer goroutine did not stop within 200ms after cleanup()")
	}
}
