package orchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner"
)

// interactivePreStep holds state for the interactive/plan pre-step that runs
// before the normal layer execution loop.
type interactivePreStep struct {
	sessionID string
	waitCh    <-chan struct{} // blocks until PTY session completes
	spawner   *spawner.Spawner
}

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
	claudeSettingsJSON string,
) (*interactivePreStep, error) {
	sessionID := uuid.New().String()

	// Determine agent type and model for the session. Both modes derive the
	// model from the workflow's L0 agent (Phases[0] is the tie-breaker when
	// L0 has multiple agents) so plan capability tracks workflow capability.
	// opus_4_7 is the last-resort fallback when the workflow has no phases
	// or the L0 agent has no configured model.
	var agentType, modelName, phase string
	modelName = "opus_4_7"
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
		AgentType:          agentType,
		ModelID:            sql.NullString{String: modelID, Valid: true},
		Status:             model.AgentSessionUserInteractive,
		StartedAt:          sql.NullString{String: now, Valid: true},
	}
	if err := sessionRepo.Create(session); err != nil {
		return nil, fmt.Errorf("failed to create interactive session: %w", err)
	}

	// Build PTY command args
	args, err := o.buildInteractivePtyArgs(req, wi, sessionID, modelName, svcWf, workflows, agents, pool, projectRoot, modelConfigs, claudeSettingsJSON)
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
	sp := spawner.New(spawner.Config{
		Workflows:    workflows,
		Agents:       agents,
		DataPath:     o.dataPath,
		WSHub:        o.wsHub,
		Clock:        o.clock,
		ModelConfigs: modelConfigs,
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

// buildInteractivePtyArgs builds the claude command args for interactive/plan PTY sessions.
func (o *Orchestrator) buildInteractivePtyArgs(
	req RunRequest,
	wi *model.WorkflowInstance,
	sessionID, modelName string,
	svcWf service.SpawnerWorkflowDef,
	workflows map[string]spawner.WorkflowDef,
	agents map[string]spawner.AgentConfig,
	pool *db.Pool,
	projectRoot string,
	modelConfigs map[string]spawner.ModelConfig,
	claudeSettingsJSON string,
) ([]string, error) {
	var prompt string

	if req.PlanMode {
		// Plan mode: build a planning prompt with ticket context
		prompt = buildPlanPrompt(req)
	} else {
		// Interactive: expand the L0 agent's template
		if len(svcWf.Phases) == 0 {
			return nil, fmt.Errorf("workflow has no phases")
		}
		l0Agent := svcWf.Phases[0].Agent
		l0Model := "opus_4_7"
		if cfg, ok := agents[l0Agent]; ok && cfg.Model != "" {
			l0Model = cfg.Model
		}
		cliName := cliNameFromModelConfigs(modelConfigs, l0Model)
		modelID := fmt.Sprintf("%s:%s", cliName, l0Model)

		// Template-only spawner for prompt expansion. Manifest fields not needed (CLI-only).
		// Callbacks wired for uniformity; this spawner never registers sessions.
		tmplWfiID := wi.ID
		sp := spawner.New(spawner.Config{
			Workflows:    workflows,
			Agents:       agents,
			DataPath:     o.dataPath,
			WSHub:        o.wsHub,
			Pool:         pool,
			Clock:        o.clock,
			ModelConfigs: modelConfigs,
			OnSessionRegister: func(sid string, s *spawner.Spawner) {
				o.mu.Lock()
				if rs, ok := o.runs[tmplWfiID]; ok {
					rs.spawners[sid] = s
				}
				o.mu.Unlock()
			},
			OnSessionUnregister: func(sid string) {
				o.mu.Lock()
				if rs, ok := o.runs[tmplWfiID]; ok {
					delete(rs.spawners, sid)
				}
				o.mu.Unlock()
			},
		})

		tmpl, _, err := sp.LoadTemplate(l0Agent, req.TicketID, req.ProjectID, "", sessionID, req.WorkflowName, modelID, l0Agent, wi.ID, nil, 0)
		if err != nil {
			return nil, fmt.Errorf("failed to load L0 template: %w", err)
		}

		prompt = "You are in an interactive session. The user will guide the work directly.\n" +
			"When the user is done, they will exit the session.\n\n" + tmpl
	}

	// Write prompt to a temp file so Claude can read it as initial context.
	// We don't use -p (--print) because that makes Claude non-interactive.
	promptFile, err := os.CreateTemp("", "nrf-interactive-*.md")
	if err != nil {
		return nil, fmt.Errorf("failed to create prompt file: %w", err)
	}
	if _, err := promptFile.WriteString(prompt); err != nil {
		promptFile.Close()
		os.Remove(promptFile.Name())
		return nil, fmt.Errorf("failed to write prompt file: %w", err)
	}
	promptFile.Close()

	// Resolve mapped model: DB-sourced MappedModel wins, else fall back to
	// the Claude adapter's hardcoded mapping. Without this, the raw nrflo ID
	// (e.g. "opus_4_7") reaches `claude --model` and the CLI rejects it.
	ptyModel := modelName
	if cfg, ok := modelConfigs[modelName]; ok && cfg.MappedModel != "" {
		ptyModel = cfg.MappedModel
	} else {
		ptyModel = (&spawner.ClaudeAdapter{}).MapModel(modelName)
	}

	args := []string{
		"--session-id", sessionID,
		"--model", ptyModel,
		"--append-system-prompt-file", promptFile.Name(),
	}
	if req.PlanMode {
		// Plan mode: --permission-mode plan handles permissions on its own.
		// Do NOT use --dangerously-skip-permissions — it overrides plan mode.
		args = append(args, "--permission-mode", "plan", "--disallowed-tools", "ExitPlanMode")
	} else {
		args = append(args, "--dangerously-skip-permissions")
	}
	if claudeSettingsJSON != "" {
		args = append(args, "--settings", claudeSettingsJSON)
	}

	return args, nil
}

// buildPlanPrompt creates the prompt for plan mode PTY sessions.
func buildPlanPrompt(req RunRequest) string {
	prompt := "You are in a planning session. Create a detailed implementation plan.\n\n"

	if req.TicketID != "" {
		prompt += fmt.Sprintf("Ticket: %s\n", req.TicketID)
	}
	if req.Instructions != "" {
		prompt += fmt.Sprintf("\nInstructions:\n%s\n", req.Instructions)
	}

	prompt += "\nWhen your plan is complete, exit the session."
	return prompt
}

// waitForInteractivePreStep blocks until the interactive PTY session completes
// or the context is cancelled. Returns true if completed normally, false if cancelled.
func waitForInteractivePreStep(ctx context.Context, pre *interactivePreStep) bool {
	select {
	case <-pre.waitCh:
		return true
	case <-ctx.Done():
		return false
	}
}

// handlePlanModePostStep reads the plan file and stores it as user_instructions.
// Returns an error if no plan file is found.
func handlePlanModePostStep(sessionID, projectRoot string, pool *db.Pool, wfiID string, clk clock.Clock) error {
	planContent := readPlanFile(sessionID, projectRoot)
	if planContent == "" {
		return fmt.Errorf("no plan file found for session %s", sessionID)
	}

	findingRepo := repo.NewFindingRepo(pool, clk)
	instrVal, _ := json.Marshal(planContent)
	if err := findingRepo.Upsert("workflow_instance", wfiID, "user_instructions", instrVal,
		repo.Denorm{WorkflowInstanceID: wfiID},
		repo.Actor{Source: "orchestrator"}); err != nil {
		return fmt.Errorf("failed to store user_instructions finding: %w", err)
	}

	if err := repo.NewAgentSessionRepo(pool, clk).UpdateStatusToInteractiveCompleted(sessionID); err != nil {
		logger.Error(context.Background(), "failed to mark planner session interactive_completed", "session_id", sessionID, "err", err)
		return err
	}

	logger.Info(context.Background(), "plan file stored as user_instructions", "wfi_id", wfiID, "plan_length", len(planContent))
	return nil
}
