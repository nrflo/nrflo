package api

import (
	"net/http"

	"be/internal/model"
)

// AddDependencyRequest represents the request body for adding a dependency
type AddDependencyRequest struct {
	IssueID     string `json:"issue_id"`
	DependsOnID string `json:"depends_on_id"`
}

// handleAddDependency adds a dependency between tickets
func (s *Server) handleAddDependency(w http.ResponseWriter, r *http.Request) {
	_, depRepo, database, err := s.getRepos(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer database.Close()

	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	var req AddDependencyRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.IssueID == "" || req.DependsOnID == "" {
		writeError(w, http.StatusBadRequest, "issue_id and depends_on_id are required")
		return
	}

	dep := &model.Dependency{
		ProjectID:   projectID,
		IssueID:     req.IssueID,
		DependsOnID: req.DependsOnID,
		Type:        "blocks",
		CreatedBy:   "api",
	}

	if err := depRepo.Create(dep); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"message": "dependency added"})
}

// RemoveDependencyRequest represents the request body for removing a dependency
type RemoveDependencyRequest struct {
	IssueID     string `json:"issue_id"`
	DependsOnID string `json:"depends_on_id"`
}

// handleRemoveDependency removes a dependency between tickets
func (s *Server) handleRemoveDependency(w http.ResponseWriter, r *http.Request) {
	_, depRepo, database, err := s.getRepos(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer database.Close()

	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	var req RemoveDependencyRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.IssueID == "" || req.DependsOnID == "" {
		writeError(w, http.StatusBadRequest, "issue_id and depends_on_id are required")
		return
	}

	if err := depRepo.Delete(projectID, req.IssueID, req.DependsOnID); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "dependency removed"})
}

// handleGetDependencies returns dependencies for a ticket
func (s *Server) handleGetDependencies(w http.ResponseWriter, r *http.Request) {
	_, depRepo, database, err := s.getRepos(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer database.Close()

	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	id := extractID(r)

	blockers, err := depRepo.GetBlockers(projectID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	blocked, err := depRepo.GetBlocked(projectID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if blockers == nil {
		blockers = []*model.Dependency{}
	}
	if blocked == nil {
		blocked = []*model.Dependency{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"blockers": blockers,
		"blocks":   blocked,
	})
}
