package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alexedwards/scs/v2"

	"be/internal/auth"
	"be/internal/chainrunner"
	"be/internal/clock"
	"be/internal/config"
	"be/internal/db"
	"be/internal/logger"
	"be/internal/notify"
	"be/internal/orchestrator"
	ptyPkg "be/internal/pty"
	"be/internal/repo"
	"be/internal/scheduler"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/static"
	"be/internal/ws"
)

// Server represents the HTTP API server
type Server struct {
	config                 *config.Config
	dataPath               string
	logsDir                string
	pool                   *db.Pool
	httpServer             *http.Server
	wsHub                  *ws.Hub
	orchestrator           *orchestrator.Orchestrator
	chainRunner            *orchestrator.ChainRunner
	wfChainRunner          *chainrunner.Runner
	ptyManager             *ptyPkg.Manager
	clock                  clock.Clock
	apiMode                bool
	cliAdapterFunc         func(cliType string) (spawner.CLIAdapter, error) // defaults to spawner.GetCLIAdapter
	scheduler              *scheduler.Scheduler
	notifyWaker            service.NotificationWaker
	notifyWorker           *notify.Worker
	notifyWorkerCancel     context.CancelFunc
	sessionMgr             *scs.SessionManager
	authSvc                *service.AuthService
	userSvc                *service.UserService
	rateLimiter            *loginRateLimiter
}

// NewServer creates a new API server.
// insecureCookies=true disables the Secure cookie flag (for local HTTP dev/testing).
func NewServer(cfg *config.Config, dataPath string, logsDir string, pool *db.Pool, apiMode bool, insecureCookies bool) *Server {
	clk := clock.Real()
	hub := ws.NewHub(clk)
	errorSvc := service.NewErrorService(pool, clk, hub)
	orch := orchestrator.New(dataPath, hub, clk, errorSvc, apiMode)
	ptyMgr := ptyPkg.NewManager()
	orch.OnRegisterPtyCommand = func(sessionID string, cmd string, args []string) {
		ptyMgr.RegisterCommand(sessionID, cmd, args)
	}
	orch.PTYManager = ptyMgr

	sched := scheduler.New(pool, orch, hub, clk)

	// Notification subsystem
	notifyWakeCh := make(chan struct{}, 8)
	channelRepo := repo.NewNotificationChannelRepo(pool, clk)
	deliveryRepo := repo.NewNotificationDeliveryRepo(pool, clk)
	dispatcher := notify.NewDispatcher(channelRepo, deliveryRepo, notifyWakeCh)
	hub.RegisterListener(dispatcher)
	waker := service.NewChanWaker(notifyWakeCh)
	notifyWorker := notify.NewWorker(deliveryRepo, channelRepo, hub, errorSvc, clk, notifyWakeCh)

	// Auth subsystem
	sessionMgr := auth.NewManager(pool.DB, insecureCookies)
	authSvc := service.NewAuthService(pool, clk)
	userSvc := service.NewUserService(pool, clk)

	return &Server{
		config:        cfg,
		dataPath:      dataPath,
		logsDir:       logsDir,
		pool:          pool,
		wsHub:         hub,
		orchestrator:  orch,
		chainRunner:   orchestrator.NewChainRunner(orch, dataPath, hub, clk),
		wfChainRunner: chainrunner.New(orch, dataPath, hub, clk),
		ptyManager:    ptyMgr,
		clock:         clk,
		apiMode:       apiMode,
		scheduler:     sched,
		notifyWaker:   waker,
		notifyWorker:  notifyWorker,
		sessionMgr:    sessionMgr,
		authSvc:       authSvc,
		userSvc:       userSvc,
		rateLimiter:   newLoginRateLimiter(),
	}
}

// GetWSHub returns the WebSocket hub for external access (e.g., spawner)
func (s *Server) GetWSHub() *ws.Hub {
	return s.wsHub
}

