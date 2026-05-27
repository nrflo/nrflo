package spawner

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"be/internal/repo"
	"be/internal/ws"
)

const consultTimeout = 10 * time.Minute

// Consult synchronously spawns a named consultant agent under the parent workflow
// instance, waits for it to finish, then reads and returns the _consult_answer finding.
// Implements apirun.ConsultantSpawner.
func (s *Spawner) Consult(ctx context.Context, callerSessionID, consultantID, question string) (string, error) {
	pool := s.pool()
	if pool == nil {
		return "", fmt.Errorf("consult: no database pool")
	}

	// Resolve caller context from DB.
	sessionRepo := repo.NewAgentSessionRepo(pool, s.config.Clock)
	callerSession, err := sessionRepo.Get(callerSessionID)
	if err != nil {
		return "", fmt.Errorf("consult: resolve caller session: %w", err)
	}

	wfiRepo := repo.NewWorkflowInstanceRepo(pool, s.config.Clock)
	wfi, err := wfiRepo.Get(callerSession.WorkflowInstanceID)
	if err != nil {
		return "", fmt.Errorf("consult: resolve workflow instance: %w", err)
	}

	projectID := callerSession.ProjectID
	ticketID := callerSession.TicketID
	workflowName := wfi.WorkflowID
	scopeType := wfi.ScopeType
	parentWFI := callerSession.WorkflowInstanceID

	// Validate consultant agent definition.
	consultDef := s.loadAgentDefinition(consultantID, projectID, workflowName)
	if consultDef == nil {
		return "", fmt.Errorf("consult: agent definition %q not found in workflow %q", consultantID, workflowName)
	}
	if !consultDef.Consultant {
		return "", fmt.Errorf("consult: agent %q is not flagged as a consultant", consultantID)
	}
	if consultDef.ExecutionMode != "api" {
		return "", fmt.Errorf("consult: agent %q must have execution_mode=api (got %q)", consultantID, consultDef.ExecutionMode)
	}

	// Read and format caller transcript.
	msgRepo := repo.NewAgentMessageRepo(pool, s.config.Clock)
	messages, _ := msgRepo.GetBySession(callerSessionID)
	transcript := formatMessagesForSave(messages, maxMessageChars)

	// Capture consultant session ID via OnSessionRegister closure.
	var consultMu sync.Mutex
	var consultSID string

	sp := New(Config{
		Workflows: map[string]WorkflowDef{
			workflowName: {
				Phases: []PhaseDef{{ID: "_consult", Agent: consultantID, Layer: 0}},
			},
		},
		Agents: map[string]AgentConfig{
			consultantID: {
				Model:            consultDef.Model,
				Timeout:          consultDef.Timeout,
				ExecutionMode:    "api",
				Tools:            consultDef.Tools,
				APIMaxIterations: consultDef.APIMaxIterations,
				APIMaxTokens:     consultDef.APIMaxTokens,
			},
		},
		DataPath:           s.config.DataPath,
		ProjectRoot:        s.config.ProjectRoot,
		WSHub:              s.config.WSHub,
		Pool:               pool,
		Clock:              s.config.Clock,
		ClaudeSettingsJSON: s.config.ClaudeSettingsJSON,
		ModelConfigs:       s.config.ModelConfigs,
		ErrorSvc:           s.config.ErrorSvc,
		BuildAPIProvider:   s.config.BuildAPIProvider,
		APIModelConfigs:    s.config.APIModelConfigs,
		AgentSvc:           s.config.AgentSvc,
		FindingsSvc:        s.config.FindingsSvc,
		ProjectFindingsSvc: s.config.ProjectFindingsSvc,
		AgentSvcReal:       s.config.AgentSvcReal,
		WorkflowSvc:        s.config.WorkflowSvc,
		TicketSvc:          s.config.TicketSvc,
		DispatchRepo:       s.config.DispatchRepo,
		ArtifactSvc:        s.config.ArtifactSvc,
		PTYManager:         s.config.PTYManager,
		ProjectEnv:         s.config.ProjectEnv,
		APIMode:            true,
		OnSessionRegister: func(sid string, _ *Spawner) {
			consultMu.Lock()
			consultSID = sid
			consultMu.Unlock()
		},
	})

	s.broadcast(ws.EventConsultStarted, projectID, ticketID, workflowName, map[string]interface{}{
		"caller_session_id": callerSessionID,
		"consultant_id":     consultantID,
	})

	timeout := consultTimeout
	if consultDef.Timeout > 0 {
		timeout = time.Duration(consultDef.Timeout) * time.Second
	}
	ctxTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	spawnErr := sp.Spawn(ctxTimeout, SpawnRequest{
		AgentType:          consultantID,
		TicketID:           ticketID,
		ProjectID:          projectID,
		WorkflowName:       workflowName,
		WorkflowInstanceID: parentWFI,
		ScopeType:          scopeType,
		ExtraVars: map[string]string{
			"CALLER_TRANSCRIPT": transcript,
			"CONSULT_QUESTION":  question,
		},
	})
	sp.Close()

	consultMu.Lock()
	sid := consultSID
	consultMu.Unlock()

	failBroadcast := func(errMsg string) {
		s.broadcast(ws.EventConsultFailed, projectID, ticketID, workflowName, map[string]interface{}{
			"caller_session_id": callerSessionID,
			"consultant_id":     consultantID,
			"error":             errMsg,
		})
	}

	if spawnErr != nil {
		failBroadcast(spawnErr.Error())
		return "", fmt.Errorf("consult: spawn failed: %w", spawnErr)
	}
	if sid == "" {
		msg := fmt.Sprintf("no session registered for consultant %q", consultantID)
		failBroadcast(msg)
		return "", fmt.Errorf("consult: %s", msg)
	}

	findingRepo := repo.NewFindingRepo(pool, s.config.Clock)
	findings, err := findingRepo.GetOwn("session", sid)
	if err != nil {
		failBroadcast(err.Error())
		return "", fmt.Errorf("consult: read findings: %w", err)
	}

	rawAnswer, ok := findings["_consult_answer"]
	if !ok {
		msg := fmt.Sprintf("consultant %q did not write _consult_answer", consultantID)
		failBroadcast(msg)
		return "", fmt.Errorf("consult: %s", msg)
	}

	var answer string
	if jsonErr := json.Unmarshal(rawAnswer, &answer); jsonErr != nil {
		answer = string(rawAnswer)
	}

	findingRepo.DeleteKeys("session", sid, []string{"_consult_answer"}, repo.Actor{Source: "system", ID: "consult"}) //nolint:errcheck

	s.broadcast(ws.EventConsultAnswered, projectID, ticketID, workflowName, map[string]interface{}{
		"caller_session_id": callerSessionID,
		"consultant_id":     consultantID,
		"session_id":        sid,
	})

	return answer, nil
}
