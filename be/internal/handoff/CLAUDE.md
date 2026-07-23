# handoff Package

Composes the code-owned, model-free "Verified State" channel and combines it with a caller-supplied narrative and a verbatim message tail into one three-section handoff document.

## Invariant

`Compose(ctx, pool, clk, sessionID, narrative) string` (`handoff.go`) runs at READ time, not fold time — nothing it produces is stored, so the document is always fresh with no second digest row to keep in sync. It renders up to three sections, each capped independently and then the whole document capped to `maxHandoffBytes` (12288): `## Verified State` (`verified.go`, authoritative, no model — Task/Plan/Outcome/Files touched/Unverified references/Commands run/Test results blocks, each omitted when empty), `## Narrative Summary` (the caller's narrative, wrapped verbatim, explicitly non-authoritative for identifiers), and `## Recent Uncompressed Context` (`tail.go`, newest transcript rows joined via `foldfmt.JoinTail`). Compose returns `""` only when every section would be empty, so callers keep their pre-existing fallback (e.g. the raw narrative).

**Never synthesize.** Every identifier under Verified State comes from a DB row or an `os.Stat`/`repo.TicketRepo.Get` hit against the run's working tree — never a fuzzy match, never a "closest" file, never the first of several candidates. `resolveContext` (`context.go`) resolves `repoRoot` as `workflow_instances.worktree_path` else `projects.root_path` (mirrors `orchestrator/consult.go:64-70`); `extractFrom` (`extract.go`) pulls path/ticket/command/test-result candidates from `agent_messages` (payload.input first, `[Tool] detail` content second, conservative regexes over free text last), copying every match verbatim; `resolvePaths`/`resolveTickets` (`resolve.go`) then either canonicalize (a bare basename ONLY on a unique repo match) or leave the candidate in "Unverified references" — 0 or >1 basename matches always stay unverified, verbatim as extracted. `selectPlanFindings` (`findings_select.go`) reads `_`-suffixed plan finding keys + `workflow_final_result` via `repo.FindingRepo.ListByWorkflowInstance`, sorted by `(agent_type, key)` so two Compose calls over the same DB state are byte-identical; plan-finding paths are NOT run through the resolver (e.g. `files_to_create` legitimately does not exist yet).

**Best-effort, never propagate.** Every DB/FS read failure is logged via `logger.Warn`/`logger.Error` and degrades that block to empty; `Compose` never returns an error and never blocks a caller.

`chainSessionIDs` (`context.go`) walks `ancestor_session_id` backwards, capped at `maxChainSessions=3`, so a kill->relaunch chain's message-derived refs survive the handoff; `GetBySessionTail` (`be/internal/repo/agent_message_handoff.go`) is the newest-N read each chain session contributes to the shared `maxScanMessages=1200` scan budget.

## Consumers

Three call sites compose instead of passing raw narrative text: the `${previous_data}` relaunch injection (`spawner/template_findings_prev.go`), `GET /api/v1/sessions/{id}/handoff-digest` (`api/handlers_handoff_digest.go`), and the `agent.handoff_digest` WS broadcast (`refinery/session_sidecar.go`). All three fall back to the raw narrative/digest content when `Compose` returns `""`.

## Import Hygiene

`handoff` imports `db`, `repo`, `model`, `clock`, `logger`, `foldfmt` only — no `service`, no `spawner`, no `refinery`, no `ws`. This keeps `spawner -> handoff`, `api -> handoff`, and `refinery -> handoff` all acyclic.
