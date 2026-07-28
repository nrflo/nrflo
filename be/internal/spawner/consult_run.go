package spawner

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

const consultTimeout = 10 * time.Minute

// consultRequest bundles what runConsult needs regardless of whether the
// caller is a session-bound in-run agent (Consult) or a host caller with no
// bound instance (ConsultHost) — CallerSessionID/TicketID/ParentWFI/
// Transcript are empty in the host case.
type consultRequest struct {
	CallerSessionID string
	ProjectID       string
	TicketID        string
	WorkflowName    string
	ParentWFI       string
	ScopeType       string
	ConsultantID    string
	Def             *model.AgentDefinition
	Question        string
	Transcript      string
}

// runConsult validates req.Def, synchronously spawns it as a one-phase
// "_consult" child, waits for completion, and returns its _consult_answer
// finding. Shared by Consult and ConsultHost.
func (s *Spawner) runConsult(ctx context.Context, req consultRequest) (string, error) {
	pool := s.pool()
	if pool == nil {
		return "", fmt.Errorf("consult: no database pool")
	}
	if req.Def == nil {
		return "", fmt.Errorf("consult: agent definition %q not found", req.ConsultantID)
	}
	if !req.Def.Consultant {
		return "", fmt.Errorf("consult: agent %q is not flagged as a consultant", req.ConsultantID)
	}
	if req.Def.ExecutionMode != "api" {
		return "", fmt.Errorf("consult: agent %q must have execution_mode=api (got %q)", req.ConsultantID, req.Def.ExecutionMode)
	}

	var consultMu sync.Mutex
	var consultSID string
	var consultSp *Spawner
	consultRegister, consultUnregister := s.childSessionHooks(func(sid string, child *Spawner) {
		consultMu.Lock()
		if child == consultSp {
			consultSID = sid
		}
		consultMu.Unlock()
	})

	sp := New(Config{
		Workflows: map[string]WorkflowDef{
			req.WorkflowName: {
				Phases: []PhaseDef{{NodeID: "_consult", Agent: req.ConsultantID, Layer: 0}},
			},
		},
		Agents: map[string]AgentConfig{
			req.ConsultantID: {
				Model:            req.Def.Model,
				Timeout:          req.Def.Timeout,
				ExecutionMode:    "api",
				Tools:            req.Def.Tools,
				APIMaxIterations: req.Def.APIMaxIterations,
				APIMaxTokens:     req.Def.APIMaxTokens,
			},
		},
		DataPath:            s.config.DataPath,
		ProjectRoot:         s.config.ProjectRoot,
		WSHub:               s.config.WSHub,
		Pool:                pool,
		Clock:               s.config.Clock,
		ClaudeSettingsJSON:  s.config.ClaudeSettingsJSON,
		ModelConfigs:        s.config.ModelConfigs,
		ErrorSvc:            s.config.ErrorSvc,
		BuildAPIProvider:    s.config.BuildAPIProvider,
		AgentSvc:            s.config.AgentSvc,
		FindingsSvc:         s.config.FindingsSvc,
		ProjectFindingsSvc:  s.config.ProjectFindingsSvc,
		AgentSvcReal:        s.config.AgentSvcReal,
		WorkflowSvc:         s.config.WorkflowSvc,
		TicketSvc:           s.config.TicketSvc,
		DispatchRepo:        s.config.DispatchRepo,
		ArtifactSvc:         s.config.ArtifactSvc,
		PTYManager:          s.config.PTYManager,
		ProjectEnv:          s.config.ProjectEnv,
		APIMode:             true,
		OnSessionRegister:   consultRegister,
		OnSessionUnregister: consultUnregister,
	})
	consultMu.Lock()
	consultSp = sp
	consultMu.Unlock()

	s.broadcast(ws.EventConsultStarted, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"caller_session_id": req.CallerSessionID,
		"consultant_id":     req.ConsultantID,
	})

	timeout := consultTimeout
	if req.Def.Timeout > 0 {
		timeout = time.Duration(req.Def.Timeout) * time.Second
	}
	ctxTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	spawnErr := sp.Spawn(ctxTimeout, SpawnRequest{
		AgentType:          req.ConsultantID,
		NodeID:             "_consult",
		TicketID:           req.TicketID,
		ProjectID:          req.ProjectID,
		WorkflowName:       req.WorkflowName,
		WorkflowInstanceID: req.ParentWFI,
		ScopeType:          req.ScopeType,
		ExtraVars: map[string]string{
			"CALLER_TRANSCRIPT": req.Transcript,
			"CONSULT_QUESTION":  req.Question,
		},
	})
	sp.Close()

	consultMu.Lock()
	sid := consultSID
	consultMu.Unlock()

	failBroadcast := func(errMsg string) {
		s.broadcast(ws.EventConsultFailed, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
			"caller_session_id": req.CallerSessionID,
			"consultant_id":     req.ConsultantID,
			"error":             errMsg,
		})
	}

	if spawnErr != nil {
		failBroadcast(spawnErr.Error())
		return "", fmt.Errorf("consult: spawn failed: %w", spawnErr)
	}
	if sid == "" {
		msg := fmt.Sprintf("no session registered for consultant %q", req.ConsultantID)
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
		msg := fmt.Sprintf("consultant %q did not write _consult_answer", req.ConsultantID)
		failBroadcast(msg)
		return "", fmt.Errorf("consult: %s", msg)
	}

	var answer string
	if jsonErr := json.Unmarshal(rawAnswer, &answer); jsonErr != nil {
		answer = string(rawAnswer)
	}

	findingRepo.DeleteKeys("session", sid, []string{"_consult_answer"}, repo.Actor{Source: "system", ID: "consult"}) //nolint:errcheck

	s.broadcast(ws.EventConsultAnswered, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"caller_session_id": req.CallerSessionID,
		"consultant_id":     req.ConsultantID,
		"session_id":        sid,
	})

	return answer, nil
}
