package api

import (
	"net/http"

	"be/internal/artifact"
	"be/internal/service"
)

type projectObserverResponse struct {
	SystemContext string `json:"system_context"`
	Provider      string `json:"provider"`
	Model         string `json:"model"`
}

type putProjectObserverRequest struct {
	SystemContext *string `json:"system_context,omitempty"`
	Provider      *string `json:"provider,omitempty"`
	Model         *string `json:"model,omitempty"`
}

func (s *Server) handleGetProjectObserver(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project id is required")
		return
	}
	svc := service.NewGlobalSettingsService(s.pool, s.clock)
	sysCtx, err := svc.GetObserverSystemContextForProject(projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	provider, err := svc.GetObserverProviderForProject(projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	model, err := svc.GetObserverModelForProject(projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, projectObserverResponse{
		SystemContext: sysCtx,
		Provider:      provider,
		Model:         model,
	})
}

func (s *Server) handlePutProjectObserver(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project id is required")
		return
	}
	var req putProjectObserverRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	svc := service.NewGlobalSettingsService(s.pool, s.clock)
	if req.SystemContext != nil {
		if err := svc.SetObserverSystemContextForProject(projectID, *req.SystemContext); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if req.Provider != nil {
		if err := svc.SetObserverProviderForProject(projectID, *req.Provider); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if req.Model != nil {
		if err := svc.SetObserverModelForProject(projectID, *req.Model); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	sysCtx, err := svc.GetObserverSystemContextForProject(projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	provider, err := svc.GetObserverProviderForProject(projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	model, err := svc.GetObserverModelForProject(projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, projectObserverResponse{
		SystemContext: sysCtx,
		Provider:      provider,
		Model:         model,
	})
}

type projectCleanupResponse struct {
	Enabled        bool `json:"enabled"`
	RetentionLimit int  `json:"retention_limit"`
}

type putProjectCleanupRequest struct {
	Enabled        bool `json:"enabled"`
	RetentionLimit *int `json:"retention_limit,omitempty"`
}

func (s *Server) handleGetProjectArtifactStorage(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project id is required")
		return
	}
	svc := service.NewGlobalSettingsService(s.pool, s.clock)
	cfg, err := svc.GetArtifactStorageRedacted(projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) handlePutProjectArtifactStorage(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project id is required")
		return
	}
	var cfg artifact.Config
	if err := readJSON(r, &cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	svc := service.NewGlobalSettingsService(s.pool, s.clock)
	if err := svc.SetArtifactStorage(projectID, cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := svc.GetArtifactStorageRedacted(projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleGetProjectCleanup(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project id is required")
		return
	}
	svc := service.NewGlobalSettingsService(s.pool, s.clock)
	enabled, err := svc.GetWorkflowCleanupEnabled(projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	limit, err := svc.GetSessionRetentionLimit(projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, projectCleanupResponse{
		Enabled:        enabled,
		RetentionLimit: limit,
	})
}

func (s *Server) handlePutProjectCleanup(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project id is required")
		return
	}
	var req putProjectCleanupRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	svc := service.NewGlobalSettingsService(s.pool, s.clock)
	if err := svc.SetWorkflowCleanupEnabled(projectID, req.Enabled); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if req.RetentionLimit != nil {
		if err := svc.SetSessionRetentionLimit(projectID, *req.RetentionLimit); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	enabled, err := svc.GetWorkflowCleanupEnabled(projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	limit, err := svc.GetSessionRetentionLimit(projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, projectCleanupResponse{
		Enabled:        enabled,
		RetentionLimit: limit,
	})
}
