package api

import (
	"context"
	"net/http"

	"be/internal/db"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/types"
)

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
	chainRepo := repo.NewChainRepo(pool)
	chains, err := chainRepo.List(projectID, status)
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

	chainSvc := service.NewChainService(pool)
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

	chainSvc := service.NewChainService(pool)
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

	chainSvc := service.NewChainService(pool)
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

	err := s.chainRunner.Start(context.Background(), chainID)
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
