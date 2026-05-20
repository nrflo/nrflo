package service

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"be/internal/model"
)

// GetWorkflowDef gets a single workflow definition from the database
func (s *WorkflowService) GetWorkflowDef(projectID, workflowID string) (*WorkflowDef, error) {
	var description, scopeType, groupsStr, nextWorkflowOnSuccess string
	var finalizeSuccessCommand, finalizeSuccessScriptID, finalizeFailureCommand, finalizeFailureScriptID string
	var closeTicketOnComplete bool
	var observerContext string
	var observerProvider, observerModel sql.NullString

	err := s.pool.QueryRow(`
		SELECT description, scope_type, groups, close_ticket_on_complete, next_workflow_on_success, finalize_success_command, finalize_success_script_id, finalize_failure_command, finalize_failure_script_id, observer_context, observer_provider, observer_model
		FROM workflows WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)`,
		projectID, workflowID).Scan(&description, &scopeType, &groupsStr, &closeTicketOnComplete, &nextWorkflowOnSuccess,
		&finalizeSuccessCommand, &finalizeSuccessScriptID, &finalizeFailureCommand, &finalizeFailureScriptID,
		&observerContext, &observerProvider, &observerModel)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workflow not found: %s", workflowID)
	}
	if err != nil {
		return nil, err
	}

	agentDefs, err := s.listAgentDefsForWorkflow(projectID, workflowID)
	if err != nil {
		return nil, err
	}

	wf := parseWorkflowDefFromDB(description, agentDefs)
	wf.ScopeType = scopeType
	wf.CloseTicketOnComplete = closeTicketOnComplete
	wf.NextWorkflowOnSuccess = nextWorkflowOnSuccess
	wf.FinalizeSuccessCommand = finalizeSuccessCommand
	wf.FinalizeSuccessScriptID = finalizeSuccessScriptID
	wf.FinalizeFailureCommand = finalizeFailureCommand
	wf.FinalizeFailureScriptID = finalizeFailureScriptID
	wf.ObserverContext = observerContext
	if observerProvider.Valid && observerProvider.String != "" {
		wf.ObserverProvider = &observerProvider.String
	}
	if observerModel.Valid && observerModel.String != "" {
		wf.ObserverModel = &observerModel.String
	}
	var groups []string
	if groupsStr != "" {
		json.Unmarshal([]byte(groupsStr), &groups)
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
		SELECT id, description, scope_type, groups, close_ticket_on_complete, next_workflow_on_success, finalize_success_command, finalize_success_script_id, finalize_failure_command, finalize_failure_script_id, observer_context, observer_provider, observer_model
		FROM workflows WHERE LOWER(project_id) = LOWER(?)
		ORDER BY id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type wfMeta struct {
		id, description, scopeType, groupsStr, nextWorkflowOnSuccess string
		finalizeSuccessCommand, finalizeSuccessScriptID               string
		finalizeFailureCommand, finalizeFailureScriptID               string
		closeTicketOnComplete                                          bool
		observerContext                                                string
		observerProvider, observerModel                               sql.NullString
	}
	var metas []wfMeta
	for rows.Next() {
		var m wfMeta
		if err := rows.Scan(&m.id, &m.description, &m.scopeType, &m.groupsStr, &m.closeTicketOnComplete, &m.nextWorkflowOnSuccess,
			&m.finalizeSuccessCommand, &m.finalizeSuccessScriptID, &m.finalizeFailureCommand, &m.finalizeFailureScriptID,
			&m.observerContext, &m.observerProvider, &m.observerModel); err != nil {
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
		wf.NextWorkflowOnSuccess = m.nextWorkflowOnSuccess
		wf.FinalizeSuccessCommand = m.finalizeSuccessCommand
		wf.FinalizeSuccessScriptID = m.finalizeSuccessScriptID
		wf.FinalizeFailureCommand = m.finalizeFailureCommand
		wf.FinalizeFailureScriptID = m.finalizeFailureScriptID
		wf.ObserverContext = m.observerContext
		if m.observerProvider.Valid && m.observerProvider.String != "" {
			wf.ObserverProvider = &m.observerProvider.String
		}
		if m.observerModel.Valid && m.observerModel.String != "" {
			wf.ObserverModel = &m.observerModel.String
		}
		var groups []string
		if m.groupsStr != "" {
			json.Unmarshal([]byte(m.groupsStr), &groups)
		}
		if groups == nil {
			groups = []string{}
		}
		wf.Groups = groups
		result[m.id] = *wf
	}

	return result, nil
}
