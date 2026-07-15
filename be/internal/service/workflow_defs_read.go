package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"be/internal/model"
	"be/internal/types"
)

// workflowDefCols is the workflows column list shared by the single-get and
// list queries (kept in one place so the scan order can never drift).
const workflowDefCols = `id, description, scope_type, groups, close_ticket_on_complete, purge_on_completion, callable_as_subworkflow, is_global, next_workflow_on_success, finalize_success_command, finalize_success_script_id, finalize_failure_command, finalize_failure_script_id, pause_event_command, pause_event_script_id, observer_context, observer_provider, observer_model, finding_schemas`

// wfMeta holds the raw workflow-definition columns scanned from a workflows row.
type wfMeta struct {
	id, description, scopeType, groupsStr, nextWorkflowOnSuccess string
	finalizeSuccessCommand, finalizeSuccessScriptID              string
	finalizeFailureCommand, finalizeFailureScriptID              string
	pauseEventCommand, pauseEventScriptID                        string
	closeTicketOnComplete, purgeOnCompletion, isGlobal           bool
	callableAsSubworkflow                                        bool
	observerContext                                              string
	observerProvider, observerModel                              sql.NullString
	findingSchemasStr                                            string
}

// scanWFMeta scans one workflows row (in workflowDefCols order) into a wfMeta.
// It accepts both *sql.Row and *sql.Rows.
func scanWFMeta(sc interface{ Scan(...any) error }) (wfMeta, error) {
	var m wfMeta
	err := sc.Scan(&m.id, &m.description, &m.scopeType, &m.groupsStr, &m.closeTicketOnComplete, &m.purgeOnCompletion, &m.callableAsSubworkflow, &m.isGlobal, &m.nextWorkflowOnSuccess,
		&m.finalizeSuccessCommand, &m.finalizeSuccessScriptID, &m.finalizeFailureCommand, &m.finalizeFailureScriptID,
		&m.pauseEventCommand, &m.pauseEventScriptID,
		&m.observerContext, &m.observerProvider, &m.observerModel, &m.findingSchemasStr)
	return m, err
}

// buildWorkflowDef assembles a WorkflowDef from a scanned meta + its agent defs.
func buildWorkflowDef(m wfMeta, agentDefs []*model.AgentDefinition) WorkflowDef {
	wf := parseWorkflowDefFromDB(m.description, agentDefs)
	wf.ScopeType = m.scopeType
	wf.CloseTicketOnComplete = m.closeTicketOnComplete
	wf.PurgeOnCompletion = m.purgeOnCompletion
	wf.CallableAsSubworkflow = m.callableAsSubworkflow
	wf.IsGlobal = m.isGlobal
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
	wf.Groups = parseGroups(m.groupsStr)
	return *wf
}

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

// parseGroups parses the groups JSON array column, always returning non-nil.
func parseGroups(s string) []string {
	var groups []string
	if s != "" {
		_ = json.Unmarshal([]byte(s), &groups)
	}
	if groups == nil {
		groups = []string{}
	}
	return groups
}

// GetWorkflowDef gets a single workflow definition from the database.
// Definitions may be global: it tries the selected project, then falls back to
// GlobalProjectID. Agent defs load from the project the row was found under.
func (s *WorkflowService) GetWorkflowDef(projectID, workflowID string) (*WorkflowDef, error) {
	load := func(pid string) (wfMeta, error) {
		row := s.pool.QueryRow(`SELECT `+workflowDefCols+`
			FROM workflows WHERE LOWER(project_id) = LOWER(?) AND LOWER(id) = LOWER(?)`,
			pid, workflowID)
		return scanWFMeta(row)
	}

	defProjectID := projectID
	m, err := load(projectID)
	if err == sql.ErrNoRows && !strings.EqualFold(projectID, GlobalProjectID) {
		defProjectID = GlobalProjectID
		m, err = load(GlobalProjectID)
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

	wf := buildWorkflowDef(m, agentDefs)
	wf.defProjectID = defProjectID
	return &wf, nil
}
