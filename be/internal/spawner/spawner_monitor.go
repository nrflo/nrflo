package spawner

import (
	"context"
	"syscall"
	"time"

	"be/internal/logger"
	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
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
	// relaunchWithBookkeeping runs spawnFn (either relaunchForContinuation or
	// relaunchForFallback) and keeps the terminal-signal registry, context
	// ledger, refinery sidecar, and session cost bookkeeping in sync across
	// the swap — shared by every relaunch path so fallback relaunches are not
	// a second, drifting copy of this plumbing.
	relaunchWithBookkeeping := func(oldProc *processInfo, spawnFn func() (*processInfo, error)) (*processInfo, error) {
		newProc, err := spawnFn()
		if err != nil {
			return nil, err
		}
		// Release the old handoff when it was not transferred onward (e.g. a
		// fallback relaunch, which never opts into resumeOnRelaunch) so its
		// temp CODEX_HOME dir doesn't leak; nil-safe/no-op when already moved.
		oldProc.discardResume()
		s.unregisterTerminalSignal(oldProc.sessionID)
		delete(registeredSessions, oldProc.sessionID)
		s.registerTerminalSignal(newProc.sessionID, ownTerminalCh)
		registeredSessions[newProc.sessionID] = struct{}{}
		globalLedgerStore.drop(oldProc.sessionID)
		// Final-fold the old session before its cost entry is finalized so
		// the fold's usage still attributes to a live running-cost row; the
		// new session's StartSession fires automatically inside startBackend
		// on relaunch, same slot (workflow_instance_id, node_id).
		if s.config.RefinerySidecar != nil {
			s.config.RefinerySidecar.StopSession(oldProc.sessionID)
		}
		FinalizeSessionCost(oldProc.sessionID)
		FinalizeSessionTiming(oldProc.sessionID)
		DropProactiveRestartState(oldProc.sessionID)
		return newProc, nil
	}
	relaunchAndRegister := func(oldProc *processInfo) (*processInfo, error) {
		return relaunchWithBookkeeping(oldProc, func() (*processInfo, error) {
			return s.relaunchForContinuation(ctx, oldProc, req, phase)
		})
	}
	relaunchFallbackAndRegister := func(oldProc *processInfo, entry service.AgentChainEntry, nextPos int) (*processInfo, error) {
		return relaunchWithBookkeeping(oldProc, func() (*processInfo, error) {
			return s.relaunchForFallback(ctx, oldProc, req, phase, entry, nextPos)
		})
	}

	for len(running) > 0 {
		// Check for context cancellation or manual restart signal
		select {
		case <-ctx.Done():
			completed = append(completed, s.cancelRunningProcs(ctx, running, req)...)
			s.unregisterSessionProcs(completed)
			return ctx.Err()
		case restartSessionID := <-s.restartCh:
			s.dispatchManualRestart(ctx, running, req, restartSessionID)
		case takeControlSessionID := <-s.takeControlCh:
			// Take-control requested — find matching proc, validate, kill, and
			// block (see spawner_monitor_takecontrol_case.go).
			var tcCompleted *processInfo
			running, tcCompleted = s.handleTakeControlRequest(ctx, running, req, takeControlSessionID)
			if tcCompleted != nil {
				completed = append(completed, tcCompleted)
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
		case bumpSessionID := <-s.bumpMessageCh: // hook event signal, see dispatchBumpMessage
			s.dispatchBumpMessage(running, bumpSessionID)
		case nr := <-s.nudgeRequestCh: // in-band Notification-hook signal, see idle_nudge.go
			s.dispatchNudgeRequest(ctx, running, req, nr)
		case rotateSessionID := <-s.stepRotateCh: // complete_step OutcomeRotate signal, see step_rotation.go
			s.dispatchStepRotation(ctx, running, req, rotateSessionID)
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

				// Tier fallback: a HARD provider failure (never rate-limit)
				// advances to the next chain entry before the ordinary
				// same-model maxFailRestarts retry gets a chance; chain
				// exhaustion leaves FAIL terminal. Guarded on hardProviderFail
				// so an ordinary (non-provider) fail never takes this path.
				if proc.finalStatus == "FAIL" && proc.hardProviderFail {
					if newProc, advanced := s.tryChainFallback(ctx, proc, req, relaunchFallbackAndRegister); advanced {
						if newProc != nil {
							stillRunning = append(stillRunning, newProc)
						} else {
							completed = append(completed, proc)
						}
						continue
					}
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
						proc.resumeOnRelaunch = true
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
