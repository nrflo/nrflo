package spawner

import (
	ptyPkg "be/internal/pty"
	"be/internal/ws"
)

// rejectTakeControl broadcasts EventAgentTakeControlRejected with reason and
// unblocks any HTTP caller waiting in WaitForTakeControlReady.
func (s *Spawner) rejectTakeControl(req SpawnRequest, proc *processInfo, sessionID, reason string) {
	s.broadcast(ws.EventAgentTakeControlRejected, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
		"session_id": proc.sessionID,
		"agent_type": proc.agentType,
		"model_id":   proc.modelID,
		"reason":     reason,
	})
	s.signalTakeControlReady(sessionID)
}

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

// canResumeTakeControl reports whether proc's backend/adapter can be resumed
// after a take-control kill. False means the PTY manager would have nothing
// to launch when the viewer connects — Create() errors honestly instead of
// falling back to a hardcoded claude default.
func canResumeTakeControl(proc *processInfo) bool {
	if proc.backend == nil || !proc.backend.SupportsResume() || proc.adapter == nil {
		return false
	}
	return sessionIDForResume(proc.adapter, proc) != ""
}

// registerTakeControlResumeLaunch builds the adapter-owned resume command for
// proc (post-kill) and registers it with the PTY manager, so a take-control
// viewer connecting after the kill has a launch to attach to.
func (s *Spawner) registerTakeControlResumeLaunch(proc *processInfo) {
	if s.config.PTYManager == nil {
		return
	}
	_, rawModel := parseModelID(proc.modelID)
	var mappedModel, reasoningEffort string
	if cfg, ok := s.config.ModelConfigs[rawModel]; ok {
		mappedModel = cfg.CLIModel
		reasoningEffort = cfg.DefaultEffort
	}
	if mappedModel == "" {
		mappedModel = proc.adapter.MapModel(rawModel)
	}
	resumeCmd := proc.adapter.BuildInteractiveCommand(InteractiveSpawnOptions{
		SessionID:       proc.sessionID,
		Model:           mappedModel,
		ReasoningEffort: reasoningEffort,
		WorkDir:         proc.workDir,
		ResumeSessionID: sessionIDForResume(proc.adapter, proc),
	})
	s.config.PTYManager.RegisterLaunch(proc.sessionID, ptyPkg.Launch{
		Command: resumeCmd.Path,
		Args:    resumeCmd.Args[1:],
		Env:     resumeCmd.Env,
		Dir:     resumeCmd.Dir,
	})
}
