---
name: analyze-sessions
description: >-
  Audit the last 20 finished workflows across ALL projects for engine issues:
  dump instances, session anomalies (restarts, nudges, stall/stop-blocks,
  rate-limits, low-context), error messages, final-result findings and be.log
  errors via gather.sh, classify each signal against the signature map, verify
  root causes in the codebase, then fix confirmed engine bugs (small fixes
  directly + /finalize; systemic ones as tickets). Use when the user asks to
  check agent logs/sessions, "are the agents healthy", post-epic retrospectives,
  or suspects spawner/orchestrator misbehavior. Optional argument: N (instance
  count, default 20).
---

# /analyze-sessions

Evidence first, classification second, code fixes last — and only for issues
that are actually the engine's fault.

## Step 1 — Gather evidence

```bash
bash .claude/skills/analyze-sessions/gather.sh [N] > /tmp/nrflo-analyze-sessions.txt
```

Read the whole report. It is read-only against `$NRFLO_HOME/nrflo.data`
(default `~/.nrflo`) and covers, for the last N (default 20) instances with
status `completed|failed|project_completed` across all projects:

1. the instance list, 2. an anomaly aggregate, 3. anomalous sessions,
4. error/validation agent messages, 5. `workflow_final_result` +
`review_summary` findings, 6. be.log ERROR/WARN lines in the window.

Never write to the DB. Any scratch files go to `/tmp`, never the repo.

## Step 2 — Classify every signal

Walk section [3] row by row and section [5]/[6] for narrative signals. Assign
each to one class using this signature map; a signal may implicate more than
one.

| Signal | Likely class | Where to verify |
|---|---|---|
| `result_reason=fail_restart` clustered on ONE phase across tickets | agent-definition prompt / gate flakiness, or restart policy | the agent def (service seeds / project defs), spawner restart cap |
| `stall_restart_start_stall`, `nudge_count>0` | stall detection tuned too tight vs provider latency | spawner stall timeouts ([spawner/CLAUDE.md](../../../be/internal/spawner/CLAUDE.md)) |
| `stop_block_count>0` | agent ended turn without lifecycle tool; Stop-hook loop | `be/internal/socket/handler_stop.go` + the agent's prompt template |
| `rate_limit_retry_count>0`, `last_retry_class` set | provider throttling; check backoff behaved | `be/internal/spawner/inband_rate_limit.go`, rate-limit columns |
| `ctx` low, `result=continue` frequent | low-context relaunch churn → prompt bloat or threshold | spawner low-context relaunch, prompt sizes |
| `status=failed` + `result_reason=cancelled` | human cancellation — benign | skip (note it, don't fix) |
| category `error`/`validation` messages | tool dispatch / socket / schema failures | `be/internal/socket/`, `be/internal/spawner/apirun/tools_builtin/` |
| be.log ERROR/WARN | api / ws / db / spawner runtime errors | the logging call site |
| final-result notes: missing binaries, `npm ci`, stale caches, worktree gaps | environment/bootstrap friction | `Makefile`, worktree setup |
| final-result notes: flaky/parallel-load test failures | test isolation (Rule 4) | the named test package |
| instance `status=failed` with passing reruns | orchestrator retry/fan-in policy | [orchestrator/CLAUDE.md](../../../be/internal/orchestrator/CLAUDE.md) |

Three buckets come out of this:

- **Engine bug** — nrflo code misbehaved (fix here).
- **Environment/bootstrap friction** — repeatable tax on agents (fix here:
  Makefile/scripts/docs).
- **Agent/model/project behavior** — model quirks, project config, human
  cancels. Report only; do NOT "fix" these with engine code, and never edit
  another project's code from this repo.

## Step 3 — Verify before fixing (NEVER GUESS)

For each suspected engine bug, confirm the root cause in code before touching
anything: read the implicated file(s), and when the failure is reproducible,
write the failing test first. Use the codebase-exploration skill for anything
broader than one known file. Cross-check whether the issue already has a
ticket or was fixed after the sessions ran (`git log` since the instance's
`updated_at`).

## Step 4 — Fix

- **Contained fix** (one subsystem, clear test): implement it now, then run
  `/finalize` to sync docs, run the gates, and commit.
- **Systemic / multi-ticket work**: create ticket(s) in the `nrworkflow`
  project (epic + children if phased) instead of a drive-by patch; put the
  evidence (ticket IDs, session rows, log lines) in the description.
- **Unclear ownership or product decision**: surface it in the report with a
  recommendation; do not act unilaterally.

## Step 5 — Report

End with a compact summary, most severe first:

- issues found: class, evidence (project/ticket/phase + counts), verdict
  (engine bug / friction / behavior — with root cause file:line when verified)
- fixes applied (commit SHAs) and tickets created
- signals inspected and dismissed as benign (one line each)
- overall health verdict for the window

## Hard rules

- The DB is read-only for this skill — no UPDATE/DELETE/INSERT, ever.
- No analysis `.md` files in the repo; `/tmp` only (global rule).
- Don't reclassify model/agent behavior as an engine bug to justify a code
  change — the fix must match the verified root cause.
- Fixes go through the normal gates (`/finalize`); never commit red.
