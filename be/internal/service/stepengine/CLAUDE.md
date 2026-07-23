# stepengine Package

Server-owned stepwise step engine for `prompt_mode='stepwise'` agent definitions: cursor snapshot, per-step evidence validation, transactional advance, and the rotate decision.

## Invariants

- **Server-owned cursor.** `agent_step_cursors`, keyed by `(workflow_instance_id, node_id)`, survives session swaps — a kill→relaunch reuses the same cursor rather than restarting the step sequence.
- **Agent never self-advances.** `Advance` is the only mutation path; it is CAS-guarded on `(revision, current_index)` (`repo.AgentStepCursorRepo.Advance`) for exactly-once semantics, with idempotent same-revision replay (a retried `(step_id, revision)` after a successful advance returns the original outcome again, `Replayed=true`, without re-mutating the row).
- **Instructions are immutable.** `Snapshot` copies `agent_definitions.steps` into `agent_step_cursors.steps_snapshot` once; `Advance`/`ValidateEvidence` always read the snapshot, never the live definition — a mid-run edit to the definition's steps has no effect on an in-flight cursor.
- **Rejection is for missing keys / invalid schema only.** An unresolved or ambiguous path-bearing finding value (`handoff.ResolvePathCandidates`) is a non-fatal `Flags` entry, never a rejection — a bare basename is the only candidate shape that can land ambiguous; absolute/slash-containing candidates are a direct `Stat` so they're only ever resolved-or-unresolved. This mirrors why `handoff.selectPlanFindings` never resolves `files_to_create` paths: they legitimately don't exist yet.
- **Import hygiene.** Depends only on `db`/`repo`/`model`/`clock`/`logger`/`handoff` — never `service` or `spawner` — so both `tools_builtin` (service-layer) and the spawner wiring can import this package without a cycle.

- **complete_step owns Advance.** `be/internal/spawner/apirun/tools_builtin/complete_step.go` is the only caller of `Advance`; the rejection counter (`agent_step_cursors.rejections`, `RecordRejection`/`RejectionCount`) is durable cursor state keyed by `step_id`, with the cap enforced in the builtin against `service.StepRejectionCap` — `Rejection.CountsTowardEvidenceCap()` decides which reasons count. `Outcome.Flags` carries non-fatal path notices through to the agent on `OutcomeNext`/`OutcomeDone`.

## Entry Points

`New(pool, clk, checks CheckRunner) *Engine` — `checks` is the injectable command runner (`RunChecks`, same return shape as `spawner.runValidationCommands`); nil skips checks. `Engine.Snapshot`, `Engine.State`, `Engine.ValidateEvidence`, `Engine.Advance`, `Engine.CompletedEvidence` (structured per-completed-step evidence — snapshot-declared keys/values/resolved-paths, no prompt prose, consumed by the spawner's stepwise resume body), and the pure `ShouldRotate` are the package's exported surface — see `stepengine.go`/`snapshot.go`/`evidence.go`/`advance.go`/`rotate.go`/`evidence_digest.go`.
