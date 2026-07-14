package api

import (
	"fmt"
	"strings"

	"be/internal/model"
	ptyPkg "be/internal/pty"
	"be/internal/repo"
	"be/internal/spawner"
)

// startResumeSession registers the CLI resume launch with the PTY manager and
// flips the session to user_interactive. Registration must come first: the PTY
// attach the UI opens next has no default command to fall back on, so a launch
// registered late (or not at all) fails the attach with "no PTY launch
// registered" after the session is already marked interactive.
func (s *Server) startResumeSession(asRepo *repo.AgentSessionRepo, session *model.AgentSession) error {
	if err := s.registerResumeLaunch(session); err != nil {
		return fmt.Errorf("failed to prepare resume: %w", err)
	}
	return asRepo.UpdateStatus(session.ID, model.AgentSessionUserInteractive)
}

// registerResumeLaunch builds the CLI resume command for a finished session via
// the spawner adapter registry and registers it with the PTY manager, so the PTY
// attach that follows (GET /api/v1/pty/{session_id}) has a launch to start.
//
// The PTY manager has no built-in default command: a session with no registered
// launch fails Create() honestly rather than silently exec'ing claude. Working
// directory and env are left unset here so the PTY handler's own values (project
// root + agent envelope) apply.
func (s *Server) registerResumeLaunch(session *model.AgentSession) error {
	if s.ptyManager == nil {
		return fmt.Errorf("pty manager not available")
	}

	cliName, rawModel := splitModelID(session.ModelID.String)
	adapter, err := spawner.GetCLIAdapter(cliName)
	if err != nil {
		return err
	}
	if !adapter.SupportsResume() {
		return fmt.Errorf("session CLI %q does not support resume", cliName)
	}

	cmd := adapter.BuildInteractiveCommand(spawner.InteractiveSpawnOptions{
		SessionID:       session.ID,
		Model:           adapter.MapModel(rawModel),
		ResumeSessionID: session.ID,
	})
	s.ptyManager.RegisterLaunch(session.ID, ptyPkg.Launch{
		Command: cmd.Path,
		Args:    cmd.Args[1:],
	})
	return nil
}

// splitModelID splits a stored model_id ("claude:sonnet") into CLI name and
// bare model. A value with no ":" is treated as CLI-only.
func splitModelID(modelID string) (cli, bareModel string) {
	if idx := strings.Index(modelID, ":"); idx >= 0 {
		return modelID[:idx], modelID[idx+1:]
	}
	return modelID, ""
}
