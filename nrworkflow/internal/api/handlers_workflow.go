package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"nrworkflow/internal/model"
	"nrworkflow/internal/repo"
)

// WorkflowState represents the parsed agents_state for a single workflow
type WorkflowState struct {
	Workflow     string                   `json:"workflow,omitempty"`
	Version      int                      `json:"version,omitempty"`
	CurrentPhase string                   `json:"current_phase,omitempty"`
	Category     string                   `json:"category,omitempty"`
	Phases       map[string]PhaseState    `json:"phases,omitempty"`
	ActiveAgent  *ActiveAgent             `json:"active_agent,omitempty"`  // v3 compat
	ActiveAgents map[string]ActiveAgentV4 `json:"active_agents,omitempty"` // v4 format
	Findings     map[string]interface{}   `json:"findings,omitempty"`      // workflow-level findings
	History      []HistoryEntry           `json:"history,omitempty"`
}

// PhaseState represents the state of a workflow phase
type PhaseState struct {
	Status    string `json:"status"`
	Result    string `json:"result,omitempty"`
	StartedAt string `json:"started_at,omitempty"`
	EndedAt   string `json:"ended_at,omitempty"`
	Error     string `json:"error,omitempty"`
}

// ActiveAgent represents the currently active agent (v3 format)
type ActiveAgent struct {
	Type      string `json:"type"`
	PID       int    `json:"pid,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	StartedAt string `json:"started_at,omitempty"`
}

// ActiveAgentV4 represents an active agent in v4 format (parallel agents)
type ActiveAgentV4 struct {
	AgentID   string `json:"agent_id,omitempty"`
	AgentType string `json:"agent_type"`
	ModelID   string `json:"model_id,omitempty"`
	CLI       string `json:"cli,omitempty"`
	Model     string `json:"model,omitempty"`
	PID       int    `json:"pid,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	StartedAt string `json:"started_at,omitempty"`
	EndedAt   string `json:"ended_at,omitempty"`
	Result    string `json:"result,omitempty"`
}

// HistoryEntry represents a historical agent run
type HistoryEntry struct {
	Type      string `json:"type"`
	Phase     string `json:"phase"`
	Status    string `json:"status"`
	StartedAt string `json:"started_at,omitempty"`
	EndedAt   string `json:"ended_at,omitempty"`
}

// parseWorkflowStateFromMap parses a single workflow state from a map
func parseWorkflowStateFromMap(raw map[string]interface{}, workflowName string) *WorkflowState {
	state := &WorkflowState{
		Workflow:     workflowName,
		Phases:       make(map[string]PhaseState),
		ActiveAgents: make(map[string]ActiveAgentV4),
		History:      []HistoryEntry{},
	}

	// Extract version
	if version, ok := raw["version"].(float64); ok {
		state.Version = int(version)
	}

	// Extract known fields
	if workflow, ok := raw["workflow"].(string); ok {
		state.Workflow = workflow
	}
	if currentPhase, ok := raw["current_phase"].(string); ok {
		state.CurrentPhase = currentPhase
	}
	if category, ok := raw["category"].(string); ok {
		state.Category = category
	}

	// Extract phases
	if phases, ok := raw["phases"].(map[string]interface{}); ok {
		for name, phase := range phases {
			if phaseMap, ok := phase.(map[string]interface{}); ok {
				ps := PhaseState{}
				if status, ok := phaseMap["status"].(string); ok {
					ps.Status = status
				}
				if result, ok := phaseMap["result"].(string); ok {
					ps.Result = result
				}
				if startedAt, ok := phaseMap["started_at"].(string); ok {
					ps.StartedAt = startedAt
				}
				if endedAt, ok := phaseMap["ended_at"].(string); ok {
					ps.EndedAt = endedAt
				}
				if errMsg, ok := phaseMap["error"].(string); ok {
					ps.Error = errMsg
				}
				state.Phases[name] = ps
			}
		}
	}

	// Extract active_agents (v4 format - plural)
	if agents, ok := raw["active_agents"].(map[string]interface{}); ok {
		for key, agent := range agents {
			if agentMap, ok := agent.(map[string]interface{}); ok {
				aa := ActiveAgentV4{}
				if agentID, ok := agentMap["agent_id"].(string); ok {
					aa.AgentID = agentID
				}
				if agentType, ok := agentMap["agent_type"].(string); ok {
					aa.AgentType = agentType
				}
				if modelID, ok := agentMap["model_id"].(string); ok {
					aa.ModelID = modelID
				}
				if cli, ok := agentMap["cli"].(string); ok {
					aa.CLI = cli
				}
				if model, ok := agentMap["model"].(string); ok {
					aa.Model = model
				}
				if pid, ok := agentMap["pid"].(float64); ok {
					aa.PID = int(pid)
				}
				if sessionID, ok := agentMap["session_id"].(string); ok {
					aa.SessionID = sessionID
				}
				if startedAt, ok := agentMap["started_at"].(string); ok {
					aa.StartedAt = startedAt
				}
				if endedAt, ok := agentMap["ended_at"].(string); ok {
					aa.EndedAt = endedAt
				}
				if result, ok := agentMap["result"].(string); ok {
					aa.Result = result
				}
				state.ActiveAgents[key] = aa
			}
		}
	}

	// Extract active_agent (v3 format - singular) for backward compatibility
	if agent, ok := raw["active_agent"].(map[string]interface{}); ok {
		state.ActiveAgent = &ActiveAgent{}
		if agentType, ok := agent["type"].(string); ok {
			state.ActiveAgent.Type = agentType
		}
		if pid, ok := agent["pid"].(float64); ok {
			state.ActiveAgent.PID = int(pid)
		}
		if sessionID, ok := agent["session_id"].(string); ok {
			state.ActiveAgent.SessionID = sessionID
		}
		if startedAt, ok := agent["started_at"].(string); ok {
			state.ActiveAgent.StartedAt = startedAt
		}
	}

	// Extract workflow-level findings (v4 format)
	if findings, ok := raw["findings"].(map[string]interface{}); ok {
		state.Findings = findings
	}

	// Extract history
	if history, ok := raw["history"].([]interface{}); ok {
		for _, entry := range history {
			if entryMap, ok := entry.(map[string]interface{}); ok {
				he := HistoryEntry{}
				if t, ok := entryMap["type"].(string); ok {
					he.Type = t
				}
				if phase, ok := entryMap["phase"].(string); ok {
					he.Phase = phase
				}
				if status, ok := entryMap["status"].(string); ok {
					he.Status = status
				}
				if startedAt, ok := entryMap["started_at"].(string); ok {
					he.StartedAt = startedAt
				}
				if endedAt, ok := entryMap["ended_at"].(string); ok {
					he.EndedAt = endedAt
				}
				state.History = append(state.History, he)
			}
		}
	}

	return state
}

