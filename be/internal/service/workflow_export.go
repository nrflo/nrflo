package service

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/types"
)

// WorkflowExportService orchestrates workflow export and import operations.
type WorkflowExportService struct {
	pool            *db.Pool
	clk             clock.Clock
	workflowSvc     *WorkflowService
	agentDefSvc     *AgentDefinitionService
	layerPolicySvc  *WorkflowLayerPolicyService
	notifySvc       *NotificationService
	pythonScriptSvc *PythonScriptService
}

// NewWorkflowExportService creates a WorkflowExportService.
func NewWorkflowExportService(
	pool *db.Pool,
	clk clock.Clock,
	workflowSvc *WorkflowService,
	agentDefSvc *AgentDefinitionService,
	layerPolicySvc *WorkflowLayerPolicyService,
	notifySvc *NotificationService,
	pythonScriptSvc *PythonScriptService,
) *WorkflowExportService {
	return &WorkflowExportService{
		pool:            pool,
		clk:             clk,
		workflowSvc:     workflowSvc,
		agentDefSvc:     agentDefSvc,
		layerPolicySvc:  layerPolicySvc,
		notifySvc:       notifySvc,
		pythonScriptSvc: pythonScriptSvc,
	}
}

// Export builds a WorkflowBundle for the given workflow IDs.
// If workflowIDs is empty, all non-reserved workflows in the project are exported.
func (s *WorkflowExportService) Export(projectID string, workflowIDs []string) (*types.WorkflowBundle, error) {
	if len(workflowIDs) == 0 {
		defs, err := s.workflowSvc.ListWorkflowDefs(projectID)
		if err != nil {
			return nil, err
		}
		for id := range defs {
			workflowIDs = append(workflowIDs, id)
		}
		sort.Strings(workflowIDs)
	}

	bundle := &types.WorkflowBundle{
		Version:    "1.0",
		ExportedAt: s.clk.Now().UTC().Format(time.RFC3339),
		Workflows:  []types.WorkflowBundleEntry{},
	}

	seenScripts := map[string]bool{}

	for _, wfID := range workflowIDs {
		wf, err := s.fetchWorkflowModel(projectID, wfID)
		if err != nil {
			return nil, err
		}
		wf.ProjectID = ""

		agents, err := s.agentDefSvc.ListAgentDefs(projectID, wfID)
		if err != nil {
			return nil, err
		}
		for _, a := range agents {
			if a.PythonScriptID != nil {
				seenScripts[*a.PythonScriptID] = true
			}
			a.ProjectID = ""
			a.WorkflowID = ""
		}

		policies, err := s.layerPolicySvc.GetLayerPolicies(projectID, wfID)
		if err != nil {
			return nil, err
		}

		channels, err := s.notifySvc.List(projectID, wfID)
		if err != nil {
			return nil, err
		}
		for _, ch := range channels {
			ch.ProjectID = ""
			ch.WorkflowID = ""
		}

		bundle.Workflows = append(bundle.Workflows, types.WorkflowBundleEntry{
			Workflow:      wf,
			Agents:        agents,
			LayerPolicies: policies,
			Notifications: channels,
		})
	}

	scriptIDs := make([]string, 0, len(seenScripts))
	for id := range seenScripts {
		scriptIDs = append(scriptIDs, id)
	}
	sort.Strings(scriptIDs)

	for _, sid := range scriptIDs {
		script, err := s.pythonScriptSvc.Get(projectID, sid)
		if err != nil {
			continue
		}
		script.ProjectID = ""
		bundle.PythonScripts = append(bundle.PythonScripts, script)
	}

	return bundle, nil
}

// fetchWorkflowModel queries the workflows table and returns a model.Workflow.
func (s *WorkflowExportService) fetchWorkflowModel(projectID, workflowID string) (*model.Workflow, error) {
	var desc, scopeType, groupsStr, nextWF string
	var closeOnComplete bool
	err := s.pool.QueryRow(`
		SELECT description, scope_type, groups, close_ticket_on_complete, next_workflow_on_success
		FROM workflows WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)`,
		projectID, workflowID).Scan(&desc, &scopeType, &groupsStr, &closeOnComplete, &nextWF)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workflow not found: %s", workflowID)
	}
	if err != nil {
		return nil, err
	}
	return &model.Workflow{
		ID:                    strings.ToLower(workflowID),
		ProjectID:             strings.ToLower(projectID),
		Description:           desc,
		ScopeType:             scopeType,
		CloseTicketOnComplete: closeOnComplete,
		NextWorkflowOnSuccess: nextWF,
		Groups:                groupsStr,
	}, nil
}