// GetOrchestrator returns the orchestrator for external access (e.g., socket server).
func (s *Server) GetOrchestrator() *orchestrator.Orchestrator {
	return s.orchestrator
}

// Start starts the HTTP server
func (s *Server) Start(host string, port int) error {
	// Recover zombie chains from previous crash
	if s.chainRunner != nil {
		s.chainRunner.RecoverZombieChains()
	}
	if s.wfChainRunner != nil {
		s.wfChainRunner.RecoverZombieRuns()
	}

	// Initialize event log for durable WS event persistence
	s.initEventLog()

	// Start retention cleanup for workflow instances and agent sessions
	s.startRetentionCleanup()

	// Reclaim orphaned prompt / system-suffix files and codex profile dirs
	// left behind by previous server processes that were killed mid-run.
	spawner.CleanupOrphanedTempFiles(1 * time.Hour)

	// Start WebSocket hub
	go s.wsHub.Run()

	// Start notification delivery worker
	if s.notifyWorker != nil {
		workerCtx, workerCancel := context.WithCancel(context.Background())
		s.notifyWorkerCancel = workerCancel
		go s.notifyWorker.Run(workerCtx)
	}

	// Start cron scheduler
	if s.scheduler != nil {
		if err := s.scheduler.Start(context.Background()); err != nil {
			logger.Info(context.Background(), "scheduler start error", "error", err)
		}
	}

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	// cors -> requestID -> projectMiddleware -> LoadAndSave (for /api/ paths only) -> mux
	handler := s.corsMiddleware(s.requestIDMiddleware(s.projectMiddleware(s.withSessionForAPI(mux))))

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf("%s:%d", host, port),
		Handler: handler,
	}

	ctx := context.Background()
	logger.Info(ctx, "server starting", "host", host, "port", port)
	logger.Info(ctx, "database path", "path", db.GetDBPath(s.dataPath))
	logger.Info(ctx, "websocket endpoint", "url", fmt.Sprintf("ws://%s:%d/api/v1/ws", host, port))
	return s.httpServer.ListenAndServe()
}

// withSessionForAPI applies SCS LoadAndSave only for /api/ path prefix.
// Static UI routes are excluded so session cookies are not set on SPA page loads.
func (s *Server) withSessionForAPI(next http.Handler) http.Handler {
	if s.sessionMgr == nil {
		return next
	}
	ls := s.sessionMgr.LoadAndSave(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			ls.ServeHTTP(w, r)
		} else {
			next.ServeHTTP(w, r)
		}
	})
}

