package socket

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"nrworkflow/internal/db"
	"nrworkflow/internal/service"
	"nrworkflow/internal/ws"
)

const (
	// DefaultSocketDir is the default directory for the socket file
	DefaultSocketDir = "/tmp/nrworkflow"
	// DefaultSocketName is the default socket file name
	DefaultSocketName = "nrworkflow.sock"
)

// GetSocketPath returns the socket path from env or default
func GetSocketPath() string {
	if path := os.Getenv("NRWORKFLOW_SOCKET"); path != "" {
		return path
	}
	return filepath.Join(DefaultSocketDir, DefaultSocketName)
}

// Server is the Unix socket server
type Server struct {
	pool       *db.Pool
	listener   net.Listener
	handler    *Handler
	socketPath string
	wsHub      *ws.Hub

	// Shutdown handling
	shutdown chan struct{}
	wg       sync.WaitGroup
	mu       sync.Mutex
	running  bool
}

// NewServer creates a new socket server
func NewServer(pool *db.Pool) *Server {
	return &Server{
		pool:       pool,
		socketPath: GetSocketPath(),
		shutdown:   make(chan struct{}),
		handler:    NewHandler(pool, nil),
	}
}

// NewServerWithHub creates a new socket server with WebSocket hub
func NewServerWithHub(pool *db.Pool, hub *ws.Hub) *Server {
	return &Server{
		pool:       pool,
		socketPath: GetSocketPath(),
		shutdown:   make(chan struct{}),
		handler:    NewHandler(pool, hub),
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

	log.Printf("Socket server listening on %s", s.socketPath)

	// Accept connections
	go s.acceptLoop()

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

	select {
	case <-done:
		log.Println("Socket server stopped gracefully")
	case <-ctx.Done():
		log.Println("Socket server shutdown timed out")
	}

	// Clean up socket file
	os.Remove(s.socketPath)

	return nil
}

// SocketPath returns the socket path
func (s *Server) SocketPath() string {
	return s.socketPath
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.shutdown:
				return
			default:
				log.Printf("Accept error: %v", err)
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
			log.Printf("Write error: %v", err)
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
	findingsSvc *service.FindingsService
	agentSvc    *service.AgentService
	wsHub       *ws.Hub
}

// NewHandler creates a new request handler
func NewHandler(pool *db.Pool, hub *ws.Hub) *Handler {
	return &Handler{
		findingsSvc: service.NewFindingsService(pool),
		agentSvc:    service.NewAgentService(pool),
		wsHub:       hub,
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
