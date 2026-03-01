package spawner

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"syscall"
	"time"

	"be/internal/logger"
)

const (
	resumeSaveTimeout = 3 * time.Minute
	killGracePeriod   = 5 * time.Second
)

// initiateContextSave handles the low-context save flow:
// 1. Kill the running agent
// 2. Resume with save instructions (if CLI supports it)
// 3. Wait for the resumed agent to call `agent continue`
// 4. Register stop and set finalStatus = "CONTINUE" to trigger relaunch
//
// processDoneCh is the original process's done channel (closed by the wait goroutine).
// completeCh is the replacement channel; closed when the full flow finishes, signaling monitorAll.
func (s *Spawner) initiateContextSave(ctx context.Context, proc *processInfo, req SpawnRequest, processDoneCh, completeCh chan struct{}) {
	defer close(completeCh)

	logger.Warn(ctx, "low context detected", "context_left", proc.contextLeft, "session_id", proc.sessionID)

	// 1. Kill the running agent: stop container (if Docker) → SIGTERM → wait → SIGKILL
	StopContainer(proc.containerName)
	if proc.cmd.Process != nil {
		proc.cmd.Process.Signal(syscall.SIGTERM)
	}
	select {
	case <-processDoneCh:
		// Original process exited
	case <-time.After(killGracePeriod):
		if proc.cmd.Process != nil {
			proc.cmd.Process.Kill()
		}
		<-processDoneCh
	}

	// 2. Flush messages from the killed process
	s.saveMessages(proc)

	// 3. Resume with save instructions (only for CLIs that support it)
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
		SessionID: proc.sessionID,
		Prompt:    savePrompt,
		WorkDir:   s.config.ProjectRoot,
		Env:       proc.cmd.Env,
	})

	// Pipe prompt via stdin (same as normal spawn)
	resumeCmd.Stdin = strings.NewReader(savePrompt)

	// Capture stdout/stderr for monitoring
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

	// Capture and log resume stderr
	if stderrErr == nil {
		go func() {
			scanner := bufio.NewScanner(stderr)
			scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
			for scanner.Scan() {
				logger.Warn(ctx, "context save stderr", "content", scanner.Text(), "session_id", proc.sessionID)
			}
		}()
	}

	// 4. Wait for resume process to finish (with timeout)
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

	// 5. Check if to_resume findings were actually saved
	findingsSaved := s.checkToResumeFindings(ctx, proc)
	if resumeSucceeded && !findingsSaved {
		logger.Warn(ctx, "resume succeeded but to_resume findings not saved, previous data will be empty on relaunch", "session_id", proc.sessionID)
	}
	if !resumeSucceeded && !findingsSaved {
		logger.Warn(ctx, "resume failed and no findings saved, relaunching without previous data", "session_id", proc.sessionID)
	}

	// 6. Register stop
	s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName,
		proc.sessionID, proc.agentID, "continue", "low_context", proc.modelID)

	proc.finalStatus = "CONTINUE"
	logger.Info(ctx, "context save flow complete, relaunching", "findings_saved", findingsSaved, "session_id", proc.sessionID)
}

// checkToResumeFindings checks whether the session has to_resume findings after resume-save.
// Returns true if the to_resume key was found in the session's findings.
func (s *Spawner) checkToResumeFindings(ctx context.Context, proc *processInfo) bool {
	pool := s.pool()
	if pool == nil {
		logger.Error(ctx, "no database pool for findings check", "session_id", proc.sessionID)
		return false
	}

	var findingsRaw sql.NullString
	err := pool.QueryRow("SELECT findings FROM agent_sessions WHERE id = ?", proc.sessionID).Scan(&findingsRaw)
	if err != nil {
		logger.Error(ctx, "failed to query findings", "err", err, "session_id", proc.sessionID)
		return false
	}

	if !findingsRaw.Valid || findingsRaw.String == "" || findingsRaw.String == "{}" {
		logger.Warn(ctx, "no findings saved by resume agent", "session_id", proc.sessionID)
		return false
	}

	var findings map[string]interface{}
	if json.Unmarshal([]byte(findingsRaw.String), &findings) != nil {
		logger.Warn(ctx, "failed to parse findings JSON", "session_id", proc.sessionID)
		return false
	}

	toResume, ok := findings["to_resume"]
	if !ok {
		logger.Warn(ctx, "findings saved but to_resume key missing", "keys_count", len(findings), "session_id", proc.sessionID)
		return false
	}

	str, isStr := toResume.(string)
	if !isStr || str == "" {
		logger.Warn(ctx, "to_resume key present but empty or non-string", "session_id", proc.sessionID)
		return false
	}

	logger.Info(ctx, "to_resume findings saved", "bytes", len(str), "session_id", proc.sessionID)
	return true
}

// buildSavePrompt constructs the prompt sent to a resumed agent to save its progress.
// The CLI reads NRWF_SESSION_ID and NRWF_WORKFLOW_INSTANCE_ID from env vars (inherited from the original process).
func buildSavePrompt() string {
	return "URGENT: Save a summary of ALL your current work progress immediately. " +
		"Run these two commands in order:\n\n" +
		"1. nrworkflow findings add to_resume \"<detailed summary of all progress, findings, files changed, and remaining work>\"\n" +
		"2. nrworkflow agent continue\n\n" +
		"The session and workflow context are provided via environment variables. Do NOT add any extra flags."
}
