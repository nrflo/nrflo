package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spec_import"
	"be/internal/ws"
)

const specImportWorkflowID = "__spec_import__"

// specImportAdapter is a minimal interface covering both Fetch and the source query.
type specImportAdapter interface {
	Source() spec_import.Source
	Fetch(ctx context.Context, in spec_import.Input) (spec_import.FetchedSpec, error)
}

// gitHubSearcher is implemented by spec_import.GitHubAdapter.
type gitHubSearcher interface {
	Search(ctx context.Context, q, ownerRepo string, env map[string]string) ([]spec_import.GitHubIssueSummary, error)
}

// jiraSearcher is implemented by spec_import.JiraAdapter.
type jiraSearcher interface {
	Search(ctx context.Context, q string, env map[string]string) ([]spec_import.JiraIssueSummary, error)
}

// resolveSpecImportAdapter returns the adapter for src, using the injectable override when set.
func (s *Server) resolveSpecImportAdapter(src spec_import.Source) (spec_import.Adapter, error) {
	if s.specImportAdapterFunc != nil {
		raw, err := s.specImportAdapterFunc(string(src))
		if err != nil {
			return nil, err
		}
		a, ok := raw.(spec_import.Adapter)
		if !ok {
			return nil, errors.New("specImportAdapterFunc returned non-Adapter value")
		}
		return a, nil
	}
	return spec_import.ResolveAdapter(src)
}

// loadProjectEnvMap loads project env vars into a map for adapter injection.
func (s *Server) loadProjectEnvMap(projectID string) map[string]string {
	env := make(map[string]string)
	if projectID == "" || s.pool == nil {
		return env
	}
	svc := service.NewProjectEnvVarService(s.pool, s.clock)
	vars, err := svc.List(projectID)
	if err != nil {
		return env
	}
	for _, v := range vars {
		env[v.Name] = v.Value
	}
	return env
}

