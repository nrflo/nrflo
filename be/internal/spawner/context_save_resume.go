package spawner

import (
	"context"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/google/uuid"

	"be/internal/logger"
)

const resumeSaveTimeout = 3 * time.Minute

// sessionIDForResume returns the session ID to pass as ResumeSessionID to
// BuildInteractiveCommand. Uses proc.sessionID when the adapter tracks custom
// session IDs (Claude), and proc.externalSessionID (codex-assigned thread_id)
// otherwise.
func sessionIDForResume(adapter CLIAdapter, proc *processInfo) string {
	if adapter.SupportsSessionID() {
		return proc.sessionID
	}
	return proc.externalSessionID
}

// contextSaveViaResume handles the resume-based context save flow.
// Spawns a one-shot cli_interactive PTY session that resumes the original
// session with the save prompt. Falls back to contextSaveViaAgent on any
// failure.
//
// Called after kill+flush in initiateContextSave when ContextSaveViaAgent is false.
func (s *Spawner) contextSaveViaResume(ctx context.Context, proc *processInfo, req SpawnRequest) {
	cliName, model := parseModelID(proc.modelID)
	adapter, err := GetCLIAdapter(cliName)
	if err != nil || !adapter.SupportsResume() {
		logger.Warn(ctx, "CLI does not support resume, falling back to agent save", "cli", cliName, "session_id", proc.sessionID)
		s.contextSaveViaAgent(ctx, proc, req)
		return
	}

	resumeSessionID := sessionIDForResume(adapter, proc)
	if resumeSessionID == "" {
		logger.Warn(ctx, "no resume session ID available, falling back to agent save", "cli", cliName, "session_id", proc.sessionID)
		s.contextSaveViaAgent(ctx, proc, req)
		return
	}

	savePrompt := buildSavePrompt()
	logger.Info(ctx, "context save prompt", "prompt", savePrompt, "session_id", proc.sessionID)

	var mappedModel, reasoningEffort string
	if cfg, ok := s.config.ModelConfigs[model]; ok {
		mappedModel = cfg.MappedModel
		reasoningEffort = cfg.ReasoningEffort
	}
	if mappedModel == "" {
		mappedModel = adapter.MapModel(model)
	}

	spawnOpts := InteractiveSpawnOptions{
		SessionID:       uuid.New().String(),
		Model:           mappedModel,
		ReasoningEffort: reasoningEffort,
		WorkDir:         s.config.ProjectRoot,
		Env:             proc.env,
		SettingsJSON:    s.config.ClaudeSettingsJSON,
		ResumeSessionID: resumeSessionID,
	}
	// Adapters that deliver the prompt via argv (codex) get it in Prompt;
	// others receive it via PTY stdin write below.
	if adapter.DeliversPromptInline() {
		spawnOpts.Prompt = savePrompt
	}

	resumeCmd := adapter.BuildInteractiveCommand(spawnOpts)
	resumeCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	ptyCh, err := pty.Start(resumeCmd)
	if err != nil {
		logger.Error(ctx, "context save failed to start resume PTY, falling back to agent save", "err", err, "session_id", proc.sessionID)
		s.contextSaveViaAgent(ctx, proc, req)
		return
	}
	logger.Info(ctx, "context save resume started", "pid", resumeCmd.Process.Pid, "cmd", resumeCmd.Args, "session_id", proc.sessionID)

	// Drain PTY output to prevent blocking when the buffer fills.
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := ptyCh.Read(buf); err != nil {
				return
			}
		}
	}()

	// For adapters that deliver the prompt via PTY stdin, write after a short
	// bootstrap delay to let the TUI initialise.
	if !adapter.DeliversPromptInline() {
		go func() {
			time.Sleep(2 * time.Second)
			_, _ = ptyCh.Write([]byte(savePrompt + "\r"))
		}()
	}

	// Wait for resume process with timeout.
	resumeDone := make(chan struct{})
	go func() {
		resumeCmd.Wait() //nolint:errcheck
		close(resumeDone)
	}()

	startTime := time.Now()
	select {
	case <-resumeDone:
		ptyCh.Close()
		exitCode := 0
		if resumeCmd.ProcessState != nil {
			exitCode = resumeCmd.ProcessState.ExitCode()
		}
		if exitCode != 0 {
			logger.Error(ctx, "context save resume exited with error, falling back to agent save", "exit_code", exitCode, "duration", time.Since(startTime).Round(time.Millisecond), "session_id", proc.sessionID)
			s.contextSaveViaAgent(ctx, proc, req)
			return
		}
		logger.Info(ctx, "context save completed", "exit_code", exitCode, "duration", time.Since(startTime).Round(time.Millisecond), "session_id", proc.sessionID)
	case <-time.After(resumeSaveTimeout):
		logger.Error(ctx, "context save timed out, falling back to agent save", "timeout", resumeSaveTimeout, "session_id", proc.sessionID)
		if resumeCmd.Process != nil {
			_ = syscall.Kill(-resumeCmd.Process.Pid, syscall.SIGKILL)
		}
		ptyCh.Close()
		<-resumeDone
		s.contextSaveViaAgent(ctx, proc, req)
		return
	}

	findingsSaved := s.checkToResumeFindings(ctx, proc)
	if !findingsSaved {
		logger.Warn(ctx, "resume succeeded but to_resume findings not saved, falling back to agent save", "session_id", proc.sessionID)
		s.contextSaveViaAgent(ctx, proc, req)
		return
	}

	s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName,
		proc.sessionID, proc.agentID, "continue", "low_context", proc.modelID)

	proc.finalStatus = "CONTINUE"
	logger.Info(ctx, "context save flow complete, relaunching", "findings_saved", findingsSaved, "session_id", proc.sessionID)
}

// buildSavePrompt constructs the prompt sent to a resumed agent to save its progress.
// The CLI reads NRF_SESSION_ID and NRF_WORKFLOW_INSTANCE_ID from env vars (inherited from the original process).
func buildSavePrompt() string {
	return "URGENT: Save a summary of ALL your current work progress immediately. " +
		"Run these two commands in order:\n\n" +
		"1. nrflo findings add to_resume \"<detailed summary of all progress, findings, files changed, and remaining work>\"\n" +
		"2. nrflo agent continue\n\n" +
		"The session and workflow context are provided via environment variables. Do NOT add any extra flags."
}
