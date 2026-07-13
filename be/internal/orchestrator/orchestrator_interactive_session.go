package orchestrator

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/spawner/apirun/provider"
)

// setupInteractivePreStep creates a user_interactive agent session, builds PTY
// command args, registers the command with the PTY manager, and sets up the
// wait channel. Called from Start() before launching runLoop.
func (o *Orchestrator) setupInteractivePreStep(
	req RunRequest,
	wi *model.WorkflowInstance,
	svcWf service.SpawnerWorkflowDef,
	svcAgents map[string]service.SpawnerAgentConfig,
	workflows map[string]spawner.WorkflowDef,
	agents map[string]spawner.AgentConfig,
	projectRoot string,
	modelConfigs map[string]spawner.ModelConfig,
	apiModelConfigs map[string]spawner.APIModelConfig,
	claudeSettingsJSON string,
) (*interactivePreStep, error) {
	sessionID := uuid.New().String()

	// Determine agent type and model for the session. Both modes derive the
	// model from the workflow's L0 agent (Phases[0] is the tie-breaker when
	// L0 has multiple agents) so plan capability tracks workflow capability.
	// opus_4_8 is the last-resort fallback when the workflow has no phases
	// or the L0 agent has no configured model.
	var agentType, modelName, phase string
	modelName = "opus_4_8"
	if len(svcWf.Phases) > 0 {
		l0Agent := svcWf.Phases[0].Agent
		if cfg, ok := svcAgents[l0Agent]; ok && cfg.Model != "" {
			modelName = cfg.Model
		}
		if req.PlanMode {
			agentType = "planner"
			phase = "planning"
		} else {
			agentType = l0Agent
			phase = l0Agent
		}
	} else if req.PlanMode {
		agentType = "planner"
		phase = "planning"
	} else {
		return nil, fmt.Errorf("workflow has no phases")
	}

	cliName := cliNameFromModelConfigs(modelConfigs, modelName)
	modelID := fmt.Sprintf("%s:%s", cliName, modelName)

	// Create agent session in DB with user_interactive status
	pool, err := db.NewPool(o.dataPath, db.DefaultPoolConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to create pool for interactive session: %w", err)
	}
	defer pool.Close()

	now := o.clock.Now().UTC().Format(time.RFC3339Nano)
	sessionRepo := repo.NewAgentSessionRepo(pool, o.clock)
	session := &model.AgentSession{
		ID:                 sessionID,
		ProjectID:          req.ProjectID,
		TicketID:           req.TicketID,
		WorkflowInstanceID: wi.ID,
		Phase:              phase,
		NodeID:             phase,
		AgentType:          agentType,
		ModelID:            sql.NullString{String: modelID, Valid: true},
		Status:             model.AgentSessionUserInteractive,
		StartedAt:          sql.NullString{String: now, Valid: true},
	}
	if err := sessionRepo.Create(session); err != nil {
		return nil, fmt.Errorf("failed to create interactive session: %w", err)
	}

	// Build PTY command args
	args, err := o.buildInteractivePtyArgs(req, wi, sessionID, modelName, svcWf, workflows, agents, pool, projectRoot, modelConfigs, apiModelConfigs, claudeSettingsJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to build interactive PTY args: %w", err)
	}

	// Register command with PTY manager
	if o.OnRegisterPtyCommand != nil {
		o.OnRegisterPtyCommand(sessionID, "claude", args)
	}

	// Create a temp spawner just for the interactive wait mechanism.
	// Manifest fields are not needed here (CLI-only, no API spawn).
	wfiID := wi.ID
	interactivePool := pool
	sp := spawner.New(spawner.Config{
		Workflows:       workflows,
		Agents:          agents,
		DataPath:        o.dataPath,
		WSHub:           o.wsHub,
		Clock:           o.clock,
		ModelConfigs:    modelConfigs,
		APIModelConfigs: apiModelConfigs,
		BuildAPIProvider: func(ctx context.Context, providerName, projectID string) (provider.Provider, error) {
			return buildAPIProvider(ctx, interactivePool, o.clock, providerName, projectID)
		},
		OnSessionRegister: func(sid string, s *spawner.Spawner) {
			o.mu.Lock()
			if rs, ok := o.runs[wfiID]; ok {
				rs.spawners[sid] = s
			}
			o.mu.Unlock()
		},
		OnSessionUnregister: func(sid string) {
			o.mu.Lock()
			if rs, ok := o.runs[wfiID]; ok {
				delete(rs.spawners, sid)
			}
			o.mu.Unlock()
		},
	})
	waitCh := sp.RegisterInteractiveWait(sessionID)

	return &interactivePreStep{
		sessionID: sessionID,
		waitCh:    waitCh,
		spawner:   sp,
	}, nil
}