// isV4WorkflowFormat checks if the raw JSON is in v4 format (workflow names as keys)
func isV4WorkflowFormat(raw map[string]interface{}) bool {
	// V4 format has workflow names as top-level keys, each containing a workflow object
	// Check if any top-level key contains a map with "version" or "current_phase"
	for _, v := range raw {
		if wfMap, ok := v.(map[string]interface{}); ok {
			if _, hasVersion := wfMap["version"]; hasVersion {
				return true
			}
			if _, hasPhase := wfMap["current_phase"]; hasPhase {
				return true
			}
			if _, hasPhases := wfMap["phases"]; hasPhases {
				return true
			}
		}
	}
	return false
}

// handleGetWorkflow returns the parsed workflow state for a ticket
func (s *Server) handleGetWorkflow(w http.ResponseWriter, r *http.Request) {
	ticketRepo, _, database, err := s.getRepos(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer database.Close()

	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	id := extractID(r)
	ticket, err := ticketRepo.Get(projectID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// Default empty state
	emptyState := &WorkflowState{
		Phases:       make(map[string]PhaseState),
		ActiveAgents: make(map[string]ActiveAgentV4),
		History:      []HistoryEntry{},
	}

	if !ticket.AgentsState.Valid || ticket.AgentsState.String == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ticket_id":    id,
			"has_workflow": false,
			"state":        emptyState,
			"workflows":    []string{},
		})
		return
	}

	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(ticket.AgentsState.String), &raw); err != nil {
		// Return empty state if parsing fails, include raw for debugging
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ticket_id":    id,
			"has_workflow": false,
			"state":        emptyState,
			"workflows":    []string{},
			"raw":          ticket.AgentsState.String,
			"parse_error":  err.Error(),
		})
		return
	}

	// Check query param for specific workflow
	requestedWorkflow := r.URL.Query().Get("workflow")

	// Check if this is v4 format (workflow names as keys)
	if isV4WorkflowFormat(raw) {
		workflowNames := []string{}
		allWorkflows := make(map[string]*WorkflowState)

		for name, wf := range raw {
			if wfMap, ok := wf.(map[string]interface{}); ok {
				workflowNames = append(workflowNames, name)
				allWorkflows[name] = parseWorkflowStateFromMap(wfMap, name)
			}
		}

		// Return specific workflow if requested, otherwise first one
		var selectedState *WorkflowState
		if requestedWorkflow != "" {
			if state, ok := allWorkflows[requestedWorkflow]; ok {
				selectedState = state
			}
		}
		if selectedState == nil && len(workflowNames) > 0 {
			// Default to first workflow alphabetically
			for _, name := range workflowNames {
				selectedState = allWorkflows[name]
				break
			}
		}
		if selectedState == nil {
			selectedState = emptyState
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ticket_id":     id,
			"has_workflow":  len(workflowNames) > 0,
			"state":         selectedState,
			"workflows":     workflowNames,
			"all_workflows": allWorkflows,
		})
		return
	}

	// Legacy format - single workflow state at top level
	state := parseWorkflowStateFromMap(raw, "")

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ticket_id":    id,
		"has_workflow": true,
		"state":        state,
		"workflows":    []string{state.Workflow},
	})
}

