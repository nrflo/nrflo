package api

import (
	"fmt"
	"net/http"

	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/types"
	"be/internal/ws"
)

// handlePreviewChain returns expanded tickets, dependency map, and auto-added tickets.
// POST /api/v1/chains/preview
func (s *Server) handlePreviewChain(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header or project query param required")
		return
	}

	var req types.ChainPreviewRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	pool, err := db.NewPool(s.dataPath, db.DefaultPoolConfig())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	defer pool.Close()

	chainSvc := service.NewChainService(pool, s.clock)
	resp, err := chainSvc.PreviewChain(projectID, &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleListChains lists chain executions for a project.
// GET /api/v1/chains?status=
func (s *Server) handleListChains(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header or project query param required")
		return
	}

	pool, err := db.NewPool(s.dataPath, db.DefaultPoolConfig())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	defer pool.Close()

	status := r.URL.Query().Get("status")
	epicTicketID := r.URL.Query().Get("epic_ticket_id")
	chainRepo := repo.NewChainRepo(pool, s.clock)
	chains, err := chainRepo.List(projectID, status, epicTicketID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if chains == nil {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}
	writeJSON(w, http.StatusOK, chains)
}

// handleGetChain returns a chain execution with its items.
// GET /api/v1/chains/{id}
func (s *Server) handleGetChain(w http.ResponseWriter, r *http.Request) {
	chainID := extractID(r)
	if chainID == "" {
		writeError(w, http.StatusBadRequest, "chain ID required")
		return
	}

	pool, err := db.NewPool(s.dataPath, db.DefaultPoolConfig())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	defer pool.Close()

	chainSvc := service.NewChainService(pool, s.clock)
	chain, err := chainSvc.GetChainWithItems(chainID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, chain)
}

// handleCreateChain creates a new chain execution in pending state.
// POST /api/v1/chains
func (s *Server) handleCreateChain(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header or project query param required")
		return
	}

	var req types.ChainCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	pool, err := db.NewPool(s.dataPath, db.DefaultPoolConfig())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	defer pool.Close()

	chainSvc := service.NewChainService(pool, s.clock)
	chain, err := chainSvc.CreateChain(projectID, &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, chain)
}

// handleUpdateChain updates a pending chain execution.
// PATCH /api/v1/chains/{id}
func (s *Server) handleUpdateChain(w http.ResponseWriter, r *http.Request) {
	chainID := extractID(r)
	if chainID == "" {
		writeError(w, http.StatusBadRequest, "chain ID required")
		return
	}

	var req types.ChainUpdateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	pool, err := db.NewPool(s.dataPath, db.DefaultPoolConfig())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	defer pool.Close()

	chainSvc := service.NewChainService(pool, s.clock)
	chain, err := chainSvc.UpdateChain(chainID, &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, chain)
}

// handleStartChain starts sequential execution of a chain.
// POST /api/v1/chains/{id}/start
func (s *Server) handleStartChain(w http.ResponseWriter, r *http.Request) {
	chainID := extractID(r)
	if chainID == "" {
		writeError(w, http.StatusBadRequest, "chain ID required")
		return
	}

	if s.chainRunner == nil {
		writeError(w, http.StatusServiceUnavailable, "chain runner not available")
		return
	}

	err := s.chainRunner.Start(r.Context(), chainID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "started", "chain_id": chainID})
}

// handleCancelChain cancels a running or pending chain.
// POST /api/v1/chains/{id}/cancel
func (s *Server) handleCancelChain(w http.ResponseWriter, r *http.Request) {
	chainID := extractID(r)
	if chainID == "" {
		writeError(w, http.StatusBadRequest, "chain ID required")
		return
	}

	if s.chainRunner == nil {
		writeError(w, http.StatusServiceUnavailable, "chain runner not available")
		return
	}

	err := s.chainRunner.Cancel(chainID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "canceled", "chain_id": chainID})
}

// handleAppendToChain appends tickets to a running chain.
// POST /api/v1/chains/{id}/append
func (s *Server) handleAppendToChain(w http.ResponseWriter, r *http.Request) {
	chainID := extractID(r)
	if chainID == "" {
		writeError(w, http.StatusBadRequest, "chain ID required")
		return
	}

	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header or project query param required")
		return
	}

	var req types.ChainAppendRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	pool, err := db.NewPool(s.dataPath, db.DefaultPoolConfig())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	defer pool.Close()

	chainSvc := service.NewChainService(pool, s.clock)
	chain, err := chainSvc.AppendToChain(chainID, &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if s.wsHub != nil {
		event := ws.NewEvent("chain.updated", projectID, "", "", map[string]interface{}{
			"chain_id": chainID,
			"action":   "append",
		})
		s.wsHub.Broadcast(event)
	}

	writeJSON(w, http.StatusOK, chain)
}

// handleRunEpicWorkflow creates a chain from an epic's child tickets and optionally starts it.
// POST /api/v1/tickets/{id}/workflow/run-epic
func (s *Server) handleRunEpicWorkflow(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header or project query param required")
		return
	}

	ticketID := extractID(r)
	if ticketID == "" {
		writeError(w, http.StatusBadRequest, "ticket ID required")
		return
	}

	var body struct {
		WorkflowName string `json:"workflow_name"`
		Start        bool   `json:"start"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.WorkflowName == "" {
		writeError(w, http.StatusBadRequest, "workflow_name is required")
		return
	}

	// Look up the ticket and validate it's an epic
	database, err := db.Open(s.dataPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	defer database.Close()

	ticketRepo := repo.NewTicketRepo(database, s.clock)
	ticket, err := ticketRepo.Get(projectID, ticketID)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("ticket not found: %s", ticketID))
		return
	}
	if ticket.IssueType != model.IssueTypeEpic {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("ticket %s is not an epic (type: %s)", ticketID, ticket.IssueType))
		return
	}

	// Get non-closed children
	children, err := ticketRepo.ListByParent(projectID, ticketID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list child tickets")
		return
	}
	if len(children) == 0 {
		writeError(w, http.StatusBadRequest, "epic has no child tickets")
		return
	}

	childIDs := make([]string, len(children))
	for i, c := range children {
		childIDs[i] = c.ID
	}

	// Create chain via ChainService
	pool, err := db.NewPool(s.dataPath, db.DefaultPoolConfig())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	defer pool.Close()

	chainSvc := service.NewChainService(pool, s.clock)
	chain, err := chainSvc.CreateChain(projectID, &types.ChainCreateRequest{
		Name:         fmt.Sprintf("Epic: %s", ticket.Title),
		WorkflowName: body.WorkflowName,
		EpicTicketID: ticketID,
		TicketIDs:    childIDs,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Optionally start the chain
	if body.Start {
		if s.chainRunner == nil {
			writeError(w, http.StatusServiceUnavailable, "chain runner not available")
			return
		}
		if err := s.chainRunner.Start(r.Context(), chain.ID); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("chain created but failed to start: %s", err.Error()))
			return
		}
	}

	writeJSON(w, http.StatusCreated, chain)
}
