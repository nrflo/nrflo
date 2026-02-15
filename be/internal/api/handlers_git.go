package api

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"

	"be/internal/db"
	"be/internal/service"
)

var commitHashRegex = regexp.MustCompile(`^[0-9a-fA-F]{4,40}$`)

// handleListGitCommits returns paginated git commit history for a project.
// GET /api/v1/projects/{id}/git/commits?page=1&per_page=20
func (s *Server) handleListGitCommits(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project ID required")
		return
	}

	page := 1
	perPage := 20
	if v := r.URL.Query().Get("page"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			page = p
		}
	}
	if v := r.URL.Query().Get("per_page"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			if p > 100 {
				p = 100
			}
			perPage = p
		}
	}

	pool, err := db.NewPool(s.dataPath, db.DefaultPoolConfig())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	defer pool.Close()

	project, err := service.NewProjectService(pool, s.clock).Get(projectID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	if !project.RootPath.Valid || project.RootPath.String == "" {
		writeError(w, http.StatusBadRequest, "project has no root_path configured")
		return
	}

	branch := "main"
	if project.DefaultBranch.Valid && project.DefaultBranch.String != "" {
		branch = project.DefaultBranch.String
	}

	gitSvc := &service.GitService{}
	commits, total, err := gitSvc.ListCommits(project.RootPath.String, branch, page, perPage)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"commits":  commits,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

// handleGetGitCommitDetail returns details for a single commit.
// GET /api/v1/projects/{id}/git/commits/{hash}
func (s *Server) handleGetGitCommitDetail(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project ID required")
		return
	}

	hash := r.PathValue("hash")
	if !commitHashRegex.MatchString(hash) {
		writeError(w, http.StatusBadRequest, "invalid commit hash format")
		return
	}

	pool, err := db.NewPool(s.dataPath, db.DefaultPoolConfig())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	defer pool.Close()

	project, err := service.NewProjectService(pool, s.clock).Get(projectID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	if !project.RootPath.Valid || project.RootPath.String == "" {
		writeError(w, http.StatusBadRequest, "project has no root_path configured")
		return
	}

	gitSvc := &service.GitService{}
	detail, err := gitSvc.GetCommitDetail(project.RootPath.String, hash)
	if err != nil {
		if errors.Is(err, service.ErrCommitNotFound) {
			writeError(w, http.StatusNotFound, "commit not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"commit": detail,
	})
}
