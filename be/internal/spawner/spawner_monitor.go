package spawner

import (
	"context"
	"syscall"
	"time"

	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/ws"
)

// monitorAll monitors all spawned processes until completion.
func (s *Spawner) monitorAll(ctx context.Context, processes []*processInfo, req SpawnRequest, phase string) error {
	const statusInterval = 30 * time.Second
	lastStatusTime := time.Time{}

	running := make([]*processInfo, len(processes))
	copy(running, processes)
	var completed []*processInfo

	// Per-monitorAll terminal-signal channel. Each session registered against
	// this channel routes its RequestTerminalSignal sends here, so concurrent
	// monitorAll goroutines cannot steal each other's signals.
	// Buffer large enough for all initial procs plus a margin for relaunches.
	ownTerminalCh := make(chan terminalSignal, len(processes)+4)
	registeredSessions := make(map[string]struct{}, len(processes))
	for _, proc := range processes {
		s.registerTerminalSignal(proc.sessionID, ownTerminalCh)
		registeredSessions[proc.sessionID] = struct{}{}
	}
	defer func() {
		for sid := range registeredSessions {
			s.unregisterTerminalSignal(sid)
		}
	}()
	// relaunchAndRegister wraps relaunchForContinuation to keep the
	// terminal-signal registry in sync across continuation relaunches:
	// drop the old session ID and bind the new one to ownTerminalCh.
	relaunchAndRegister := func(oldProc *processInfo) (*processInfo, error) {
		newProc, err := s.relaunchForContinuation(ctx, oldProc, req, phase)
		if err != nil {
			return nil, err
		}
		s.unregisterTerminalSignal(oldProc.sessionID)
		delete(registeredSessions, oldProc.sessionID)
		s.registerTerminalSignal(newProc.sessionID, ownTerminalCh)
		registeredSessions[newProc.sessionID] = struct{}{}
		globalLedgerStore.drop(oldProc.sessionID)
		DropProactiveRestartState(oldProc.sessionID)
		return newProc, nil
	}

	for len(running) > 0 {
		// Check for context cancellation or manual restart signal
		select {
		case <-ctx.Done():
			completed = append(completed, s.cancelRunningProcs(ctx, running, req)...)
			s.unregisterSessionProcs(completed)
			return ctx.Err()
		case restartSessionID := <-s.restartCh:
			// Manual restart requested — find matching proc and initiate context save
			for _, proc := range running {
				if proc.sessionID == restartSessionID && !proc.lowContextSaving {
					logger.Info(ctx, "manual restart requested", "session_id", restartSessionID)
					proc.lowContextSaving = true
					oldDoneCh := proc.doneCh
					newDoneCh := make(chan struct{})
					proc.doneCh = newDoneCh
					go s.initiateContextSave(ctx, proc, req, oldDoneCh, newDoneCh)
					break
				}
			}
		case takeControlSessionID := <-s.takeControlCh:
			// Take-control requested — find matching proc, validate, kill, and block
			tcMatched := false
			for i, proc := range running {
				if proc.sessionID != takeControlSessionID {
					continue
				}
				tcMatched = true
				// Validate backend supports take-control
				if proc.backend == nil || !proc.backend.SupportsTakeControl() {
					cliName, _ := parseModelID(proc.modelID)
					logger.Error(ctx, "take-control: backend does not support take-control", "cli", cliName, "session_id", takeControlSessionID)
					s.rejectTakeControl(req, proc, takeControlSessionID, "api_mode_unsupported")
					break
				}

				// Interactive backend: viewer-attach — broadcast but do NOT kill or block.
				// The agent keeps running; the viewer connects via /api/v1/pty/{session_id}.
				// No exit-interactive call is made on disconnect (completePtyInteractive is skipped).
				if proc.backend.Name() == "cli_interactive" {
					logger.Info(ctx, "take-control: viewer attach (interactive backend)", "session_id", takeControlSessionID)
					s.broadcast(ws.EventAgentViewerAttached, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
						"session_id": proc.sessionID,
						"agent_type": proc.agentType,
						"model_id":   proc.modelID,
					})
					s.signalTakeControlReady(takeControlSessionID)
					break
				}

				// Kill+resume needs adapter resume support — no hardcoded claude default left in the PTY manager.
				if !canResumeTakeControl(proc) {
					logger.Error(ctx, "take-control: resume unsupported for this session", "session_id", takeControlSessionID)
					s.rejectTakeControl(req, proc, takeControlSessionID, "resume_unsupported")
					break
				}

				logger.Info(ctx, "take-control: killing agent", "session_id", takeControlSessionID)

				// Kill process: SIGTERM → grace → SIGKILL
				proc.backend.Kill(ctx, proc, syscall.SIGTERM)
				gracePeriod := time.Duration(s.config.TimeoutGraceSec) * time.Second
				if gracePeriod == 0 {
					gracePeriod = 5 * time.Second
				}
				select {
				case <-proc.doneCh:
				case <-time.After(gracePeriod):
					proc.backend.Kill(ctx, proc, syscall.SIGKILL)
					<-proc.doneCh
				}

				// Flush messages, register the resume launch, and register stop.
				s.saveMessages(proc)
				s.registerTakeControlResumeLaunch(proc)
				s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName,
					proc.sessionID, proc.agentID, "user_interactive", "take_control", proc.modelID)

				// Broadcast take-control event
				s.broadcast(ws.EventAgentTakeControl, req.ProjectID, req.TicketID, req.WorkflowName, map[string]interface{}{
					"session_id": proc.sessionID,
					"agent_type": proc.agentType,
					"model_id":   proc.modelID,
				})

				// Status is now user_interactive and the agent is killed — unblock
				// any HTTP caller waiting in WaitForTakeControlReady before settling into the interactive wait.
				s.signalTakeControlReady(takeControlSessionID)

				// Remove from running
				running = append(running[:i], running[i+1:]...)

				// Create interactive wait channel and block until interactive session completes
				waitCh := make(chan struct{})
				s.mu.Lock()
				s.interactiveWaits[proc.sessionID] = waitCh
				s.mu.Unlock()

				logger.Info(ctx, "take-control: waiting for interactive session to complete", "session_id", takeControlSessionID)
				select {
				case <-waitCh:
					logger.Info(ctx, "take-control: interactive session completed", "session_id", takeControlSessionID)
				case <-ctx.Done():
					logger.Warn(ctx, "take-control: cancelled while waiting for interactive session", "session_id", takeControlSessionID)
				}

				s.mu.Lock()
				delete(s.interactiveWaits, proc.sessionID)
				_, wasKilled := s.killedInteractive[proc.sessionID]
				if wasKilled {
					delete(s.killedInteractive, proc.sessionID)
				}
				s.mu.Unlock()

				if wasKilled {
					proc.finalStatus = "FAIL"
				} else {
					proc.finalStatus = "PASS"
				}
				proc.elapsed = time.Since(proc.startTime)
				completed = append(completed, proc)
				break
			}
			if !tcMatched {
				// Session is not in our running list (already finished, or
				// owned by a different spawner). Unblock any caller waiting
				// on the readiness channel so it doesn't hang to its timeout.
				s.signalTakeControlReady(takeControlSessionID)
			}
		case sig := <-ownTerminalCh:
			// Terminal signal: DB result already written by socket handler.
			// Kill the matching agent so handleCompletion reads it on next iteration.
			// The registry ensures this signal is routed to *our* monitorAll only.
			for _, proc := range running {
				if proc.sessionID != sig.SessionID {
					continue
				}
				// Some backends write their final telemetry — token usage,
				// step-finish part — only AFTER the agent's last tool call
				// returns and the model emits its closing chunk. Give the
				// process a brief window to exit on its own before SIGTERM so
				// that telemetry lands on disk. If the process is already done
				// or exits early, the wait returns immediately.
				if grace := proc.backend.NaturalExitGrace(); grace > 0 {
					select {
					case <-proc.doneCh:
						logger.Info(ctx, "terminal signal: process exited naturally before kill",
							"session_id", sig.SessionID, "result", sig.Result)
					case <-time.After(grace):
						logger.Info(ctx, "terminal signal: natural-exit grace elapsed, sending SIGTERM",
							"session_id", sig.SessionID, "grace", grace)
					}
				}
				logger.Info(ctx, "terminal signal: killing agent", "session_id", sig.SessionID, "result", sig.Result)
				proc.backend.Kill(ctx, proc, syscall.SIGTERM)
				gracePeriod := time.Duration(s.config.TimeoutGraceSec) * time.Second
				if gracePeriod == 0 {
					gracePeriod = 5 * time.Second
				}
				select {
				case <-proc.doneCh:
				case <-time.After(gracePeriod):
					proc.backend.Kill(ctx, proc, syscall.SIGKILL)
					<-proc.doneCh
				}
				// doneCh closed; next loop iteration picks it up via handleCompletion
				break
			}
		case bumpSessionID := <-s.bumpMessageCh:
			// Hook event signal: update lastMessageTime so stall detection is not triggered.
			for _, proc := range running {
				if proc.sessionID == bumpSessionID {
					proc.messagesMutex.Lock()
					proc.lastMessageTime = s.config.Clock.Now()
					proc.hasReceivedMessage = true
					proc.messagesMutex.Unlock()
					break
				}
			}
		default:
		}

		now := time.Now()

		// Print status every interval
		if now.Sub(lastStatusTime) >= statusInterval {
			s.printStatus(running, completed, phase)
			lastStatusTime = now
		}

		// Read context_left from DB once per iteration
		readContextLeftFromDB(s.pool(), running)
		s.tailClaudeLedgers(running)

		// Check each process using doneCh (no double-wait bug)
		var stillRunning []*processInfo
		for _, proc := range running {
			elapsed := time.Since(proc.startTime)

			// Detect low context and initiate save (only for backends that track context)
			if !proc.lowContextSaving && proc.contextLeft > 0 && proc.contextLeft <= proc.restartThreshold &&
				(proc.backend == nil || proc.backend.TracksContext()) {
				proc.lowContextSaving = true
				// Replace doneCh — initiateContextSave will close the new one when the full flow completes
				oldDoneCh := proc.doneCh
				newDoneCh := make(chan struct{})
				proc.doneCh = newDoneCh
				go s.initiateContextSave(ctx, proc, req, oldDoneCh, newDoneCh)
			}

			select {
			case <-proc.doneCh:
				// Process exited
				proc.elapsed = elapsed
				proc.lowContextSaving = false

				// If context save already set finalStatus, skip handleCompletion
				if proc.finalStatus == "" {
					s.handleCompletion(ctx, proc, req)
				}

				// Auto-restart failed agent if configured
				if proc.finalStatus == "FAIL" && proc.maxFailRestarts > 0 && proc.failRestartCount < proc.maxFailRestarts {
					if s.waitBeforeRetry(ctx, proc) {
						logger.Info(ctx, "auto-restarting failed agent", "model", proc.modelID,
							"fail_restart_count", proc.failRestartCount+1, "max", proc.maxFailRestarts)
						// Override the already-registered failed session to continued/fail_restart
						if pool := s.pool(); pool != nil {
							sessionRepo := repo.NewAgentSessionRepo(pool, s.config.Clock)
							sessionRepo.UpdateResult(proc.sessionID, "continue", "fail_restart")
							sessionRepo.UpdateStatus(proc.sessionID, model.AgentSessionContinued)
						}
						proc.failRestartCount++
						proc.finalStatus = "CONTINUE"
					}
				}

				// Check for continuation
				if proc.finalStatus == "CONTINUE" {
					// A proactive rotation has already killed+saved the agent, so
					// the continuation cap must not convert it into a phase FAIL
					// (the rotation itself resets restartCount on relaunch). When
					// proactiveRotationPending is false this is byte-identical to
					// the emergency/manual continuation cap.
					if proc.proactiveRotationPending || proc.restartCount < defaultMaxContinuations {
						logger.Info(ctx, "continuation relaunching", "model", proc.modelID, "count", proc.restartCount+1, "max", defaultMaxContinuations)
						newProc, err := relaunchAndRegister(proc)
						if err != nil {
							logger.Error(ctx, "failed to relaunch", "model", proc.modelID, "err", err)
							completed = append(completed, proc)
						} else {
							stillRunning = append(stillRunning, newProc)
						}
					} else {
						logger.Error(ctx, "max continuations reached", "model", proc.modelID, "max", defaultMaxContinuations)
						proc.finalStatus = "FAIL"
						s.registerAgentStopWithReason(req.ProjectID, req.TicketID, req.WorkflowName,
							proc.sessionID, proc.agentID, "fail", "max_continuations", proc.modelID)
						completed = append(completed, proc)
					}
				} else {
					completed = append(completed, proc)
				}
			default:
				// Stall detection — check before timeout
				if s.checkStall(ctx, proc, req) {
					proc.elapsed = elapsed
					// checkStall already killed the process and set finalStatus=CONTINUE
					// Wait before relaunching
					if !s.waitBeforeStallRetry(ctx, proc, req) {
						completed = append(completed, proc)
						continue
					}
					if proc.restartCount < defaultMaxContinuations {
						newProc, err := relaunchAndRegister(proc)
						if err != nil {
							logger.Error(ctx, "failed to relaunch after stall", "model", proc.modelID, "err", err)
							completed = append(completed, proc)
						} else {
							stillRunning = append(stillRunning, newProc)
						}
					} else {
						logger.Error(ctx, "max continuations reached after stall", "model", proc.modelID)
						proc.finalStatus = "FAIL"
						completed = append(completed, proc)
					}
					continue
				}
				// Idle/nudge loop — send reminder or auto-fail unresponsive agent
				s.checkIdleNudge(ctx, proc, req)
				// Watcher-triggered proactive restart-with-digest — fires only
				// at a task boundary while idle; never mid-tool-chain.
				s.checkProactiveRestart(ctx, proc, req)
				// Still running - check timeout
				if elapsed > proc.timeout {
					s.handleGracefulTimeout(ctx, proc, req)
					// Auto-restart timed-out agent if configured
					if proc.maxFailRestarts > 0 && proc.failRestartCount < proc.maxFailRestarts {
						if !s.waitBeforeRetry(ctx, proc) {
							completed = append(completed, proc)
						} else {
							logger.Info(ctx, "auto-restarting timed-out agent", "model", proc.modelID,
								"fail_restart_count", proc.failRestartCount+1, "max", proc.maxFailRestarts)
							if pool := s.pool(); pool != nil {
								sessionRepo := repo.NewAgentSessionRepo(pool, s.config.Clock)
								sessionRepo.UpdateResult(proc.sessionID, "continue", "timeout_restart")
								sessionRepo.UpdateStatus(proc.sessionID, model.AgentSessionContinued)
							}
							proc.failRestartCount++
							proc.finalStatus = "CONTINUE"
							newProc, err := relaunchAndRegister(proc)
							if err != nil {
								logger.Error(ctx, "failed to relaunch after timeout", "model", proc.modelID, "err", err)
								completed = append(completed, proc)
							} else {
								stillRunning = append(stillRunning, newProc)
							}
						}
					} else {
						completed = append(completed, proc)
					}
				} else {
					stillRunning = append(stillRunning, proc)
					s.maybeFlushMessages(proc)
				}
			}
		}

		running = stillRunning
		if len(running) > 0 {
			time.Sleep(1 * time.Second)
		}
	}

	// Finalize phase
	return s.finalizePhase(ctx, completed, req, phase)
}
