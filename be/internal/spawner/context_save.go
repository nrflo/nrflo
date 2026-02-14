package spawner

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"syscall"
	"time"

	"be/internal/db"
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

	// 2. Flush messages and context from the killed process
	s.saveMessages(proc)
	s.saveContextLeft(proc)

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

	savePrompt := fmt.Sprintf(
		"Save a summary of all your current work progress by running: "+
			"nrworkflow findings add %s %s to_resume:<your summary of all progress, findings, and context> -w %s --model %s"+
			" — then call: nrworkflow agent continue %s %s -w %s --model %s",
		req.TicketID, proc.agentType, req.WorkflowName, proc.modelID,
		req.TicketID, proc.agentType, req.WorkflowName, proc.modelID)

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
	select {
	case <-resumeDone:
		exitCode := 0
		if resumeCmd.ProcessState != nil {
			exitCode = resumeCmd.ProcessState.ExitCode()
		}
		logger.Info(ctx, "context save completed", "exit_code", exitCode, "duration", time.Since(startTime).Round(time.Millisecond), "session_id", proc.sessionID)
	case <-time.After(resumeSaveTimeout):
		logger.Error(ctx, "context save timed out", "timeout", resumeSaveTimeout, "session_id", proc.sessionID)
		if resumeCmd.Process != nil {
			resumeCmd.Process.Kill()
		}
		<-resumeDone
	}

	// 5. Wait briefly for `agent continue` to propagate through socket
	time.Sleep(2 * time.Second)

	// 6. Check if findings were saved
	s.logFindingsStatus(ctx, proc)

	// 7. Register stop
	s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName,
		proc.sessionID, proc.agentID, "continue", "low_context", proc.modelID)

	proc.finalStatus = "CONTINUE"
	logger.Info(ctx, "context save flow complete, relaunching", "session_id", proc.sessionID)
}

// logFindingsStatus checks and logs whether the session has findings after resume-save
func (s *Spawner) logFindingsStatus(ctx context.Context, proc *processInfo) {
	database, err := db.Open(s.config.DataPath)
	if err != nil {
		logger.Error(ctx, "failed to open DB for findings check", "err", err, "session_id", proc.sessionID)
		return
	}
	defer database.Close()

	var findings sql.NullString
	err = database.QueryRow("SELECT findings FROM agent_sessions WHERE id = ?", proc.sessionID).Scan(&findings)
	if err != nil {
		logger.Error(ctx, "failed to query findings", "err", err, "session_id", proc.sessionID)
		return
	}

	if !findings.Valid || findings.String == "" || findings.String == "{}" {
		logger.Warn(ctx, "no findings saved by resume agent", "session_id", proc.sessionID)
	} else {
		logger.Info(ctx, "findings saved", "bytes", len(findings.String), "session_id", proc.sessionID)
	}
}
