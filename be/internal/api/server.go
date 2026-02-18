package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"be/internal/clock"
	"be/internal/config"
	"be/internal/db"
	"be/internal/logger"
	"be/internal/orchestrator"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/ws"
)

// Server represents the HTTP API server
type Server struct {
	config       *config.Config
	dataPath     string
	httpServer   *http.Server
	wsHub        *ws.Hub
	orchestrator *orchestrator.Orchestrator
	chainRunner  *orchestrator.ChainRunner
	clock        clock.Clock
}

// NewServer creates a new API server
func NewServer(cfg *config.Config, dataPath string) *Server {
	clk := clock.Real()
	hub := ws.NewHub(clk)
	orch := orchestrator.New(dataPath, hub, clk)
	return &Server{
		config:       cfg,
		dataPath:     dataPath,
		wsHub:        hub,
		orchestrator: orch,
		chainRunner:  orchestrator.NewChainRunner(orch, dataPath, hub, clk),
		clock:        clk,
	}
}

// GetWSHub returns the WebSocket hub for external access (e.g., spawner)
func (s *Server) GetWSHub() *ws.Hub {
	return s.wsHub
}

// Start starts the HTTP server
func (s *Server) Start(port int) error {
	// Recover zombie chains from previous crash
	if s.chainRunner != nil {
		s.chainRunner.RecoverZombieChains()
	}

	// Initialize event log for durable WS event persistence
	s.initEventLog()

	// Start retention cleanup for workflow instances and agent sessions
	s.startRetentionCleanup()

	// Start WebSocket hub
	go s.wsHub.Run()

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	handler := s.corsMiddleware(s.projectMiddleware(mux))

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: handler,
	}

	ctx := context.Background()
	logger.Info(ctx, "server starting", "port", port)
	logger.Info(ctx, "database path", "path", db.GetDBPath(s.dataPath))
	logger.Info(ctx, "websocket endpoint", "url", fmt.Sprintf("ws://localhost:%d/api/v1/ws", port))
	return s.httpServer.ListenAndServe()
}

// Stop gracefully stops the server
func (s *Server) Stop(ctx context.Context) error {
	// Cancel all active orchestrations
	if s.orchestrator != nil {
		s.orchestrator.StopAll()
	}
	// Stop WebSocket hub
	if s.wsHub != nil {
		s.wsHub.Stop()
	}
	if s.httpServer != nil {
		if ctx == nil {
			ctx = context.Background()
		}
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// initEventLog sets up the event log repo on the hub, configures the snapshot provider,
// and starts the retention cleanup goroutine.
func (s *Server) initEventLog() {
	database, err := s.getDatabase()
	if err != nil {
		logger.Info(context.Background(), "event log init failed, continuing without persistence", "error", err)
		return
	}
	elRepo := repo.NewEventLogRepo(database, s.clock)
	s.wsHub.SetEventLog(elRepo)

	// Set up snapshot provider backed by WorkflowService
	pool := db.WrapAsPool(database)
	wfSvc := service.NewWorkflowService(pool, s.clock)
	s.wsHub.SetSnapshotProvider(service.NewWorkflowSnapshotProvider(wfSvc))

	// Start retention cleanup: delete events older than 24h, every hour
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			deleted, err := elRepo.Cleanup(24 * time.Hour)
			if err != nil {
				logger.Info(context.Background(), "event log cleanup error", "error", err)
			} else if deleted > 0 {
				logger.Info(context.Background(), "event log cleanup", "deleted", deleted)
			}
		}
	}()
}

// startRetentionCleanup trims workflow_instances and agent_sessions to keep
// only the latest 100 non-active/non-running rows, every 20 minutes.
func (s *Server) startRetentionCleanup() {
	const keep = 100

	cleanup := func() {
		database, err := s.getDatabase()
		if err != nil {
			logger.Info(context.Background(), "retention cleanup: db open failed", "error", err)
			return
		}
		defer database.Close()

		pool := db.WrapAsPool(database)
		wfiRepo := repo.NewWorkflowInstanceRepo(pool, s.clock)
		asRepo := repo.NewAgentSessionRepo(database, s.clock)

		if deleted, err := wfiRepo.CleanupKeepLatest(keep); err != nil {
			logger.Info(context.Background(), "retention cleanup: workflow_instances error", "error", err)
		} else if deleted > 0 {
			logger.Info(context.Background(), "retention cleanup: workflow_instances", "deleted", deleted)
		}

		if deleted, err := asRepo.CleanupKeepLatest(keep); err != nil {
			logger.Info(context.Background(), "retention cleanup: agent_sessions error", "error", err)
		} else if deleted > 0 {
			logger.Info(context.Background(), "retention cleanup: agent_sessions", "deleted", deleted)
		}
	}

	// Run once immediately on startup
	go func() {
		cleanup()
		ticker := time.NewTicker(20 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			cleanup()
		}
	}()
}

