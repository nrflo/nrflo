package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/spawner/apirun/provider"
)

// interactivePreStep holds state for the interactive/plan pre-step that runs
// before the normal layer execution loop.
type interactivePreStep struct {
	sessionID string
	waitCh    <-chan struct{} // blocks until PTY session completes
	spawner   *spawner.Spawner
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
	_ string,
	modelConfigs map[string]spawner.ModelConfig,
	apiModelConfigs map[string]spawner.APIModelConfig,
	claudeSettingsJSON string,
) ([]string, error) {
	var prompt string
	var interactiveSystemPromptOverrideFile string

	if req.PlanMode {
		// Plan mode: build a planning prompt with ticket context
		prompt = buildPlanPrompt(req)
	} else {
		// Interactive: expand the L0 agent's template
		if len(svcWf.Phases) == 0 {
			return nil, fmt.Errorf("workflow has no phases")
		}
		l0Agent := svcWf.Phases[0].Agent
		l0Model := "opus_4_8"
		if cfg, ok := agents[l0Agent]; ok && cfg.Model != "" {
			l0Model = cfg.Model
		}
		cliName := cliNameFromModelConfigs(modelConfigs, l0Model)
		modelID := fmt.Sprintf("%s:%s", cliName, l0Model)

		// Template-only spawner for prompt expansion. Manifest fields not needed (CLI-only).
		// Callbacks wired for uniformity; this spawner never registers sessions.
		tmplWfiID := wi.ID
		tmplPool := pool
		sp := spawner.New(spawner.Config{
			Workflows:       workflows,
			Agents:          agents,
			DataPath:        o.dataPath,
			WSHub:           o.wsHub,
			Pool:            pool,
			Clock:           o.clock,
			ModelConfigs:    modelConfigs,
			APIModelConfigs: apiModelConfigs,
			BuildAPIProvider: func(ctx context.Context, providerName, projectID string) (provider.Provider, error) {
				return buildAPIProvider(ctx, tmplPool, o.clock, providerName, projectID)
			},
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

		tmpl, _, systemPromptOverride, err := sp.LoadTemplate(l0Agent, req.TicketID, req.ProjectID, "", sessionID, req.WorkflowName, modelID, l0Agent, wi.ID, nil, 0)
		if err != nil {
			return nil, fmt.Errorf("failed to load L0 template: %w", err)
		}

		prompt = "You are in an interactive session. The user will guide the work directly.\n" +
			"When the user is done, they will exit the session.\n\n" + tmpl

		// Write system-prompt override to a temp file when the model is Claude and the override setting is on.
		if systemPromptOverride != "" {
			of, ofErr := os.CreateTemp("", "nrf-interactive-system-prompt-*.md")
			if ofErr != nil {
				return nil, fmt.Errorf("failed to create system-prompt override file: %w", ofErr)
			}
			if _, ofErr = of.WriteString(systemPromptOverride); ofErr != nil {
				of.Close()
				os.Remove(of.Name())
				return nil, fmt.Errorf("failed to write system-prompt override file: %w", ofErr)
			}
			of.Close()
			interactiveSystemPromptOverrideFile = of.Name()
		}
	}

	// Write prompt to a temp file so Claude can read it as initial context.
	// We don't use -p (--print) because that makes Claude non-interactive.
	promptFile, err := os.CreateTemp("", "nrf-interactive-*.md")
	if err != nil {
		if interactiveSystemPromptOverrideFile != "" {
			os.Remove(interactiveSystemPromptOverrideFile)
		}
		return nil, fmt.Errorf("failed to create prompt file: %w", err)
	}
	if _, err := promptFile.WriteString(prompt); err != nil {
		promptFile.Close()
		os.Remove(promptFile.Name())
		if interactiveSystemPromptOverrideFile != "" {
			os.Remove(interactiveSystemPromptOverrideFile)
		}
		return nil, fmt.Errorf("failed to write prompt file: %w", err)
	}
	promptFile.Close()

	// Resolve mapped model: DB-sourced MappedModel wins, else fall back to
	// the Claude adapter's hardcoded mapping. Without this, the raw nrflo ID
	// (e.g. "opus_4_8") reaches `claude --model` and the CLI rejects it.
	var ptyModel string
	if cfg, ok := modelConfigs[modelName]; ok && cfg.MappedModel != "" {
		ptyModel = cfg.MappedModel
	} else {
		ptyModel = (&spawner.ClaudeAdapter{}).MapModel(modelName)
	}

	args := []string{
		"--session-id", sessionID,
		"--model", ptyModel,
	}
	if interactiveSystemPromptOverrideFile != "" {
		args = append(args, "--system-prompt-file", interactiveSystemPromptOverrideFile)
	}
	args = append(args, "--append-system-prompt-file", promptFile.Name())
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
