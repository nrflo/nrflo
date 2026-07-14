package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"be/internal/db"
	"be/internal/model"
	ptyPkg "be/internal/pty"
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
	adapter   spawner.CLIAdapter
	planFile  string
	cleanup   func()
}

// buildInteractiveLaunch resolves the CLI adapter for the workflow's L0 model
// (claude or codex, via spawner.GetCLIAdapter — the only name switch) and
// delegates to adapter.PrepareUserSession for the interactive/plan PTY
// launch. Returns a cleanup func that removes the prompt/system-prompt temp
// files this function writes and calls the adapter's own cleanup.
func (o *Orchestrator) buildInteractiveLaunch(
	req RunRequest,
	wi *model.WorkflowInstance,
	sessionID, modelName string,
	svcWf service.SpawnerWorkflowDef,
	workflows map[string]spawner.WorkflowDef,
	agents map[string]spawner.AgentConfig,
	pool *db.Pool,
	projectRoot string,
	modelConfigs map[string]spawner.ModelConfig,
	apiModelConfigs map[string]spawner.APIModelConfig,
	claudeSettingsJSON string,
) (ptyPkg.Launch, spawner.CLIAdapter, string, func(), error) {
	cliName := cliNameFromModelConfigs(modelConfigs, modelName)
	adapter, err := spawner.GetCLIAdapter(cliName)
	if err != nil {
		return ptyPkg.Launch{}, nil, "", nil, fmt.Errorf("resolve CLI adapter %q: %w", cliName, err)
	}

	// Model resolution: DB cli_models row wins, else the adapter's own
	// mapping; effort/fallback ride along from the same row so registry ids
	// that are many-to-one on mapped_model (e.g. codex_gpt55_high/_normal)
	// don't silently collapse to the same launch.
	var mappedModel, reasoningEffort, fallbackModels string
	if cfg, ok := modelConfigs[modelName]; ok {
		mappedModel = cfg.MappedModel
		reasoningEffort = cfg.ReasoningEffort
		fallbackModels = cfg.FallbackModels
	}
	if mappedModel == "" {
		mappedModel = adapter.MapModel(modelName)
	}

	var planFile string
	if req.PlanMode {
		planFile = filepath.Join(projectRoot, ".nrflo-plan-"+sessionID+".md")
	}
	planCapture := spawner.PlanCaptureOptions{SessionID: sessionID, WorkDir: projectRoot, PlanFile: planFile}

	var prompt string
	var interactiveSystemPromptOverrideFile string

	if req.PlanMode {
		// Plan mode: build a planning prompt with ticket context, plus
		// adapter-specific instructions for CLIs with no native plan store.
		prompt = buildPlanPrompt(req) + adapter.PlanPromptSuffix(planCapture)
	} else {
		// Interactive: expand the L0 agent's template.
		if len(svcWf.Phases) == 0 {
			return ptyPkg.Launch{}, nil, "", nil, fmt.Errorf("workflow has no phases")
		}
		l0Agent := svcWf.Phases[0].Agent

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
				return service.BuildAPIProvider(ctx, tmplPool, o.clock, providerName, projectID)
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

		modelID := fmt.Sprintf("%s:%s", cliName, modelName)
		tmpl, _, systemPromptOverride, err := sp.LoadTemplate(l0Agent, req.TicketID, req.ProjectID, "", sessionID, req.WorkflowName, modelID, l0Agent, wi.ID, nil, 0)
		if err != nil {
			return ptyPkg.Launch{}, nil, "", nil, fmt.Errorf("failed to load L0 template: %w", err)
		}

		prompt = "You are in an interactive session. The user will guide the work directly.\n" +
			"When the user is done, they will exit the session.\n\n" + tmpl

		// Write system-prompt override to a temp file when the override setting is on.
		if systemPromptOverride != "" {
			of, ofErr := os.CreateTemp("", "nrf-interactive-system-prompt-*.md")
			if ofErr != nil {
				return ptyPkg.Launch{}, nil, "", nil, fmt.Errorf("failed to create system-prompt override file: %w", ofErr)
			}
			if _, ofErr = of.WriteString(systemPromptOverride); ofErr != nil {
				of.Close()
				os.Remove(of.Name())
				return ptyPkg.Launch{}, nil, "", nil, fmt.Errorf("failed to write system-prompt override file: %w", ofErr)
			}
			of.Close()
			interactiveSystemPromptOverrideFile = of.Name()
		}
	}

	// Write prompt to a temp file so the CLI can read it as initial context.
	promptFile, err := os.CreateTemp("", "nrf-interactive-*.md")
	if err != nil {
		if interactiveSystemPromptOverrideFile != "" {
			os.Remove(interactiveSystemPromptOverrideFile)
		}
		return ptyPkg.Launch{}, nil, "", nil, fmt.Errorf("failed to create prompt file: %w", err)
	}
	if _, err := promptFile.WriteString(prompt); err != nil {
		promptFile.Close()
		os.Remove(promptFile.Name())
		if interactiveSystemPromptOverrideFile != "" {
			os.Remove(interactiveSystemPromptOverrideFile)
		}
		return ptyPkg.Launch{}, nil, "", nil, fmt.Errorf("failed to write prompt file: %w", err)
	}
	promptFile.Close()

	opts := spawner.UserSessionOptions{
		SessionID:                sessionID,
		Model:                    mappedModel,
		ReasoningEffort:          reasoningEffort,
		FallbackModels:           fallbackModels,
		WorkDir:                  projectRoot,
		Prompt:                   prompt,
		PromptFile:               promptFile.Name(),
		SystemPromptOverrideFile: interactiveSystemPromptOverrideFile,
		SettingsJSON:             claudeSettingsJSON,
		PlanMode:                 req.PlanMode,
		PlanFile:                 planFile,
	}

	launch, adapterCleanup, err := adapter.PrepareUserSession(opts)
	if err != nil {
		os.Remove(promptFile.Name())
		if interactiveSystemPromptOverrideFile != "" {
			os.Remove(interactiveSystemPromptOverrideFile)
		}
		return ptyPkg.Launch{}, nil, "", nil, fmt.Errorf("failed to prepare user session: %w", err)
	}
	if adapterCleanup == nil {
		adapterCleanup = func() {}
	}

	promptFilePath := promptFile.Name()
	overridePath := interactiveSystemPromptOverrideFile
	cleanup := func() {
		os.Remove(promptFilePath)
		if overridePath != "" {
			os.Remove(overridePath)
		}
		adapterCleanup()
	}

	return launch, adapter, planFile, cleanup, nil
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
