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

const (
	// DefaultSocketDir is the default directory for the socket file
	DefaultSocketDir = "/tmp/nrworkflow"
	// DefaultSocketName is the default socket file name
	DefaultSocketName = "nrworkflow.sock"
	// DefaultTCPPort is the TCP port for Docker agent communication
	DefaultTCPPort = 6588
)

// GetSocketPath returns the socket path from env or default
func GetSocketPath() string {
	if path := os.Getenv("NRWORKFLOW_SOCKET"); path != "" {
		return path
	}
	return filepath.Join(DefaultSocketDir, DefaultSocketName)
}

// GetServerAddr returns the network and address for connecting to the server.
// If NRWORKFLOW_AGENT_HOST is set (e.g. "host.docker.internal:6588"), returns ("tcp", host).
// Otherwise falls back to ("unix", socketPath).
func GetServerAddr() (network, address string) {
	if host := os.Getenv("NRWORKFLOW_AGENT_HOST"); host != "" {
		return "tcp", host
	}
	return "unix", GetSocketPath()
}

// Server is the Unix socket server (with optional TCP listener for Docker agents)
type Server struct {
	pool        *db.Pool
	listener    net.Listener
	tcpListener net.Listener
	handler     *Handler
	socketPath  string
	wsHub       *ws.Hub

	// Shutdown handling
	shutdown chan struct{}
	wg       sync.WaitGroup
	mu       sync.Mutex
	running  bool
}

// NewServer creates a new socket server
func NewServer(pool *db.Pool, clk clock.Clock) *Server {
	return &Server{
		pool:       pool,
		socketPath: GetSocketPath(),
		shutdown:   make(chan struct{}),
		handler:    NewHandler(pool, nil, clk),
	}
}

// NewServerWithHub creates a new socket server with WebSocket hub
func NewServerWithHub(pool *db.Pool, hub *ws.Hub, clk clock.Clock) *Server {
	return &Server{
		pool:       pool,
		socketPath: GetSocketPath(),
		shutdown:   make(chan struct{}),
		handler:    NewHandler(pool, hub, clk),
		wsHub:      hub,
	}
}

// Start starts the socket server
func (s *Server) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("server already running")
	}
	s.running = true
	s.mu.Unlock()

	// Ensure socket directory exists
	dir := filepath.Dir(s.socketPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create socket directory: %w", err)
	}

	// Remove existing socket file
	if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove existing socket: %w", err)
	}

	// Create Unix socket listener
	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("failed to create socket listener: %w", err)
	}
	s.listener = listener

	// Set socket permissions (owner only)
	if err := os.Chmod(s.socketPath, 0600); err != nil {
		s.listener.Close()
		return fmt.Errorf("failed to set socket permissions: %w", err)
	}

	logger.Info(context.Background(), "socket server listening", "path", s.socketPath)

	// Accept connections
	go s.acceptLoop(s.listener)

	return nil
}

// StartTCP starts an additional TCP listener on the given port.
// This allows Docker containers to connect via host.docker.internal:<port>.
func (s *Server) StartTCP(port int) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return fmt.Errorf("server not running; call Start() first")
	}
	s.mu.Unlock()

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to create TCP listener: %w", err)
	}
	s.tcpListener = ln

	logger.Info(context.Background(), "socket server TCP listening", "addr", addr)

	go s.acceptLoop(ln)

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

	// Close listeners to stop accepting new connections
	if s.listener != nil {
		s.listener.Close()
	}
	if s.tcpListener != nil {
		s.tcpListener.Close()
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
	wsHub              *ws.Hub
}

// NewHandler creates a new request handler
func NewHandler(pool *db.Pool, hub *ws.Hub, clk clock.Clock) *Handler {
	return &Handler{
		findingsSvc:        service.NewFindingsService(pool, clk),
		projectFindingsSvc: service.NewProjectFindingsService(pool, clk),
		agentSvc:           service.NewAgentService(pool, clk),
		workflowSvc:        service.NewWorkflowService(pool, clk),
		wsHub:              hub,
	}
}

// broadcast sends a WebSocket event if hub is configured
func (h *Handler) broadcast(eventType, projectID, ticketID, workflow string, data map[string]interface{}) {
	if h.wsHub == nil {
		return
	}
	event := ws.NewEvent(eventType, projectID, ticketID, workflow, data)
	h.wsHub.Broadcast(event)
}