// UpdateWorkflowRequest represents the request to update workflow state
type UpdateWorkflowRequest struct {
	// Full replacement of agents_state
	State *map[string]interface{} `json:"state,omitempty"`
	// Or just update a specific phase
	Phase        string                  `json:"phase,omitempty"`
	PhaseUpdate  *map[string]interface{} `json:"phase_update,omitempty"`
	// Or update specific fields
	CurrentPhase *string `json:"current_phase,omitempty"`
}

// handleUpdateWorkflow updates the workflow state for a ticket
func (s *Server) handleUpdateWorkflow(w http.ResponseWriter, r *http.Request) {
	ticketRepo, _, database, err := s.getRepos(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer database.Close()

	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	id := extractID(r)

	// Check ticket exists
	ticket, err := ticketRepo.Get(projectID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var req UpdateWorkflowRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var newState map[string]interface{}

	if req.State != nil {
		// Full state replacement
		newState = *req.State
	} else {
		// Partial update - start with existing state
		if ticket.AgentsState.Valid && ticket.AgentsState.String != "" {
			if err := json.Unmarshal([]byte(ticket.AgentsState.String), &newState); err != nil {
				newState = make(map[string]interface{})
			}
		} else {
			newState = make(map[string]interface{})
		}

		// Update current_phase if specified
		if req.CurrentPhase != nil {
			newState["current_phase"] = *req.CurrentPhase
		}

		// Update specific phase if specified
		if req.Phase != "" && req.PhaseUpdate != nil {
			phases, ok := newState["phases"].(map[string]interface{})
			if !ok {
				phases = make(map[string]interface{})
				newState["phases"] = phases
			}

			existingPhase, ok := phases[req.Phase].(map[string]interface{})
			if !ok {
				existingPhase = make(map[string]interface{})
			}

			// Merge phase update
			for k, v := range *req.PhaseUpdate {
				existingPhase[k] = v
			}
			phases[req.Phase] = existingPhase
		}
	}

	// Serialize new state
	stateJSON, err := json.Marshal(newState)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to serialize state")
		return
	}

	stateStr := string(stateJSON)
	fields := &repo.UpdateFields{
		AgentsState: &stateStr,
	}

	if err := ticketRepo.Update(projectID, id, fields); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Return the updated workflow
	s.handleGetWorkflow(w, r)
}

// handleGetAgentSessions returns agent sessions for a ticket with findings
func (s *Server) handleGetAgentSessions(w http.ResponseWriter, r *http.Request) {
	ticketRepo, _, agentSessionRepo, _, database, err := s.getAllRepos(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer database.Close()

	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	id := extractID(r)
	phase := r.URL.Query().Get("phase")

	sessions, err := agentSessionRepo.GetByTicket(projectID, id, phase)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if sessions == nil {
		sessions = []*model.AgentSession{}
	}

	// Fetch findings from ticket's agents_state
	var findings map[string]interface{}
	ticket, err := ticketRepo.Get(projectID, id)
	if err == nil && ticket != nil && ticket.AgentsState.Valid {
		var allState map[string]interface{}
		if json.Unmarshal([]byte(ticket.AgentsState.String), &allState) == nil {
			// Extract findings from all workflows
			findings = make(map[string]interface{})
			for _, workflowStateRaw := range allState {
				if workflowState, ok := workflowStateRaw.(map[string]interface{}); ok {
					if workflowFindings, ok := workflowState["findings"].(map[string]interface{}); ok {
						for agentType, agentFindings := range workflowFindings {
							findings[agentType] = agentFindings
						}
					}
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ticket_id": id,
		"sessions":  sessions,
		"findings":  findings,
	})
}

// handleGetRecentAgents returns recent agent sessions across all projects
func (s *Server) handleGetRecentAgents(w http.ResponseWriter, r *http.Request) {
	database, err := s.getDatabase()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer database.Close()

	agentSessionRepo := repo.NewAgentSessionRepo(database)
	projectRepo := repo.NewProjectRepo(database)

	// Parse limit from query param (default 10, max 50)
	limit := 10
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
			if limit > 50 {
				limit = 50
			}
		}
	}

	sessions, err := agentSessionRepo.GetRecent(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if sessions == nil {
		sessions = []*model.AgentSession{}
	}

	// Get project names
	projects, err := projectRepo.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	projectMap := make(map[string]string)
	for _, p := range projects {
		projectMap[p.ID] = p.Name
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sessions": sessions,
		"projects": projectMap,
	})
}