// getDatabase returns a fresh database connection
func (s *Server) getDatabase() (*db.DB, error) {
	return db.Open(s.dataPath)
}

// getRepos returns ticket and dependency repos for the current request
func (s *Server) getRepos(r *http.Request) (*repo.TicketRepo, *repo.DependencyRepo, *db.DB, error) {
	database, err := s.getDatabase()
	if err != nil {
		return nil, nil, nil, err
	}
	return repo.NewTicketRepo(database, s.clock), repo.NewDependencyRepo(database, s.clock), database, nil
}

// getAllRepos returns all repos including agent session repo
func (s *Server) getAllRepos(r *http.Request) (*repo.TicketRepo, *repo.DependencyRepo, *repo.AgentSessionRepo, *repo.ProjectRepo, *db.DB, error) {
	database, err := s.getDatabase()
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	return repo.NewTicketRepo(database, s.clock), repo.NewDependencyRepo(database, s.clock), repo.NewAgentSessionRepo(database, s.clock), repo.NewProjectRepo(database, s.clock), database, nil
}

// corsMiddleware adds CORS headers
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowed := false
		for _, o := range s.config.Server.CORSOrigins {
			if o == origin || o == "*" {
				allowed = true
				break
			}
		}

		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Project")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

type contextKey string

const projectKey contextKey = "project"

// projectMiddleware extracts the project from X-Project header
func (s *Server) projectMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		project := r.Header.Get("X-Project")
		ctx := context.WithValue(r.Context(), projectKey, project)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// getProjectID extracts project ID from context or query param
func getProjectID(r *http.Request) string {
	// First check query param
	if p := r.URL.Query().Get("project"); p != "" {
		return p
	}
	// Then check header via context
	if p, ok := r.Context().Value(projectKey).(string); ok && p != "" {
		return p
	}
	return ""
}

