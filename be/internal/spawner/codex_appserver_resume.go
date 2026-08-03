package spawner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"be/internal/logger"
)

// codexThreadHandoff carries a live codex app-server thread across a
// fail-restart relaunch: the per-session CODEX_HOME profile dir (holding the
// thread's rollout JSONL under sessions/) and the thread id to resume, plus
// the dying session's raw reported cumulative usage so the resumed session's
// reported high water can be seeded (codex reports thread-cumulative totals,
// so without seeding, the resumed thread's first tokenUsage event would
// re-bill the dead session's tokens onto the new agent_sessions row).
type codexThreadHandoff struct {
	threadID   string
	profileDir string

	baseline CostSnapshot // dying session's raw reported cumulative at hand-off time
}

// discard implements resumeHandoff — removes the temp profile dir. Called
// only when the handoff was not transferred onward to a successor proc.
func (h *codexThreadHandoff) discard() {
	if h == nil {
		return
	}
	_ = os.RemoveAll(h.profileDir)
}

// captureCostBaseline implements costBaselineCapture — called by
// transferResume with the dying session's final cost snapshot, right before
// the handoff moves to the successor proc.
func (h *codexThreadHandoff) captureCostBaseline(snap CostSnapshot) {
	h.baseline = snap
}

// threadResumeParams builds a thread/resume params object. Validated against
// the installed codex-cli 0.145.0 generate-json-schema output
// (v2/ThreadResumeParams.json): required threadId; thread/resume takes no
// effort param, unlike threadStartParams — effort keeps riding turn/start.
func threadResumeParams(threadID, model, cwd, sandbox, approvalPolicy string) map[string]any {
	return map[string]any{
		"threadId":       threadID,
		"model":          model,
		"cwd":            cwd,
		"sandbox":        sandbox,
		"approvalPolicy": approvalPolicy,
	}
}

// resolveCodexProfileDir resolves the per-session CODEX_HOME profile dir for
// Start. When proc carries an armed *codexThreadHandoff (fail-restart
// relaunch), the handoff's dir is reused — writeCodexSessionProfile is
// re-run on it so config.toml's [mcp_servers.nrflo] env carries the NEW
// session id (the sessions/ rollout tree itself is untouched). Otherwise a
// fresh MkdirTemp profile is minted, exactly as before this feature.
func resolveCodexProfileDir(proc *processInfo) (dir string, resumed bool, err error) {
	if h, ok := proc.resumeHandoff.(*codexThreadHandoff); ok && h != nil {
		if err := writeCodexSessionProfile(h.profileDir, proc); err != nil {
			return "", false, fmt.Errorf("codex app-server: rewrite resumed profile: %w", err)
		}
		return h.profileDir, true, nil
	}

	dir, err = os.MkdirTemp("", "nrflo-codex-as-"+proc.sessionID+"-*")
	if err != nil {
		return "", false, fmt.Errorf("codex app-server: mkdir profile: %w", err)
	}
	if err := writeCodexSessionProfile(dir, proc); err != nil {
		_ = os.RemoveAll(dir)
		return "", false, fmt.Errorf("codex app-server: write profile: %w", err)
	}
	return dir, false, nil
}

// startOrResumeThread starts a fresh codex thread, or resumes the one handed
// off by a crashed predecessor. On resume, firstTurnText is the rendered
// crash-resume injectable rather than prep.prompt — the full prompt already
// lives inside the resumed thread's history. A thread/resume rpc error (e.g.
// -32600 "no rollout found for thread id ...", which fires when the prior
// thread never completed a turn) is logged and falls through to thread/start
// + the full prompt: a resume failure must never become a hard spawn
// failure.
func (b *codexAppServerBackend) startOrResumeThread(runCtx context.Context, proc *processInfo, prep *prepResult, client *appServerClient, cliModel string) (threadID, firstTurnText string, err error) {
	sandbox := effectiveSpawnSandbox(prep.opts.Sandbox)
	logCtx := logger.WithTrx(context.Background(), proc.trx)

	if h, ok := proc.resumeHandoff.(*codexThreadHandoff); ok && h != nil {
		resp, rerr := client.call(runCtx, "thread/resume", threadResumeParams(h.threadID, cliModel, proc.workDir, sandbox, "never"))
		if rerr == nil {
			if id := unmarshalThreadID(resp); id != "" {
				SeedSessionCostReported(proc.sessionID, h.baseline.InputTokens, h.baseline.OutputTokens, h.baseline.CacheReadTokens, h.baseline.CacheWriteTokens)
				firstTurn := b.s.expandInjectable("crash-resume", map[string]string{"RESTART_REASON": reasonFailRestart})
				if fb := b.s.restartFeedbackForProc(proc); fb != "" {
					firstTurn += "\n\n" + fb
				}
				return id, firstTurn, nil
			}
			rerr = fmt.Errorf("thread/resume: empty thread id")
		}
		logger.Warn(logCtx, "codex app-server: thread/resume failed, falling back to thread/start",
			"session_id", proc.sessionID, "thread_id", h.threadID, "err", rerr)
	}

	resp, serr := client.call(runCtx, "thread/start", threadStartParams(cliModel, proc.workDir, sandbox, "never"))
	if serr != nil {
		return "", "", fmt.Errorf("thread/start: %w", serr)
	}
	id := unmarshalThreadID(resp)
	if id == "" {
		return "", "", fmt.Errorf("thread/start: empty thread id")
	}
	return id, prep.prompt, nil
}

// unmarshalThreadID extracts thread.id from a thread/start or thread/resume
// response; returns "" on any decode failure or missing field.
func unmarshalThreadID(resp json.RawMessage) string {
	var threadResp struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	_ = json.Unmarshal(resp, &threadResp)
	return threadResp.Thread.ID
}

// TakeControlExtras implements interactiveHandoff: hands the live per-session
// CODEX_HOME dir to a take-control PTY resume launch, so `codex resume
// <thread_id>` finds the thread's rollout instead of pointing at the viewer's
// real ~/.codex.
func (b *codexAppServerBackend) TakeControlExtras() (InteractiveExtras, bool) {
	if b.profileDir == "" {
		return InteractiveExtras{}, false
	}
	return InteractiveExtras{CodexHome: b.profileDir}, true
}