// handleStartSpecImport fetches a spec from an external source and creates a spec-import session.
// POST /api/v1/import/spec
func (s *Server) handleStartSpecImport(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project ID required (X-Project header)")
		return
	}

	var body struct {
		Source string            `json:"source"`
		Body   string            `json:"body"`
		Env    map[string]string `json:"env"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Source == "" {
		writeError(w, http.StatusBadRequest, "source is required")
		return
	}

	adapter, err := s.resolveSpecImportAdapter(spec_import.Source(body.Source))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Merge project env with request-provided overrides.
	env := s.loadProjectEnvMap(projectID)
	for k, v := range body.Env {
		env[k] = v
	}

	spec, err := adapter.Fetch(r.Context(), spec_import.Input{Body: body.Body, Env: env})
	if err != nil {
		var missingEnv spec_import.MissingEnvError
		if errors.As(err, &missingEnv) {
			writeJSON(w, http.StatusPreconditionFailed, map[string]interface{}{
				"error":   "missing_env",
				"missing": missingEnv.Missing,
			})
			return
		}
		// Network / adapter failure: log + 502.
		errRepo := repo.NewErrorLogRepo(s.pool, s.clock)
		_ = errRepo.Insert(&model.ErrorLog{
			ID:         uuid.New().String(),
			ProjectID:  projectID,
			ErrorType:  model.ErrorTypeSystem,
			InstanceID: "",
			Message:    err.Error(),
		})
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "adapter_failure"})
		return
	}

	// Create workflow instance for this import session.
	wfiRepo := repo.NewWorkflowInstanceRepo(s.pool, s.clock)
	instanceID := uuid.New().String()
	wfi := &model.WorkflowInstance{
		ID:         instanceID,
		ProjectID:  projectID,
		WorkflowID: specImportWorkflowID,
		ScopeType:  "project",
		Status:     model.WorkflowInstanceActive,
	}
	refsJSON, _ := json.Marshal(spec.AttachedRefs)
	wfi.SetFindings(map[string]interface{}{
		"_spec_source":        body.Source,
		"_raw_spec":           spec.RawText,
		"_spec_attached_refs": string(refsJSON),
	})
	if err := wfiRepo.Create(wfi); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create import session: "+err.Error())
		return
	}

	if s.wsHub != nil {
		s.wsHub.Broadcast(ws.NewEvent(ws.EventSpecImportStarted, projectID, "", specImportWorkflowID, map[string]interface{}{
			"instance_id": instanceID,
		}))
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"instance_id": instanceID,
		"status":      "running",
	})
}

// handleGetSpecImport returns status and spec preview for a spec-import session.
// GET /api/v1/import/spec/{instance_id}
func (s *Server) handleGetSpecImport(w http.ResponseWriter, r *http.Request) {
	instanceID := r.PathValue("instance_id")
	if instanceID == "" {
		writeError(w, http.StatusBadRequest, "instance ID required")
		return
	}

	wfiRepo := repo.NewWorkflowInstanceRepo(s.pool, s.clock)
	wfi, err := wfiRepo.Get(instanceID)
	if err != nil || wfi.WorkflowID != specImportWorkflowID {
		writeError(w, http.StatusNotFound, "import session not found")
		return
	}

	findings := wfi.GetFindings()
	rawSpec, _ := findings["_raw_spec"].(string)

	var preview interface{} // null when pending
	status := "pending"
	if rawSpec != "" {
		status = "completed"
		attachedRefsStr, _ := findings["_spec_attached_refs"].(string)
		var refs []interface{}
		if attachedRefsStr != "" {
			json.Unmarshal([]byte(attachedRefsStr), &refs) //nolint:errcheck
		}
		preview = map[string]interface{}{
			"raw_spec":      rawSpec,
			"attached_refs": refs,
			"source":        findings["_spec_source"],
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"instance_id": instanceID,
		"status":      status,
		"preview":     preview,
	})
}

// handleGitHubSearch searches GitHub issues.
// GET /api/v1/import/github/search?q=...&repo=owner/repo
func (s *Server) handleGitHubSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "q parameter required")
		return
	}
	ownerRepo := r.URL.Query().Get("repo")
	projectID := getProjectID(r)
	env := s.loadProjectEnvMap(projectID)

	adapter, err := s.resolveSpecImportAdapter(spec_import.SourceGitHubIssue)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	searcher, ok := adapter.(gitHubSearcher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "github adapter does not support search")
		return
	}
	results, err := searcher.Search(r.Context(), q, ownerRepo, env)
	if err != nil {
		var missingEnv spec_import.MissingEnvError
		if errors.As(err, &missingEnv) {
			writeJSON(w, http.StatusPreconditionFailed, map[string]interface{}{
				"error":   "missing_env",
				"missing": missingEnv.Missing,
			})
			return
		}
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"results": results})
}

// handleJiraSearch searches Jira issues.
// GET /api/v1/import/jira/search?q=...
func (s *Server) handleJiraSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "q parameter required")
		return
	}
	projectID := getProjectID(r)
	env := s.loadProjectEnvMap(projectID)

	adapter, err := s.resolveSpecImportAdapter(spec_import.SourceJira)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	searcher, ok := adapter.(jiraSearcher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "jira adapter does not support search")
		return
	}
	results, err := searcher.Search(r.Context(), q, env)
	if err != nil {
		var missingEnv spec_import.MissingEnvError
		if errors.As(err, &missingEnv) {
			writeJSON(w, http.StatusPreconditionFailed, map[string]interface{}{
				"error":   "missing_env",
				"missing": missingEnv.Missing,
			})
			return
		}
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"results": results})
}

// handleEnvVarCatalog returns the list of env vars consumed by spec-import adapters.
// GET /api/v1/import/env-var-catalog
func (s *Server) handleEnvVarCatalog(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"vars": spec_import.Catalog})
}
