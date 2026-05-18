package api

import (
	"fmt"
	"net/http"
	"strings"

	"be/internal/repo"
	"be/internal/service"
	"be/internal/types"
)

func (s *Server) workflowExportService() *service.WorkflowExportService {
	wfSvc := service.NewWorkflowService(s.pool, s.clock)
	cliModelSvc := service.NewCLIModelService(s.pool, s.clock)
	pythonScriptRepo := repo.NewPythonScriptRepo(s.pool, s.clock)
	agentDefSvc := service.NewAgentDefinitionService(s.pool, s.clock, cliModelSvc, pythonScriptRepo)
	layerPolicySvc := service.NewWorkflowLayerPolicyService(s.pool, s.clock)
	notifySvc := service.NewNotificationService(s.pool, s.clock, s.wsHub, s.notifyWaker, wfSvc)
	pythonScriptSvc := service.NewPythonScriptService(s.pool, s.clock)
	return service.NewWorkflowExportService(s.pool, s.clock, wfSvc, agentDefSvc, layerPolicySvc, notifySvc, pythonScriptSvc)
}

func (s *Server) handleExportWorkflow(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	id := r.PathValue("id")
	svc := s.workflowExportService()
	bundle, err := svc.Export(projectID, []string{id})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	filename := fmt.Sprintf("nrflo-workflows-%s-%s.json", projectID, s.clock.Now().UTC().Format("20060102-150405"))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	writeJSON(w, http.StatusOK, bundle)
}

func (s *Server) handleExportAllWorkflows(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	svc := s.workflowExportService()
	bundle, err := svc.Export(projectID, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	filename := fmt.Sprintf("nrflo-workflows-%s-%s.json", projectID, s.clock.Now().UTC().Format("20060102-150405"))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	writeJSON(w, http.StatusOK, bundle)
}

func (s *Server) handleImportCheck(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	var bundle types.WorkflowBundle
	if err := readJSON(r, &bundle); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	svc := s.workflowExportService()
	conflicts, err := svc.CheckImport(projectID, &bundle)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, conflicts)
}

func (s *Server) handleImportWorkflow(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	var req types.ImportRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	svc := s.workflowExportService()
	result, err := svc.Import(projectID, &req)
	if err != nil {
		if strings.Contains(err.Error(), "invalid action") {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}
