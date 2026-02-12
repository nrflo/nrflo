package spawner

import (
	"bufio"
	"fmt"
	"os"
	"syscall"
	"time"
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
func (s *Spawner) initiateContextSave(proc *processInfo, req SpawnRequest, processDoneCh, completeCh chan struct{}) {
	defer close(completeCh)

	prefix := s.formatPrefix(proc)
	fmt.Printf("  %s [context-save] Low context detected (%d%% remaining), initiating save...\n",
		prefix, proc.contextLeft)

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
		// CLI doesn't support resume — just mark as continue with whatever findings exist
		fmt.Printf("  %s [context-save] CLI '%s' does not support resume, relaunching without save\n", prefix, cliName)
		s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName,
			proc.sessionID, proc.agentID, "continue", "low_context", proc.modelID)
		proc.finalStatus = "CONTINUE"
		return
	}

	savePrompt := fmt.Sprintf(
		"Save all your current work progress to findings using nrworkflow findings commands, "+
			"then call: nrworkflow agent continue %s %s -w %s --model %s",
		req.TicketID, proc.agentType, req.WorkflowName, proc.modelID)

	resumeCmd := adapter.BuildResumeCommand(ResumeOptions{
		SessionID: proc.sessionID,
		Prompt:    savePrompt,
		WorkDir:   s.config.ProjectRoot,
		Env:       proc.cmd.Env,
	})

	fmt.Printf("  %s [context-save] Resuming session for findings save...\n", prefix)

	// Capture stdout/stderr for monitoring
	stdout, err := resumeCmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "  %s [context-save] Failed to create stdout pipe: %v\n", prefix, err)
		s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName,
			proc.sessionID, proc.agentID, "continue", "low_context", proc.modelID)
		proc.finalStatus = "CONTINUE"
		return
	}

	if err := resumeCmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "  %s [context-save] Failed to start resume: %v\n", prefix, err)
		s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName,
			proc.sessionID, proc.agentID, "continue", "low_context", proc.modelID)
		proc.finalStatus = "CONTINUE"
		return
	}

	// Monitor resume output (just drain it)
	go func() {
		scanner := bufio.NewScanner(stdout)
		buf := make([]byte, 0, 1024*1024)
		scanner.Buffer(buf, 10*1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			s.trackRawOutput(proc, "[resume-save] "+line)
		}
	}()

	// 4. Wait for resume process to finish (with timeout)
	resumeDone := make(chan struct{})
	go func() {
		resumeCmd.Wait()
		close(resumeDone)
	}()

	select {
	case <-resumeDone:
		fmt.Printf("  %s [context-save] Resume save completed\n", prefix)
	case <-time.After(resumeSaveTimeout):
		fmt.Fprintf(os.Stderr, "  %s [context-save] Resume save timed out, killing\n", prefix)
		if resumeCmd.Process != nil {
			resumeCmd.Process.Kill()
		}
		<-resumeDone
	}

	// 5. Wait briefly for `agent continue` to propagate through socket
	time.Sleep(2 * time.Second)

	// 6. Register stop
	s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName,
		proc.sessionID, proc.agentID, "continue", "low_context", proc.modelID)

	proc.finalStatus = "CONTINUE"
	fmt.Printf("  %s [context-save] Save flow complete, will relaunch with fresh context\n", prefix)
}
