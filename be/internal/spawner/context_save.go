package spawner

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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

	// 1. Kill the running agent: SIGTERM → wait → SIGKILL
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

	savePrompt := buildSavePrompt(req.TicketID, proc.agentType, req.WorkflowName, proc.modelID)

	resumeCmd := adapter.BuildResumeCommand(ResumeOptions{
		SessionID: proc.sessionID,
		Prompt:    savePrompt,
		WorkDir:   s.config.ProjectRoot,
		Env:       proc.cmd.Env,
	})

	// Capture stdout/stderr for monitoring
	stdout, err := resumeCmd.StdoutPipe()
	if err != nil {
		logger.Error(ctx, "context save failed to create stdout pipe", "err", err, "session_id", proc.sessionID)
		s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName,
			proc.sessionID, proc.agentID, "continue", "low_context", proc.modelID)
		proc.finalStatus = "CONTINUE"
		return
	}

	if err := resumeCmd.Start(); err != nil {
		logger.Error(ctx, "context save failed to start resume", "err", err, "session_id", proc.sessionID)
		s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName,
			proc.sessionID, proc.agentID, "continue", "low_context", proc.modelID)
		proc.finalStatus = "CONTINUE"
		return
	}
	logger.Info(ctx, "context save resume started", "pid", resumeCmd.Process.Pid, "session_id", proc.sessionID)

	// Drain resume stdout to prevent blocking
	go func() {
		scanner := bufio.NewScanner(stdout)
		buf := make([]byte, 0, 1024*1024)
		scanner.Buffer(buf, 10*1024*1024)
		for scanner.Scan() {
		}
	}()

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
func buildSavePrompt(ticketID, agentType, workflowName, modelID string) string {
	ticketArg := ticketID
	if ticketID == "" {
		ticketArg = "-T"
	}
	return fmt.Sprintf(
		"Save a summary of all your current work progress by running: "+
			"nrworkflow findings add %s %s to_resume:<your summary of all progress, findings, and context> -w %s --model %s"+
			" — then call: nrworkflow agent continue %s %s -w %s --model %s",
		ticketArg, agentType, workflowName, modelID,
		ticketArg, agentType, workflowName, modelID)
}
