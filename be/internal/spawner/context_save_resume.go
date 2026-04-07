package spawner

import (
	"bufio"
	"context"
	"strings"
	"time"

	"be/internal/logger"
)

const resumeSaveTimeout = 3 * time.Minute

// contextSaveViaResume handles the resume-based context save flow.
// Resumes the same Claude session with a save prompt so the agent writes its own
// to_resume findings. Only works for Claude CLI (SupportsResume); other CLIs skip
// context save and relaunch without previous data.
//
// Called after kill+flush in initiateContextSave when ContextSaveViaAgent is false.
func (s *Spawner) contextSaveViaResume(ctx context.Context, proc *processInfo, req SpawnRequest) {
	cliName, _ := parseModelID(proc.modelID)
	adapter, err := GetCLIAdapter(cliName)
	if err != nil || !adapter.SupportsResume() {
		logger.Warn(ctx, "CLI does not support resume, relaunching without save", "cli", cliName, "session_id", proc.sessionID)
		s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName,
			proc.sessionID, proc.agentID, "continue", "low_context", proc.modelID)
		proc.finalStatus = "CONTINUE"
		return
	}

	savePrompt := buildSavePrompt()
	logger.Info(ctx, "context save prompt", "prompt", savePrompt, "session_id", proc.sessionID)

	resumeCmd := adapter.BuildResumeCommand(ResumeOptions{
		SessionID:    proc.sessionID,
		Prompt:       savePrompt,
		WorkDir:      s.config.ProjectRoot,
		Env:          proc.cmd.Env,
		SettingsJSON: s.config.ClaudeSettingsJSON,
	})

	resumeCmd.Stdin = strings.NewReader(savePrompt)

	stdout, err := resumeCmd.StdoutPipe()
	if err != nil {
		logger.Error(ctx, "context save failed to create stdout pipe", "err", err, "session_id", proc.sessionID)
		s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName,
			proc.sessionID, proc.agentID, "continue", "low_context", proc.modelID)
		proc.finalStatus = "CONTINUE"
		return
	}
	stderr, stderrErr := resumeCmd.StderrPipe()
	if stderrErr != nil {
		logger.Error(ctx, "context save failed to create stderr pipe", "err", stderrErr, "session_id", proc.sessionID)
	}

	if err := resumeCmd.Start(); err != nil {
		logger.Error(ctx, "context save failed to start resume", "err", err, "session_id", proc.sessionID)
		s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName,
			proc.sessionID, proc.agentID, "continue", "low_context", proc.modelID)
		proc.finalStatus = "CONTINUE"
		return
	}
	logger.Info(ctx, "context save resume started", "pid", resumeCmd.Process.Pid, "cmd", resumeCmd.Args, "session_id", proc.sessionID)

	// Capture and log resume stdout
	go func() {
		scanner := bufio.NewScanner(stdout)
		buf := make([]byte, 0, 1024*1024)
		scanner.Buffer(buf, 10*1024*1024)
		lineCount := 0
		for scanner.Scan() {
			lineCount++
			line := scanner.Text()
			if len(line) > 500 {
				line = line[:250] + "..." + line[len(line)-250:]
			}
			logger.Info(ctx, "context save output", "line", lineCount, "content", line, "session_id", proc.sessionID)
		}
		logger.Info(ctx, "context save output finished", "total_lines", lineCount, "session_id", proc.sessionID)
	}()

	if stderrErr == nil {
		go func() {
			scanner := bufio.NewScanner(stderr)
			scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
			for scanner.Scan() {
				logger.Warn(ctx, "context save stderr", "content", scanner.Text(), "session_id", proc.sessionID)
			}
		}()
	}

	// Wait for resume process with timeout
	resumeDone := make(chan struct{})
	go func() {
		resumeCmd.Wait()
		close(resumeDone)
	}()

	startTime := time.Now()
	resumeSucceeded := false
	select {
	case <-resumeDone:
		exitCode := 0
		if resumeCmd.ProcessState != nil {
			exitCode = resumeCmd.ProcessState.ExitCode()
		}
		resumeSucceeded = exitCode == 0
		if !resumeSucceeded {
			logger.Error(ctx, "context save resume exited with error", "exit_code", exitCode, "duration", time.Since(startTime).Round(time.Millisecond), "session_id", proc.sessionID)
		} else {
			logger.Info(ctx, "context save completed", "exit_code", exitCode, "duration", time.Since(startTime).Round(time.Millisecond), "session_id", proc.sessionID)
		}
	case <-time.After(resumeSaveTimeout):
		logger.Error(ctx, "context save timed out", "timeout", resumeSaveTimeout, "session_id", proc.sessionID)
		if resumeCmd.Process != nil {
			resumeCmd.Process.Kill()
		}
		<-resumeDone
	}

	findingsSaved := s.checkToResumeFindings(ctx, proc)
	if resumeSucceeded && !findingsSaved {
		logger.Warn(ctx, "resume succeeded but to_resume findings not saved, previous data will be empty on relaunch", "session_id", proc.sessionID)
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
		"1. nrflow findings add to_resume \"<detailed summary of all progress, findings, files changed, and remaining work>\"\n" +
		"2. nrflow agent continue\n\n" +
		"The session and workflow context are provided via environment variables. Do NOT add any extra flags."
}
