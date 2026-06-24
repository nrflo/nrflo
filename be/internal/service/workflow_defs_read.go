package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"be/internal/model"
	"be/internal/types"
)

// parseFindingSchemas parses the finding_schemas JSON column into a slice,
// always returning a non-nil slice.
func parseFindingSchemas(s string) []types.FindingSchema {
	defs := []types.FindingSchema{}
	if s != "" {
		_ = json.Unmarshal([]byte(s), &defs)
	}
	if defs == nil {
		defs = []types.FindingSchema{}
	}
	return defs
}

// GetWorkflowDef gets a single workflow definition from the database
func (s *WorkflowService) GetWorkflowDef(projectID, workflowID string) (*WorkflowDef, error) {
	var description, scopeType, groupsStr, nextWorkflowOnSuccess string
	var finalizeSuccessCommand, finalizeSuccessScriptID, finalizeFailureCommand, finalizeFailureScriptID string
	var pauseEventCommand, pauseEventScriptID string
	var closeTicketOnComplete, purgeOnCompletion bool
	var observerContext string
	var observerProvider, observerModel sql.NullString
	var findingSchemasStr string

	load := func(pid string) error {
		return s.pool.QueryRow(`
		SELECT description, scope_type, groups, close_ticket_on_complete, purge_on_completion, next_workflow_on_success, finalize_success_command, finalize_success_script_id, finalize_failure_command, finalize_failure_script_id, pause_event_command, pause_event_script_id, observer_context, observer_provider, observer_model, finding_schemas
		FROM workflows WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)`,
			pid, workflowID).Scan(&description, &scopeType, &groupsStr, &closeTicketOnComplete, &purgeOnCompletion, &nextWorkflowOnSuccess,
			&finalizeSuccessCommand, &finalizeSuccessScriptID, &finalizeFailureCommand, &finalizeFailureScriptID,
			&pauseEventCommand, &pauseEventScriptID,
			&observerContext, &observerProvider, &observerModel, &findingSchemasStr)
	}
	// Definitions may be global: try the selected project, then fall back to
	// GlobalProjectID. Sub-definitions (agent defs) load from the project the
	// workflow row was actually found under.
	defProjectID := projectID
	err := load(projectID)
	if err == sql.ErrNoRows && !strings.EqualFold(projectID, GlobalProjectID) {
		defProjectID = GlobalProjectID
		err = load(GlobalProjectID)
	}
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workflow not found: %s", workflowID)
	}
	if err != nil {
		return nil, err
	}

	agentDefs, err := s.listAgentDefsForWorkflow(defProjectID, workflowID)
	if err != nil {
		return nil, err
	}

	wf := parseWorkflowDefFromDB(description, agentDefs)
	wf.ScopeType = scopeType
	wf.CloseTicketOnComplete = closeTicketOnComplete
	wf.PurgeOnCompletion = purgeOnCompletion
	wf.NextWorkflowOnSuccess = nextWorkflowOnSuccess
	wf.FinalizeSuccessCommand = finalizeSuccessCommand
	wf.FinalizeSuccessScriptID = finalizeSuccessScriptID
	wf.FinalizeFailureCommand = finalizeFailureCommand
	wf.FinalizeFailureScriptID = finalizeFailureScriptID
	wf.PauseEventCommand = pauseEventCommand
	wf.PauseEventScriptID = pauseEventScriptID
	wf.ObserverContext = observerContext
	if observerProvider.Valid && observerProvider.String != "" {
		wf.ObserverProvider = &observerProvider.String
	}
	if observerModel.Valid && observerModel.String != "" {
		wf.ObserverModel = &observerModel.String
	}
	wf.FindingSchemas = parseFindingSchemas(findingSchemasStr)
	var groups []string
	if groupsStr != "" {
		_ = json.Unmarshal([]byte(groupsStr), &groups)
	}
	if groups == nil {
		groups = []string{}
	}
	wf.Groups = groups
	return wf, nil
}

// ListWorkflowDefs loads all workflow definitions for a project from the database
func (s *WorkflowService) ListWorkflowDefs(projectID string) (map[string]WorkflowDef, error) {
	rows, err := s.pool.Query(`
		SELECT id, description, scope_type, groups, close_ticket_on_complete, purge_on_completion, next_workflow_on_success, finalize_success_command, finalize_success_script_id, finalize_failure_command, finalize_failure_script_id, pause_event_command, pause_event_script_id, observer_context, observer_provider, observer_model, finding_schemas
		FROM workflows WHERE LOWER(project_id) = LOWER(?)
		ORDER BY id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type wfMeta struct {
		id, description, scopeType, groupsStr, nextWorkflowOnSuccess string
		finalizeSuccessCommand, finalizeSuccessScriptID              string
		finalizeFailureCommand, finalizeFailureScriptID              string
		pauseEventCommand, pauseEventScriptID                        string
		closeTicketOnComplete, purgeOnCompletion                     bool
		observerContext                                              string
		observerProvider, observerModel                              sql.NullString
		findingSchemasStr                                            string
	}
	var metas []wfMeta
	for rows.Next() {
		var m wfMeta
		if err := rows.Scan(&m.id, &m.description, &m.scopeType, &m.groupsStr, &m.closeTicketOnComplete, &m.purgeOnCompletion, &m.nextWorkflowOnSuccess,
			&m.finalizeSuccessCommand, &m.finalizeSuccessScriptID, &m.finalizeFailureCommand, &m.finalizeFailureScriptID,
			&m.pauseEventCommand, &m.pauseEventScriptID,
			&m.observerContext, &m.observerProvider, &m.observerModel, &m.findingSchemasStr); err != nil {
			return nil, err
		}
		if IsReservedWorkflowName(m.id) {
			continue
		}
		metas = append(metas, m)
	}

	allAgentDefs, err := s.listAgentDefsForProject(projectID)
	if err != nil {
		return nil, err
	}

	agentsByWorkflow := make(map[string][]*model.AgentDefinition)
	for _, ad := range allAgentDefs {
		agentsByWorkflow[ad.WorkflowID] = append(agentsByWorkflow[ad.WorkflowID], ad)
	}

	result := make(map[string]WorkflowDef)
	for _, m := range metas {
		wf := parseWorkflowDefFromDB(m.description, agentsByWorkflow[m.id])
		wf.ScopeType = m.scopeType
		wf.CloseTicketOnComplete = m.closeTicketOnComplete
		wf.PurgeOnCompletion = m.purgeOnCompletion
		wf.NextWorkflowOnSuccess = m.nextWorkflowOnSuccess
		wf.FinalizeSuccessCommand = m.finalizeSuccessCommand
		wf.FinalizeSuccessScriptID = m.finalizeSuccessScriptID
		wf.FinalizeFailureCommand = m.finalizeFailureCommand
		wf.FinalizeFailureScriptID = m.finalizeFailureScriptID
		wf.PauseEventCommand = m.pauseEventCommand
		wf.PauseEventScriptID = m.pauseEventScriptID
		wf.ObserverContext = m.observerContext
		if m.observerProvider.Valid && m.observerProvider.String != "" {
			wf.ObserverProvider = &m.observerProvider.String
		}
		if m.observerModel.Valid && m.observerModel.String != "" {
			wf.ObserverModel = &m.observerModel.String
		}
		wf.FindingSchemas = parseFindingSchemas(m.findingSchemasStr)
		var groups []string
		if m.groupsStr != "" {
			_ = json.Unmarshal([]byte(m.groupsStr), &groups)
		}
		if groups == nil {
			groups = []string{}
		}
		wf.Groups = groups
		result[m.id] = *wf
	}

	return result, nil
}
