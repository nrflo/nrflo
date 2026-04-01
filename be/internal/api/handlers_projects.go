package api

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/spawner"
)

// loadSafetyHook loads the claude_safety_hook config for a project and sets it on the model.
func (s *Server) loadSafetyHook(p *model.Project) {
	val, err := s.pool.GetProjectConfig(p.ID, "claude_safety_hook")
	if err == nil {
		p.ClaudeSafetyHook = val
	}
}

// handleListProjects returns all projects
func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projectRepo := s.projectRepo()

	projects, err := projectRepo.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if projects == nil {
		projects = []*model.Project{}
	}

	for _, p := range projects {
		s.loadSafetyHook(p)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"projects": projects,
	})
}

// CreateProjectRequest represents the request body for creating a project
type CreateProjectRequest struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	RootPath        string `json:"root_path,omitempty"`
	DefaultBranch   string `json:"default_branch,omitempty"`
	UseGitWorktrees    *bool  `json:"use_git_worktrees,omitempty"`
	UseDockerIsolation *bool  `json:"use_docker_isolation,omitempty"`
}

// handleCreateProject creates a new project
func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	projectRepo := s.projectRepo()

	var req CreateProjectRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	if req.Name == "" {
		req.Name = req.ID
	}

	project := &model.Project{
		ID:   req.ID,
		Name: req.Name,
	}

	if req.RootPath != "" {
		project.RootPath = sql.NullString{String: req.RootPath, Valid: true}
	}
	if req.DefaultBranch != "" {
		project.DefaultBranch = sql.NullString{String: req.DefaultBranch, Valid: true}
	}
	if req.UseGitWorktrees != nil && *req.UseGitWorktrees {
		project.UseGitWorktrees = true
	}
	if req.UseDockerIsolation != nil && *req.UseDockerIsolation {
		project.UseDockerIsolation = true
	}

	if err := projectRepo.Create(project); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	created, err := projectRepo.Get(req.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

// handleGetProject returns a single project by ID
func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	projectRepo := s.projectRepo()

	id := extractID(r)
	project, err := projectRepo.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	s.loadSafetyHook(project)

	writeJSON(w, http.StatusOK, project)
}

// handleDeleteProject deletes a project
func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	projectRepo := s.projectRepo()

	id := extractID(r)
	if err := projectRepo.Delete(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "project deleted"})
}

// UpdateProjectRequest represents the request body for updating a project
type UpdateProjectRequest struct {
	Name            *string `json:"name,omitempty"`
	RootPath        *string `json:"root_path,omitempty"`
	DefaultBranch   *string `json:"default_branch,omitempty"`
	UseGitWorktrees    *bool   `json:"use_git_worktrees,omitempty"`
	UseDockerIsolation *bool   `json:"use_docker_isolation,omitempty"`
	ClaudeSafetyHook   *string `json:"claude_safety_hook,omitempty"`
}

// handleUpdateProject updates a project
func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	projectRepo := s.projectRepo()

	id := extractID(r)

	var req UpdateProjectRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	fields := &repo.ProjectUpdateFields{
		Name:            req.Name,
		RootPath:        req.RootPath,
		DefaultBranch:   req.DefaultBranch,
		UseGitWorktrees:    req.UseGitWorktrees,
		UseDockerIsolation: req.UseDockerIsolation,
	}

	if err := projectRepo.Update(id, fields); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// Handle safety hook config (stored in config table, not projects table)
	if req.ClaudeSafetyHook != nil {
		hookVal := *req.ClaudeSafetyHook
		if hookVal != "" {
			var cfg spawner.SafetyHookConfig
			if err := json.Unmarshal([]byte(hookVal), &cfg); err != nil {
				writeError(w, http.StatusBadRequest, "invalid claude_safety_hook JSON: "+err.Error())
				return
			}
		}
		if err := s.pool.SetProjectConfig(id, "claude_safety_hook", hookVal); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save safety hook config: "+err.Error())
			return
		}
	}

	updated, err := projectRepo.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.loadSafetyHook(updated)

	writeJSON(w, http.StatusOK, updated)
}