// registerRoutes sets up all API routes
func (s *Server) registerRoutes(mux *http.ServeMux) {
	// WebSocket endpoint
	wsHandler := ws.NewHandler(s.wsHub)
	mux.Handle("GET /api/v1/ws", wsHandler)

	// Documentation
	mux.HandleFunc("GET /api/v1/docs/agent-manual", s.handleGetAgentManual)

	// Logs
	mux.HandleFunc("GET /api/v1/logs", s.handleGetLogs)

	// Projects
	mux.HandleFunc("GET /api/v1/projects", s.handleListProjects)
	mux.HandleFunc("POST /api/v1/projects", s.handleCreateProject)
	mux.HandleFunc("GET /api/v1/projects/{id}", s.handleGetProject)
	mux.HandleFunc("PATCH /api/v1/projects/{id}", s.handleUpdateProject)
	mux.HandleFunc("DELETE /api/v1/projects/{id}", s.handleDeleteProject)

	// Tickets (project-scoped)
	mux.HandleFunc("GET /api/v1/tickets", s.handleListTickets)
	mux.HandleFunc("POST /api/v1/tickets", s.handleCreateTicket)
	mux.HandleFunc("GET /api/v1/tickets/{id}", s.handleGetTicket)
	mux.HandleFunc("PATCH /api/v1/tickets/{id}", s.handleUpdateTicket)
	mux.HandleFunc("DELETE /api/v1/tickets/{id}", s.handleDeleteTicket)
	mux.HandleFunc("POST /api/v1/tickets/{id}/close", s.handleCloseTicket)
	mux.HandleFunc("POST /api/v1/tickets/{id}/reopen", s.handleReopenTicket)

	// Workflow (ticket-scoped runtime state)
	mux.HandleFunc("GET /api/v1/tickets/{id}/workflow", s.handleGetWorkflow)
	mux.HandleFunc("PATCH /api/v1/tickets/{id}/workflow", s.handleUpdateWorkflow)

	// Workflow orchestration (run/stop/restart from UI)
	mux.HandleFunc("POST /api/v1/tickets/{id}/workflow/run", s.handleRunWorkflow)
	mux.HandleFunc("POST /api/v1/tickets/{id}/workflow/stop", s.handleStopWorkflow)
	mux.HandleFunc("POST /api/v1/tickets/{id}/workflow/restart", s.handleRestartAgent)
	mux.HandleFunc("POST /api/v1/tickets/{id}/workflow/retry-failed", s.handleRetryFailedAgent)
	mux.HandleFunc("POST /api/v1/tickets/{id}/workflow/run-epic", s.handleRunEpicWorkflow)

	// Workflow definitions (project-scoped)
	mux.HandleFunc("GET /api/v1/workflows", s.handleListWorkflowDefs)
	mux.HandleFunc("POST /api/v1/workflows", s.handleCreateWorkflowDef)
	mux.HandleFunc("GET /api/v1/workflows/{id}", s.handleGetWorkflowDef)
	mux.HandleFunc("PATCH /api/v1/workflows/{id}", s.handleUpdateWorkflowDef)
	mux.HandleFunc("DELETE /api/v1/workflows/{id}", s.handleDeleteWorkflowDef)

	// Project-scoped workflow operations
	mux.HandleFunc("POST /api/v1/projects/{id}/workflow/run", s.handleRunProjectWorkflow)
	mux.HandleFunc("POST /api/v1/projects/{id}/workflow/stop", s.handleStopProjectWorkflow)
	mux.HandleFunc("POST /api/v1/projects/{id}/workflow/restart", s.handleRestartProjectAgent)
	mux.HandleFunc("POST /api/v1/projects/{id}/workflow/retry-failed", s.handleRetryFailedProjectAgent)
	mux.HandleFunc("GET /api/v1/projects/{id}/workflow", s.handleGetProjectWorkflow)
	mux.HandleFunc("GET /api/v1/projects/{id}/agents", s.handleGetProjectAgentSessions)

	// Git
	mux.HandleFunc("GET /api/v1/projects/{id}/git/commits", s.handleListGitCommits)
	mux.HandleFunc("GET /api/v1/projects/{id}/git/commits/{hash}", s.handleGetGitCommitDetail)

	// Agent definitions (nested under workflows)
	mux.HandleFunc("GET /api/v1/workflows/{wid}/agents", s.handleListAgentDefs)
	mux.HandleFunc("POST /api/v1/workflows/{wid}/agents", s.handleCreateAgentDef)
	mux.HandleFunc("GET /api/v1/workflows/{wid}/agents/{id}", s.handleGetAgentDef)
	mux.HandleFunc("PATCH /api/v1/workflows/{wid}/agents/{id}", s.handleUpdateAgentDef)
	mux.HandleFunc("DELETE /api/v1/workflows/{wid}/agents/{id}", s.handleDeleteAgentDef)

	// Agent sessions
	mux.HandleFunc("GET /api/v1/tickets/{id}/agents", s.handleGetAgentSessions)
	mux.HandleFunc("GET /api/v1/agents/recent", s.handleGetRecentAgents)
	mux.HandleFunc("GET /api/v1/sessions/{id}/messages", s.handleGetSessionMessages)

	// Dependencies
	mux.HandleFunc("GET /api/v1/tickets/{id}/dependencies", s.handleGetDependencies)
	mux.HandleFunc("POST /api/v1/dependencies", s.handleAddDependency)
	mux.HandleFunc("DELETE /api/v1/dependencies", s.handleRemoveDependency)

	// Chain executions
	mux.HandleFunc("GET /api/v1/chains", s.handleListChains)
	mux.HandleFunc("POST /api/v1/chains", s.handleCreateChain)
	mux.HandleFunc("GET /api/v1/chains/{id}", s.handleGetChain)
	mux.HandleFunc("PATCH /api/v1/chains/{id}", s.handleUpdateChain)
	mux.HandleFunc("POST /api/v1/chains/{id}/start", s.handleStartChain)
	mux.HandleFunc("POST /api/v1/chains/{id}/cancel", s.handleCancelChain)
	mux.HandleFunc("POST /api/v1/chains/{id}/append", s.handleAppendToChain)

	// Search
	mux.HandleFunc("GET /api/v1/search", s.handleSearch)

	// Status/Dashboard
	mux.HandleFunc("GET /api/v1/status", s.handleStatus)
	mux.HandleFunc("GET /api/v1/daily-stats", s.handleGetDailyStats)
}

// Helper functions for JSON responses

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func readJSON(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

// extractID gets the ticket ID from the URL path
func extractID(r *http.Request) string {
	id := r.PathValue("id")
	// Remove any surrounding spaces or slashes
	return strings.TrimSpace(id)
}
