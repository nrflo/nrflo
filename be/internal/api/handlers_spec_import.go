package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"be/internal/model"
	"be/internal/orchestrator"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spec_import"
	"be/internal/ws"
)

// rawFindingsToInterface converts map[string]json.RawMessage to map[string]interface{}.
func rawFindingsToInterface(raw map[string]json.RawMessage) map[string]interface{} {
	m := make(map[string]interface{}, len(raw))
	for k, v := range raw {
		var parsed interface{}
		json.Unmarshal(v, &parsed) //nolint:errcheck
		m[k] = parsed
	}
	return m
}

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

	refsJSON, _ := json.Marshal(spec.AttachedRefs)
	// SeedFindings are inserted into the findings table at scope=workflow_instance.
	// The raw spec text is also passed via RunRequest.Instructions so it is
	// auto-prepended to the agent prompt as the User Instructions block.
	seed := map[string]string{
		"_spec_source":        body.Source,
		"_spec_attached_refs": string(refsJSON),
		"raw_spec":            spec.RawText,
	}

	var instanceID string
	status := "running"
	if s.orchestrator != nil {
		result, startErr := s.orchestrator.Start(r.Context(), orchestrator.RunRequest{
			ProjectID:    projectID,
			WorkflowName: specImportWorkflowID,
			ScopeType:    "project",
			SeedFindings: seed,
			Instructions: spec.RawText,
		})
		if startErr != nil || result == nil {
			writeError(w, http.StatusInternalServerError, "failed to start spec import workflow: "+startErr.Error())
			return
		}
		instanceID = result.InstanceID
	} else {
		// Test path / orchestrator unavailable: persist raw spec directly so
		// the GET preview returns the fallback (un-normalized) fields.
		wfiRepo := repo.NewWorkflowInstanceRepo(s.pool, s.clock)
		instanceID = uuid.New().String()
		wfi := &model.WorkflowInstance{
			ID:         instanceID,
			ProjectID:  projectID,
			WorkflowID: specImportWorkflowID,
			ScopeType:  "project",
			Status:     model.WorkflowInstanceActive,
		}
		if err := wfiRepo.Create(wfi); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create import session: "+err.Error())
			return
		}
		findingRepo := repo.NewFindingRepo(s.pool, s.clock)
		denorm := repo.Denorm{ProjectID: projectID, WorkflowInstanceID: instanceID}
		actor := repo.Actor{Source: "system"}
		for key, val := range map[string]string{
			"_spec_source":        body.Source,
			"_spec_attached_refs": string(refsJSON),
			"raw_spec":            spec.RawText,
		} {
			v, _ := json.Marshal(val)
			findingRepo.Upsert("workflow_instance", instanceID, key, v, denorm, actor) //nolint:errcheck
		}
		status = "ready"
	}

	if s.wsHub != nil {
		s.wsHub.Broadcast(ws.NewEvent(ws.EventSpecImportStarted, projectID, "", specImportWorkflowID, map[string]interface{}{
			"instance_id": instanceID,
		}))
		if status == "ready" {
			s.wsHub.Broadcast(ws.NewEvent(ws.EventSpecImportReady, projectID, "", specImportWorkflowID, map[string]interface{}{
				"instance_id": instanceID,
			}))
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"instance_id": instanceID,
		"status":      status,
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

	findingRepo := repo.NewFindingRepo(s.pool, s.clock)
	rawFindings, _ := findingRepo.GetOwn("workflow_instance", instanceID)
	findings := rawFindingsToInterface(rawFindings)
	rawSpec, _ := findings["raw_spec"].(string)
	if rawSpec == "" {
		rawSpec, _ = findings["_raw_spec"].(string)
	}

	resp := map[string]interface{}{
		"instance_id": instanceID,
		"status":      "pending",
	}
	if rawSpec == "" {
		writeJSON(w, http.StatusOK, resp)
		return
	}

	switch wfi.Status {
	case model.WorkflowInstanceFailed:
		resp["status"] = "failed"
	case model.WorkflowInstanceCompleted, model.WorkflowInstanceProjectCompleted, model.WorkflowInstanceActive:
		// Active is still reported as "ready" when raw_spec is set in the
		// fallback (no-orchestrator) path. The normalizer agent fills in
		// import_* findings before the workflow flips to project_completed;
		// either way, what matters is whether title is present.
		resp["status"] = "ready"
	default:
		resp["status"] = "running"
	}

	title, _ := findings["import_ticket_title"].(string)
	description, _ := findings["import_ticket_description"].(string)
	instructions, _ := findings["import_workflow_instructions"].(string)
	if title == "" && description == "" {
		// Normalizer hasn't produced structured fields yet (or it's the
		// fallback path). Derive title/description from raw_spec so the
		// preview always has something to render.
		title, description = splitRawSpec(rawSpec)
	}
	if instructions == "" {
		instructions = rawSpec
	}

	// Prefer the normalizer's merged attached_refs list when present.
	refs := buildAttachedRefs(findings)

	resp["title"] = title
	resp["description"] = description
	resp["instructions"] = instructions
	resp["raw_spec"] = rawSpec
	resp["attached_refs"] = refs
	resp["source"] = findings["_spec_source"]
	if v, _ := findings["import_suggested_workflow"].(string); v != "" {
		resp["suggested_workflow"] = v
	}
	if v, _ := findings["import_priority"].(string); v != "" {
		resp["priority"] = v
	}
	if v, _ := findings["import_issue_type"].(string); v != "" {
		resp["issue_type"] = v
	}

	writeJSON(w, http.StatusOK, resp)
}

// buildAttachedRefs returns refs in the cleaned `{kind,url,label?}` shape.
// Prefers `import_attached_refs` (normalizer output) over `_spec_attached_refs`.
func buildAttachedRefs(findings map[string]interface{}) []map[string]interface{} {
	src, _ := findings["import_attached_refs"].(string)
	if src == "" {
		src, _ = findings["_spec_attached_refs"].(string)
	}
	if src == "" {
		return []map[string]interface{}{}
	}
	// Try the structured shape first, then fall back to a permissive parse.
	var stored []model.TicketRef
	if err := json.Unmarshal([]byte(src), &stored); err == nil && len(stored) > 0 {
		out := make([]map[string]interface{}, 0, len(stored))
		for _, r := range stored {
			item := map[string]interface{}{"url": r.URL, "kind": r.Kind}
			if r.Label.Valid {
				item["label"] = r.Label.String
			}
			out = append(out, item)
		}
		return out
	}
	var generic []map[string]interface{}
	json.Unmarshal([]byte(src), &generic) //nolint:errcheck
	if generic == nil {
		return []map[string]interface{}{}
	}
	return generic
}

// splitRawSpec extracts a title (first `# heading` or first non-empty line)
// and returns the remaining body as the description.
func splitRawSpec(raw string) (title, description string) {
	lines := strings.SplitN(strings.TrimSpace(raw), "\n", 2)
	if len(lines) == 0 {
		return "", ""
	}
	first := strings.TrimSpace(lines[0])
	title = strings.TrimSpace(strings.TrimPrefix(first, "#"))
	if len(lines) == 2 {
		description = strings.TrimSpace(lines[1])
	}
	return title, description
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
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project ID required (X-Project header)")
		return
	}
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
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project ID required (X-Project header)")
		return
	}
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
