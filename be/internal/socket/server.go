package socket

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/logger"
	"be/internal/service"
	"be/internal/ws"
)

// GetSocketPath returns the socket path from env or default ($NRFLO_HOME/agent.sock).
func GetSocketPath() string {
	if path := os.Getenv("NRFLO_SOCKET"); path != "" {
		return path
	}
	return filepath.Join(db.DefaultDataDir(), "agent.sock")
}

// BindListener resolves the socket path via GetSocketPath, performs pre-flight checks
// (path length, directory-at-path detection, stale-file removal, parent-dir creation),
// binds the Unix listener, and sets 0600 permissions. Returns the bound listener and
// resolved path; callers pass both to NewServerWithListener.
func BindListener() (net.Listener, string, error) {
	path := GetSocketPath()

	// sun_path limit: 104 bytes on macOS, 108 on Linux; pre-flight with a clear message.
	if len(path) > 100 {
		return nil, "", fmt.Errorf("socket path too long (%d bytes): %s", len(path), path)
	}

	if fi, statErr := os.Stat(path); statErr == nil {
		if fi.IsDir() {
			return nil, "", fmt.Errorf("socket path is a directory: %s", path)
		}
		if removeErr := os.Remove(path); removeErr != nil {
			return nil, "", fmt.Errorf("failed to remove stale socket file at %s: %w", path, removeErr)
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, "", fmt.Errorf("failed to create socket directory for %s: %w", path, err)
	}

	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, "", fmt.Errorf("failed to bind agent socket at %s: %w", path, err)
	}

	if err := os.Chmod(path, 0600); err != nil {
		ln.Close()
		return nil, "", fmt.Errorf("failed to set socket permissions at %s: %w", path, err)
	}

	return ln, path, nil
}

// GetServerAddr returns the network and address for connecting to the server.
func GetServerAddr() (network, address string) {
	return "unix", GetSocketPath()
}

// Server is the Unix socket server
type Server struct {
	pool     *db.Pool
	listener net.Listener
	handler  *Handler
	socketPath  string
	wsHub       *ws.Hub

	// Shutdown handling
	shutdown chan struct{}
	wg       sync.WaitGroup
	mu       sync.Mutex
	running  bool
}

// TerminalSignaler dispatches a best-effort kill signal to an active spawner
// after the socket handler has already written the agent result to the DB.
// The Handler nil-guards before calling — pass nil in tests.
type TerminalSignaler interface {
	RequestTerminalSignal(projectID, ticketID, workflow, sessionID, result string) error
	// BumpLastMessage resets stall-detection state for the matching proc so
	// hook-driven activity (PreToolUse/PostToolUse) does not trigger a stall restart.
	BumpLastMessage(projectID, ticketID, workflow, sessionID string) error
	// SetLastMessage updates the running proc's in-memory lastMessage so the
	// periodic "agent status" log line surfaces hook/SSE-delivered content for
	// interactive CLI agents (whose PTY output is otherwise dropped). Empty
	// content or unknown session is a no-op.
	SetLastMessage(projectID, ticketID, workflow, sessionID, content string) error
	// SignalSessionReady marks the matching proc as TUI-ready, unblocking the
	// PTY prompt-delivery wait. Best-effort and idempotent — repeated calls,
	// or calls for unknown sessions, are no-ops.
	SignalSessionReady(sessionID string) error
}

// NewServerWithListener creates a socket server with a pre-bound listener from BindListener.
func NewServerWithListener(pool *db.Pool, hub *ws.Hub, clk clock.Clock, signaler TerminalSignaler, listener net.Listener, socketPath string) *Server {
	return &Server{
		pool:       pool,
		socketPath: socketPath,
		listener:   listener,
		shutdown:   make(chan struct{}),
		handler:    NewHandler(pool, hub, clk, signaler),
		wsHub:      hub,
	}
}

// Start starts the socket server accept loop. The listener must be pre-bound via BindListener.
func (s *Server) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("server already running")
	}
	if s.listener == nil {
		s.mu.Unlock()
		return fmt.Errorf("socket server has no listener: use NewServerWithListener")
	}
	s.running = true
	s.mu.Unlock()

	logger.Info(context.Background(), "socket server listening", "path", s.socketPath)

	go s.acceptLoop(s.listener)

	return nil
}

// Stop gracefully stops the socket server
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	s.mu.Unlock()

	// Signal shutdown
	close(s.shutdown)

	// Close listener to stop accepting new connections
	if s.listener != nil {
		s.listener.Close()
	}

	// Wait for all connections to finish with timeout
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	bgCtx := context.Background()
	select {
	case <-done:
		logger.Info(bgCtx, "socket server stopped gracefully")
	case <-ctx.Done():
		logger.Warn(bgCtx, "socket server shutdown timed out")
	}

	// Clean up socket file
	os.Remove(s.socketPath)

	return nil
}

// SocketPath returns the socket path
func (s *Server) SocketPath() string {
	return s.socketPath
}

func (s *Server) acceptLoop(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-s.shutdown:
				return
			default:
				logger.Error(context.Background(), "socket accept error", "error", err)
				continue
			}
		}

		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close()

	// Set read deadline
	conn.SetReadDeadline(time.Now().Add(5 * time.Minute))

	reader := bufio.NewReader(conn)

	for {
		// Check for shutdown
		select {
		case <-s.shutdown:
			return
		default:
		}

		// Read line (JSON request)
		line, err := reader.ReadBytes('\n')
		if err != nil {
			// Connection closed or error
			return
		}

		// Parse request
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			resp := MakeErrorResponse("", NewParseError(err.Error()))
			s.writeResponse(conn, resp)
			continue
		}

		// Handle request
		resp := s.handler.Handle(req)

		// Write response
		if err := s.writeResponse(conn, resp); err != nil {
			logger.Error(context.Background(), "socket write error", "error", err)
			return
		}

		// Reset read deadline for next request
		conn.SetReadDeadline(time.Now().Add(5 * time.Minute))
	}
}

func (s *Server) writeResponse(conn net.Conn, resp Response) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	_, err = conn.Write(data)
	return err
}

// Handler dispatches requests to services
type Handler struct {
	findingsSvc        *service.FindingsService
	projectFindingsSvc *service.ProjectFindingsService
	agentSvc           *service.AgentService
	workflowSvc        *service.WorkflowService
	wfChainRunSvc      *service.WorkflowChainRunService
	wsHub              *ws.Hub
	signaler           TerminalSignaler // optional; nil-safe
	pool               *db.Pool
	clk                clock.Clock

	// codexJSONLOffsets tracks per-session byte offsets into the codex rollout
	// JSONL so each `Stop` hook scan only reads new bytes since the last flush.
	// In-memory only; not persisted across restarts (rare event; codex sessions
	// are short, so a re-flush on restart is acceptable noise).
	codexJSONLMu      sync.Mutex
	codexJSONLOffsets map[string]int64
}

// NewHandler creates a new request handler
func NewHandler(pool *db.Pool, hub *ws.Hub, clk clock.Clock, signaler TerminalSignaler) *Handler {
	return &Handler{
		findingsSvc:        service.NewFindingsService(pool, clk),
		projectFindingsSvc: service.NewProjectFindingsService(pool, clk),
		agentSvc:           service.NewAgentService(pool, clk),
		workflowSvc:        service.NewWorkflowService(pool, clk),
		wfChainRunSvc:      service.NewWorkflowChainRunService(pool, clk),
		wsHub:              hub,
		signaler:           signaler,
		pool:               pool,
		clk:                clk,
		codexJSONLOffsets:  make(map[string]int64),
	}
}