// Stop gracefully stops the server
func (s *Server) Stop(ctx context.Context) error {
	// Stop notification delivery worker
	if s.notifyWorkerCancel != nil {
		s.notifyWorkerCancel()
	}
	// Stop cron scheduler
	if s.scheduler != nil {
		s.scheduler.Stop()
	}
	// Cancel all active orchestrations
	if s.orchestrator != nil {
		s.orchestrator.StopAll()
	}
	// Close all PTY sessions
	if s.ptyManager != nil {
		s.ptyManager.CloseAll()
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
	elRepo := repo.NewEventLogRepo(s.pool, s.clock)
	s.wsHub.SetEventLog(elRepo)

	// Set up snapshot provider backed by WorkflowService
	wfSvc := service.NewWorkflowService(s.pool, s.clock)
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
// only the latest N non-active/non-running rows, every 20 minutes.
// N is read from the session_retention_limit global setting (default 1000).
func (s *Server) startRetentionCleanup() {
	cleanup := func() {
		svc := service.NewGlobalSettingsService(s.pool, s.clock)
		keep := 1000
		if val, err := svc.Get("session_retention_limit"); err == nil && val != "" {
			if parsed, parseErr := strconv.Atoi(val); parseErr == nil && parsed >= 10 {
				keep = parsed
			}
		}

		wfiRepo := repo.NewWorkflowInstanceRepo(s.pool, s.clock)
		asRepo := repo.NewAgentSessionRepo(s.pool, s.clock)

		if deleted, err := wfiRepo.CleanupKeepLatest(keep); err != nil {
			logger.Info(context.Background(), "retention cleanup: workflow_instances error", "error", err)
		} else if deleted > 0 {
			logger.Info(context.Background(), "retention cleanup: workflow_instances", "deleted", deleted)
		}

		// Clean up orphaned messages (sessions removed by CASCADE).
		// Agent sessions are NOT cleaned independently — CASCADE from
		// workflow_instances deletion handles them.
		if deleted, err := asRepo.CleanupOrphanedMessages(); err != nil {
			logger.Info(context.Background(), "retention cleanup: orphaned messages error", "error", err)
		} else if deleted > 0 {
			logger.Info(context.Background(), "retention cleanup: orphaned messages", "deleted", deleted)
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

// ticketRepo returns a ticket repo backed by the connection pool
func (s *Server) ticketRepo() *repo.TicketRepo {
	return repo.NewTicketRepo(s.pool, s.clock)
}

// depRepo returns a dependency repo backed by the connection pool
func (s *Server) depRepo() *repo.DependencyRepo {
	return repo.NewDependencyRepo(s.pool, s.clock)
}

// projectRepo returns a project repo backed by the connection pool
func (s *Server) projectRepo() *repo.ProjectRepo {
	return repo.NewProjectRepo(s.pool, s.clock)
}

// agentSessionRepo returns an agent session repo backed by the connection pool
func (s *Server) agentSessionRepo() *repo.AgentSessionRepo {
	return repo.NewAgentSessionRepo(s.pool, s.clock)
}

// ticketService returns a ticket service backed by the connection pool
func (s *Server) ticketService() *service.TicketService {
	return service.NewTicketService(s.pool, s.clock)
}

// workflowService returns a workflow service backed by the connection pool
func (s *Server) workflowService() *service.WorkflowService {
	return service.NewWorkflowService(s.pool, s.clock)
}

// isAPISession returns true when the agent session identified by sessionID was
// spawned in API execution mode. Used by take-control handlers to reject early.
// Errors during lookup are treated as false (session not found / not API mode).
func isAPISession(s *Server, sessionID string) bool {
	sess, err := s.agentSessionRepo().Get(sessionID)
	if err != nil {
		return false
	}
	wfiRepo := repo.NewWorkflowInstanceRepo(s.pool, s.clock)
	wfi, err := wfiRepo.Get(sess.WorkflowInstanceID)
	if err != nil {
		return false
	}
	agentDefRepo := repo.NewAgentDefinitionRepo(s.pool, s.clock)
	def, err := agentDefRepo.Get(sess.ProjectID, wfi.WorkflowID, sess.AgentType)
	return err == nil && def.ExecutionMode == "api"
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
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Project, X-Request-ID")
		w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// requestIDMiddleware generates a unique trx per request, injects it into the
// context, and sets the X-Request-ID response header.
func (s *Server) requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trx := logger.NewTrx()
		ctx := logger.WithTrx(r.Context(), trx)
		w.Header().Set("X-Request-ID", trx)
		next.ServeHTTP(w, r.WithContext(ctx))
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

// registerRoutes sets up all API routes.
// protected wraps with requireAuth; admin wraps with requireAdmin (admin role required);
// public registers with no auth wrapper.
// LoadAndSave is applied upstream in Start() via withSessionForAPI.
func (s *Server) registerRoutes(mux *http.ServeMux) {
	protected := func(pat string, h http.HandlerFunc) {
		mux.Handle(pat, s.requireAuth(h))
	}
	admin := func(pat string, h http.HandlerFunc) {
		mux.Handle(pat, s.requireAdmin(h))
	}

	// Auth endpoints
	mux.HandleFunc("POST /api/v1/auth/login", s.handleAuthLogin)           // public (login page)
	protected("POST /api/v1/auth/logout", s.handleAuthLogout)
	protected("GET /api/v1/auth/me", s.handleAuthMe)
	protected("POST /api/v1/auth/change-password", s.handleAuthChangePassword)

	// User management (admin-only)
	admin("GET /api/v1/users", s.handleListUsers)
	admin("POST /api/v1/users", s.handleCreateUser)
	admin("PATCH /api/v1/users/{id}", s.handleUpdateUser)
	admin("DELETE /api/v1/users/{id}", s.handleDeleteUser)
	admin("POST /api/v1/users/{id}/reset-password", s.handleResetUserPassword)

	// Audit log (admin-only)
	admin("GET /api/v1/audit-log", s.handleListAuditLog)

	// WebSocket endpoints — gated on session before upgrade
	wsHandler := ws.NewHandler(s.wsHub)
	mux.Handle("GET /api/v1/ws", s.requireAuth(wsHandler))
	protected("GET /api/v1/pty/{session_id}", s.handlePtyWebSocket)

	// Documentation
	protected("GET /api/v1/docs/agent-manual", s.handleGetAgentManual)

	// Logs
	protected("GET /api/v1/logs", s.handleGetLogs)

	// Projects — POST is admin; reads and updates are protected
	protected("GET /api/v1/projects", s.handleListProjects)
	admin("POST /api/v1/projects", s.handleCreateProject)
	protected("GET /api/v1/projects/{id}", s.handleGetProject)
	protected("PATCH /api/v1/projects/{id}", s.handleUpdateProject)
	admin("DELETE /api/v1/projects/{id}", s.handleDeleteProject)

	// Tickets (project-scoped)
	protected("GET /api/v1/tickets", s.handleListTickets)
	protected("POST /api/v1/tickets", s.handleCreateTicket)
	protected("GET /api/v1/tickets/{id}", s.handleGetTicket)
	protected("PATCH /api/v1/tickets/{id}", s.handleUpdateTicket)
	protected("DELETE /api/v1/tickets/{id}", s.handleDeleteTicket)
	protected("POST /api/v1/tickets/{id}/close", s.handleCloseTicket)
	protected("POST /api/v1/tickets/{id}/reopen", s.handleReopenTicket)

	// Workflow (ticket-scoped runtime state)
	protected("GET /api/v1/tickets/{id}/workflow", s.handleGetWorkflow)
	protected("PATCH /api/v1/tickets/{id}/workflow", s.handleUpdateWorkflow)

	// Workflow orchestration (run/stop/restart from UI)
	protected("POST /api/v1/tickets/{id}/workflow/run", s.handleRunWorkflow)
	protected("POST /api/v1/tickets/{id}/workflow/stop", s.handleStopWorkflow)
	protected("POST /api/v1/tickets/{id}/workflow/restart", s.handleRestartAgent)
	protected("POST /api/v1/tickets/{id}/workflow/retry-failed", s.handleRetryFailedAgent)
	protected("POST /api/v1/tickets/{id}/workflow/take-control", s.handleTakeControl)
	protected("POST /api/v1/tickets/{id}/workflow/resume-session", s.handleResumeSession)
	protected("POST /api/v1/tickets/{id}/workflow/exit-interactive", s.handleExitInteractive)
	protected("POST /api/v1/tickets/{id}/workflow/run-epic", s.handleRunEpicWorkflow)

	// Workflow definitions (project-scoped)
	protected("GET /api/v1/workflows", s.handleListWorkflowDefs)
	protected("POST /api/v1/workflows", s.handleCreateWorkflowDef)
	protected("GET /api/v1/workflows/{id}", s.handleGetWorkflowDef)
	protected("PATCH /api/v1/workflows/{id}", s.handleUpdateWorkflowDef)
	protected("DELETE /api/v1/workflows/{id}", s.handleDeleteWorkflowDef)

	// Project-scoped workflow operations
	protected("POST /api/v1/projects/{id}/workflow/run", s.handleRunProjectWorkflow)
	protected("POST /api/v1/projects/{id}/workflow/stop", s.handleStopProjectWorkflow)
	protected("POST /api/v1/projects/{id}/workflow/restart", s.handleRestartProjectAgent)
	protected("POST /api/v1/projects/{id}/workflow/retry-failed", s.handleRetryFailedProjectAgent)
	protected("POST /api/v1/projects/{id}/workflow/take-control", s.handleTakeControlProject)
	protected("POST /api/v1/projects/{id}/workflow/resume-session", s.handleResumeSessionProject)
	protected("POST /api/v1/projects/{id}/workflow/exit-interactive", s.handleExitInteractiveProject)
	protected("POST /api/v1/projects/{id}/workflow/stop-endless-loop", s.handleStopEndlessLoop)
	protected("DELETE /api/v1/projects/{id}/workflow/{instance_id}", s.handleDeleteProjectWorkflowInstance)
	protected("GET /api/v1/projects/{id}/workflow", s.handleGetProjectWorkflow)
	protected("GET /api/v1/projects/{id}/agents", s.handleGetProjectAgentSessions)
	protected("GET /api/v1/projects/{id}/findings", s.handleGetProjectFindings)
	protected("POST /api/v1/projects/{id}/findings", s.handleUpsertProjectFinding)
	protected("DELETE /api/v1/projects/{id}/findings/{key}", s.handleDeleteProjectFinding)

	// Git
	protected("GET /api/v1/projects/{id}/git/commits", s.handleListGitCommits)
	protected("GET /api/v1/projects/{id}/git/commits/{hash}", s.handleGetGitCommitDetail)

	// Agent definitions (nested under workflows)
	protected("GET /api/v1/workflows/{wid}/agents", s.handleListAgentDefs)
	protected("POST /api/v1/workflows/{wid}/agents", s.handleCreateAgentDef)
	protected("GET /api/v1/workflows/{wid}/agents/{id}", s.handleGetAgentDef)
	protected("PATCH /api/v1/workflows/{wid}/agents/{id}", s.handleUpdateAgentDef)
	protected("DELETE /api/v1/workflows/{wid}/agents/{id}", s.handleDeleteAgentDef)

	// System agent definitions (global) — writes are admin-only
	protected("GET /api/v1/system-agents", s.handleListSystemAgentDefs)
	admin("POST /api/v1/system-agents", s.handleCreateSystemAgentDef)
	protected("GET /api/v1/system-agents/{id}", s.handleGetSystemAgentDef)
	admin("PATCH /api/v1/system-agents/{id}", s.handleUpdateSystemAgentDef)
	admin("DELETE /api/v1/system-agents/{id}", s.handleDeleteSystemAgentDef)

	// CLI models (global) — writes are admin-only
	protected("GET /api/v1/cli-models", s.handleListCLIModels)
	admin("POST /api/v1/cli-models", s.handleCreateCLIModel)
	protected("GET /api/v1/cli-models/{id}", s.handleGetCLIModel)
	admin("PATCH /api/v1/cli-models/{id}", s.handleUpdateCLIModel)
	admin("DELETE /api/v1/cli-models/{id}", s.handleDeleteCLIModel)
	protected("POST /api/v1/cli-models/{id}/test", s.handleTestCLIModel)

	// Notification channels (project-scoped)
	protected("GET /api/v1/notification-channels", s.handleListNotificationChannels)
	protected("POST /api/v1/notification-channels", s.handleCreateNotificationChannel)
	protected("GET /api/v1/notification-channels/{id}", s.handleGetNotificationChannel)
	protected("PATCH /api/v1/notification-channels/{id}", s.handleUpdateNotificationChannel)
	protected("DELETE /api/v1/notification-channels/{id}", s.handleDeleteNotificationChannel)
	protected("POST /api/v1/notification-channels/{id}/test", s.handleTestNotificationChannel)
	protected("GET /api/v1/notification-deliveries", s.handleListNotificationDeliveries)

	// Scheduled tasks (project-scoped) — writes are admin-only
	protected("GET /api/v1/scheduled-tasks", s.handleListScheduledTasks)
	admin("POST /api/v1/scheduled-tasks", s.handleCreateScheduledTask)
	protected("GET /api/v1/scheduled-tasks/{id}", s.handleGetScheduledTask)
	admin("PATCH /api/v1/scheduled-tasks/{id}", s.handleUpdateScheduledTask)
	admin("DELETE /api/v1/scheduled-tasks/{id}", s.handleDeleteScheduledTask)
	protected("GET /api/v1/scheduled-tasks/{id}/runs", s.handleListScheduleRuns)
	protected("POST /api/v1/scheduled-tasks/{id}/run-now", s.handleRunScheduledTaskNow)

	// Workflow chain definitions (project-scoped) — writes are admin-only
	protected("GET /api/v1/workflow-chains", s.handleListWorkflowChains)
	admin("POST /api/v1/workflow-chains", s.handleCreateWorkflowChain)
	protected("GET /api/v1/workflow-chains/{id}", s.handleGetWorkflowChain)
	admin("PATCH /api/v1/workflow-chains/{id}", s.handleUpdateWorkflowChain)
	admin("DELETE /api/v1/workflow-chains/{id}", s.handleDeleteWorkflowChain)
	admin("POST /api/v1/workflow-chains/{id}/steps", s.handleAppendChainStep)
	admin("PATCH /api/v1/workflow-chains/{id}/steps/{stepId}", s.handleUpdateChainStep)
	admin("DELETE /api/v1/workflow-chains/{id}/steps/{stepId}", s.handleDeleteChainStep)
	admin("POST /api/v1/workflow-chains/{id}/steps/reorder", s.handleReorderChainSteps)

	// Workflow chain runs (project-scoped) — cancel is admin-only
	protected("GET /api/v1/workflow-chains/{id}/runs", s.handleListChainRuns)
	protected("POST /api/v1/workflow-chains/{id}/runs", s.handleStartChainRun)
	protected("GET /api/v1/workflow-chains/{id}/runs/{runId}", s.handleGetChainRun)
	admin("POST /api/v1/workflow-chains/{id}/runs/{runId}/cancel", s.handleCancelChainRun)

	// Default templates (global) — writes are admin-only
	protected("GET /api/v1/default-templates", s.handleListDefaultTemplates)
	admin("POST /api/v1/default-templates", s.handleCreateDefaultTemplate)
	protected("GET /api/v1/default-templates/{id}", s.handleGetDefaultTemplate)
	admin("PATCH /api/v1/default-templates/{id}", s.handleUpdateDefaultTemplate)
	admin("DELETE /api/v1/default-templates/{id}", s.handleDeleteDefaultTemplate)
	admin("POST /api/v1/default-templates/{id}/restore", s.handleRestoreDefaultTemplate)

	// Global settings — GET is protected, PATCH is admin-only
	protected("GET /api/v1/settings", s.handleGetGlobalSettings)
	admin("PATCH /api/v1/settings", s.handlePatchGlobalSettings)

	// Safety hook check
	protected("POST /api/v1/safety-hook/check", s.handleCheckSafetyHook)

	// Agent sessions
	protected("GET /api/v1/tickets/{id}/agents", s.handleGetAgentSessions)
	protected("GET /api/v1/agents/running", s.handleGetRunningAgents)
	protected("GET /api/v1/agents/recent", s.handleGetRecentAgents)
	protected("GET /api/v1/sessions/{id}/messages", s.handleGetSessionMessages)
	protected("GET /api/v1/sessions/{id}/prompt", s.handleGetSessionPrompt)

	// Dependencies
	protected("GET /api/v1/tickets/{id}/dependencies", s.handleGetDependencies)
	protected("POST /api/v1/dependencies", s.handleAddDependency)
	protected("DELETE /api/v1/dependencies", s.handleRemoveDependency)

	// Chain executions
	protected("GET /api/v1/chains", s.handleListChains)
	protected("POST /api/v1/chains", s.handleCreateChain)
	protected("POST /api/v1/chains/preview", s.handlePreviewChain)
	protected("GET /api/v1/chains/{id}", s.handleGetChain)
	protected("PATCH /api/v1/chains/{id}", s.handleUpdateChain)
	protected("POST /api/v1/chains/{id}/start", s.handleStartChain)
	protected("POST /api/v1/chains/{id}/cancel", s.handleCancelChain)
	protected("DELETE /api/v1/chains/{id}", s.handleDeleteChain)
	protected("POST /api/v1/chains/{id}/append", s.handleAppendToChain)
	protected("POST /api/v1/chains/{id}/remove-items", s.handleRemoveFromChain)

	if s.apiMode {
		// Tool definitions (global; only in --mode=api) — writes are admin-only
		protected("GET /api/v1/tool-definitions", s.handleListToolDefinitions)
		admin("POST /api/v1/tool-definitions", s.handleCreateToolDefinition)
		protected("GET /api/v1/tool-definitions/{id}", s.handleGetToolDefinition)
		admin("PUT /api/v1/tool-definitions/{id}", s.handleUpdateToolDefinition)
		admin("DELETE /api/v1/tool-definitions/{id}", s.handleDeleteToolDefinition)

		// API credentials (global; only in --mode=api) — writes are admin-only
		protected("GET /api/v1/api-credentials", s.handleListAPICredentials)
		admin("POST /api/v1/api-credentials", s.handleCreateAPICredential)
		protected("GET /api/v1/api-credentials/{id}", s.handleGetAPICredential)
		admin("PUT /api/v1/api-credentials/{id}", s.handleUpdateAPICredential)
		admin("DELETE /api/v1/api-credentials/{id}", s.handleDeleteAPICredential)

		// review items (project-scoped; only in --mode=api)
		protected("GET /api/v1/review", s.handleListReviews)
		protected("POST /api/v1/review", s.handleCreateReview)
		protected("GET /api/v1/review/{id}", s.handleGetReview)
		protected("PATCH /api/v1/review/{id}", s.handlePatchReview)
		protected("POST /api/v1/review/{id}/approve", s.handleApproveReview)
		protected("POST /api/v1/review/{id}/reject", s.handleRejectReview)

		// config editor (project-scoped; only in --mode=api)
		protected("GET /api/v1/config-files", s.handleListConfigFiles)
		protected("GET /api/v1/config-files/content/{file...}", s.handleGetConfigFile)
		protected("PUT /api/v1/config-files/content/{file...}", s.handlePutConfigFile)
		protected("GET /api/v1/config-files/history/{file...}", s.handleGetConfigHistory)
		protected("POST /api/v1/config-files/rollback/{file...}", s.handleRollbackConfig)

		// insights (project-scoped; only in --mode=api)
		protected("GET /api/v1/insights/summary", s.handleInsightsSummary)
		protected("GET /api/v1/insights/edit-rate", s.handleInsightsEditRate)
		protected("GET /api/v1/insights/throughput", s.handleInsightsThroughput)
	}

	// Errors
	protected("GET /api/v1/errors", s.handleListErrors)

	// Search
	protected("GET /api/v1/search", s.handleSearch)

	// Status/Dashboard
	protected("GET /api/v1/status", s.handleStatus)
	protected("GET /api/v1/daily-stats", s.handleGetDailyStats)

	// Embedded UI (SPA catch-all — no auth, serves login page too)
	if uiFS, err := static.DistFS(); err == nil {
		if h := spaHandler(uiFS); h != nil {
			mux.Handle("/", h)
		}
	}
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
