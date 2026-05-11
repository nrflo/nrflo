# nrflo backlog

Candidate features. Each entry is a self-contained brief: motivation, design, surface area, and open questions. Not approved, not scheduled — review and triage.

---

## 1. Declarative validation commands per layer / agent

### Motivation
Today, "did this agent succeed?" is decided inside the agent prompt: the agent runs `make test`, decides if it passed, and reports pass/fail via `agent finished` / `agent fail`. This couples gate logic to agent reasoning. Two consequences:

- The same check (e.g., `make test`) gets re-described in every agent prompt that wants it.
- The orchestrator has no first-class view of "what shell commands constitute pass for this step" — it can only read the agent's verdict.

Better: each agent/layer declares a list of shell commands, and the orchestrator runs them after the implementing agent finishes. Exit-0 across the board = pass; any non-zero feeds the failing command's output back as retry context.

The wins:

- The gate is **declarative**, inspectable in the workflow definition, and reusable across agents.
- Agents stop having to lie/be-honest about test results — the orchestrator runs them.
- Findings have a **canonical source** (stdout/stderr of the failing command) rather than an agent's summary.

### Design

Add an optional `validation_commands` field on either:
- **per-agent-definition** (preferred — same agent in different workflows can have different gates), or
- **per-layer-policy** (`workflow_layer_policies` row gets a `validation_commands` column).

Schema sketch (per agent):
```sql
ALTER TABLE agent_definitions
  ADD COLUMN validation_commands TEXT;  -- JSON array of strings
```

Each command is a single shell string executed in the workflow's working directory (ticket-scoped: worktree; project-scoped: project root) with the same env that the agent saw (`ProjectEnv` + nrflo-controlled vars).

Execution order, after the agent reports `finished`:
1. Spawner / orchestrator runs each command sequentially, capturing combined stdout+stderr.
2. First non-zero exit → write `validation_failure` finding with `{command, exit_code, output_tail}` and transition the session to `failed` (counts against agent retry budget).
3. All zero → session stays `completed`.

If `validation_commands` is empty/null, behavior is unchanged (agent verdict is final).

### API & UI surface
- Workflow editor: per-agent textarea for newline-separated commands.
- API: extend `POST /api/v1/agent-definitions` and the update endpoint.
- Logs: surface the validation run output as part of the agent session log (new section, not interleaved with agent transcript).

### Open questions
- Should validation run on `cli_interactive` sessions? Probably yes, but only after the user signals "I'm done" (exits interactive).
- Should validation run on `api` and `script` modes? Yes — uniform across `effective_mode`.
- Per-command timeout? Default 5 min, override via `validation_command_timeout_sec` column.
- Should validation failure be retryable distinct from agent failure (different counters)? Likely yes — a flaky test should not consume the agent's restart budget the same way an actual failure does. Punt on this until we see real data.

---

## 2. Rate-limit pattern detection + auto-wait

### Motivation
Today, when a CLI agent hits a provider rate limit (Anthropic 429, OpenAI quota, Codex limit), nrflo treats the resulting non-zero exit as an agent failure. The session counts against the restart cap; if limits persist, the workflow eventually fails with no useful signal.

Better: separate **transient rate limits** from **real failures** via pattern matching on output. On a limit match, pause for a configurable backoff and retry the **same** session **without** consuming the retry budget.

Long-running workflows that span an hourly quota currently burn through `restart cap` for nothing — this fixes that.

### Design

Add a small, per-CLI-adapter classifier:

```go
// In CLIAdapter interface
type RetryClass int
const (
    RetryClassNone     RetryClass = iota
    RetryClassRateLimit
    RetryClassError
)

ClassifyExit(recentText, stderrTail string, exitCode int) RetryClass
```

Each adapter (`cli_adapter_claude.go`, `cli_adapter_codex.go`, `cli_adapter_opencode.go`, `apirun`'s in-process equivalent) owns its own pattern list. Anthropic API errors in api-mode get classified at the SDK error level (typed errors), not via stdout regex.

**Pattern scan must be windowed, not full-output.** Keep a ring buffer of the last ~10 text blocks per session and only match against the joined recent window. Matching against full output trips false positives when the agent earlier echoed text like "rate limit" while quoting code or docs. Maintain `recentBlocks` per session, scan only the joined tail.

**Two pattern lists, with priority.** Split into `*_limit_patterns` (wait+retry) and `*_error_patterns` (graceful exit). Same string can appear in both: e.g. `"You've hit your org's monthly usage limit"` lives in both lists, and the limit check runs **first**, so when `rate_limit_enabled=true` it wins; when disabled the string still trips graceful exit instead of generic agent failure. This single mechanism handles both modes cleanly.

**Matching is case-insensitive substring.** Not regex initially — vendors reword these messages frequently and regex tempts over-clever patterns that miss the next variant. Substring is more forgiving and cheaper to debug.

When the spawner sees `RetryClassRateLimit`:
1. Persist the limit hit on the `agent_sessions` row (new column `last_retry_class TEXT` or use an enum).
2. Sleep for a configurable backoff (start 60s, exponential up to `rate_limit_max_wait`, default 1h). Sleep is via `clock.Clock` so tests can fast-forward.
3. Re-spawn the **same** session with the same prompt and finding context. **Do not** decrement the restart counter.
4. Emit a WS event `agent.rate_limited` with `{session_id, wait_seconds, matched_pattern}` so the UI can show "waiting for quota… 47m" instead of "failed."

If `rate_limit_max_wait` is exceeded across consecutive limit hits → fall through to normal failure path (so a permanently-broken account doesn't hang forever).

### Configuration

Per-project settings (new `project_settings` row or `project_env_vars`-adjacent):
- `rate_limit_max_wait_sec` (default 3600)
- `rate_limit_initial_backoff_sec` (default 60)
- `rate_limit_enabled` (default true)

**Pattern lists are user-overridable.** Adapter defaults shipped in code, but exposed in project settings as comma-separated overrides:
- `claude_limit_patterns`, `claude_error_patterns`
- `codex_limit_patterns`, `codex_error_patterns`
- `opencode_limit_patterns`, `opencode_error_patterns`

Rationale: these strings are user-facing English produced by third-party CLIs. When Anthropic/OpenAI rewords (e.g., `"You've hit your limit"` → `"Usage limit reached"`), users shouldn't need to wait for an nrflo release to keep working — they patch the pattern themselves. We ship reasonable defaults; they own the override.

Seed defaults (validated against current CLI output as of 2026-05):
- claude limit: `You've hit your limit`, `Your usage allocation has been disabled by your admin`, `You've hit your org's monthly usage limit`
- claude error: same three, plus `API Error:`, `cannot be launched inside another Claude Code session`, `Not logged in`
- codex limit: `Rate limit exceeded`, `rate limit reached`, `429 Too Many Requests`, `quota exceeded`, `insufficient_quota`, `You've hit your usage limit`

### UI surface
- Live session view shows "Rate-limited, retrying in X" badge.
- `agent_session_logs` includes rate-limit events as a distinct row type so they're filterable.
- Live agent sessions endpoint already surfaces status — extend with `rate_limit_until_ts`.

### Open questions
- Should rate-limit waits respect `endless_loop` cancellation and graceful shutdown? Yes — `shutdownCleanup` must wake sleeping waiters.
- Does the wait survive a server restart? Initially no (sleep is in-memory). If we need it durable, persist `rate_limit_until_ts` on the session and resume on startup. Defer.
- Does the budget for `rate_limit_max_wait` reset on success? Yes — track consecutive limit hits only.
- Ring-buffer size: start at 10 text blocks. For `script` and `cli_interactive` modes, "block" is less well-defined (no JSON event stream). Likely: last 10 stdout lines + last 10 stderr lines, concatenated. Validate during impl.
- `api`-mode (apirun) classification: skip pattern matching entirely — Anthropic SDK surfaces `RateLimitError` / `OverloadedError` as typed errors. Map those directly to `RetryClassRateLimit` without going near string scans. Patterns only exist for the CLI-shell-out modes.

---

## 3. Post-success "finalize" step that is allowed to fail

### Motivation
Today, once a workflow's last layer completes, the orchestrator either:
- closes the ticket (if `close_ticket_on_complete=true`),
- spawns `NextWorkflowOnSuccess` (if set and `workflow_final_result` non-empty), and
- dispatches notifications.

There is no place to run "do this cleanup/push/deploy/rebase work, but if it fails don't fail the workflow."

Concrete use cases:
- Push the branch + open a draft PR (currently has to be inside the implementor agent, which makes that agent's prompt do double duty).
- Run a deploy script.
- Post a custom Slack message that needs the workflow's findings (the existing notification system fires before any post-step would run).
- Rebase the worktree onto `main` and clean up the worktree.
- Move a config file, archive a plan, ping an external API.

The "allowed to fail" semantics is the key feature — these tasks are housekeeping, not validation, and a failure shouldn't flip the workflow status.

### Design

Add an optional `finalize_agent_id` field on the workflow definition pointing to an agent definition. Or — cheaper — add a `finalize` block directly on the workflow:

```sql
ALTER TABLE workflows
  ADD COLUMN finalize_command TEXT,            -- single shell command, optional
  ADD COLUMN finalize_agent_definition_id INTEGER REFERENCES agent_definitions(id),
  ADD COLUMN finalize_required BOOLEAN NOT NULL DEFAULT 0;
```

Semantics:
1. Runs **after** `markCompleted` succeeds, **before** `maybeStartNextOnSuccess` and notifications. (Order matters: finalize may add a commit; downstream chain steps should see the final tree.)
2. If `finalize_agent_definition_id` is set, spawn it as a Layer-N+1 agent with effective_mode and findings inherited from the workflow. Otherwise run `finalize_command` directly in the worktree.
3. Failure handling:
   - `finalize_required=false` (default): log failure, write a `finalize_failure` row in `errors`, **keep workflow status = completed**. Emit `workflow.finalize_failed` WS event.
   - `finalize_required=true`: failure flips workflow to `failed`. Provided as an escape hatch for users who want push-or-bust.
4. Notifications fire *after* finalize completes (success or fail), so they can include finalize result in payload.

### Why not just chain a workflow?
`next_workflow_on_success` runs as a **new workflow instance** — heavier (new instance row, new agent sessions, separate UI surface). The use cases above are single-step, want to share the worktree, and want the failure to be visible *on the parent run* rather than a child run.

### UI surface
- Workflow editor: optional "Finalize step" section with command OR agent picker, plus a "required" toggle.
- Workflow run view: dedicated finalize panel showing command, exit, output tail.

### Open questions
- Should finalize have access to the workflow's `workflow_final_result` finding via env? Yes — inject as `NRF_WORKFLOW_FINAL_RESULT`.
- Does finalize run for project-scoped workflows? Yes, with no worktree (project root).
- Does finalize run on `retry_failed`? Only if the retry succeeds — same as today's success path.
- Interaction with chain runs: if a chain step has finalize and finalize fails (non-required), does the chain continue? Yes — chain sees the step as completed.

---

## 4. Spec adoption: import external spec formats into a workflow

### Motivation
Today, work enters nrflo as a ticket created in the UI or via the API. The ticket has a title and description; everything else is built up by the L0 setup-analyzer agent at run time.

A lot of upstream context already lives in structured formats elsewhere:
- GitHub Issues (with labels, linked PRs, comments).
- OpenSpec / spec-kit Markdown specs.
- Linear / Jira tickets.
- Plain feature plan documents in repos (`docs/plans/foo.md`).

We want a one-shot import path that turns an external artifact into a populated ticket + pre-loaded findings + chosen workflow, ready to spawn. The user clicks "import," picks a source, and gets a ticket with the right scope and instructions wired up.

### Design

Two layers:

**Layer A — Import adapters.** A small interface:
```go
type SpecImporter interface {
    Name() string                // "github_issue", "openspec", "spec_kit", "markdown"
    CanHandle(input ImportInput) bool
    Import(ctx context.Context, input ImportInput) (ImportedSpec, error)
}

type ImportInput struct {
    URL          string   // for issue URLs
    Body         string   // for raw paste
    FilePath     string   // for repo-relative paths
    ProjectID    int64
}

type ImportedSpec struct {
    TicketTitle       string
    TicketDescription string
    Instructions      string
    WorkflowName      string                 // suggested
    Findings          map[string]string      // pre-seed `workflow_instances.findings`
    AttachedRefs      []string               // URLs preserved as ticket metadata
}
```

Initial adapters:
- `github_issue` — `gh api` fetch by issue URL, parse body + labels.
- `markdown` — paste-in or file path; treats H2 sections as well-known fields if recognized.
- `openspec` / `spec_kit` — recognize the format from headers, map sections to ticket fields.

The **normalization step itself uses an agent** (system agent, api-mode, low-context) — an LLM does the messy parsing rather than us writing a brittle parser per format.

**Layer B — UI + API.**
- `POST /api/v1/import/spec` with `{source, url|body|file, project_id}` returns an `ImportedSpec` preview.
- UI shows the preview, lets the user edit any field, pick the workflow, then "Create ticket" → standard ticket creation + workflow spawn.
- For project-scoped imports, skip ticket creation; pass `Instructions` directly to a project-scoped workflow start.

### Surface to ship first
- One adapter: `github_issue` (highest ROI, common entry point).
- One UI route: "Import from GitHub issue" on the tickets page.
- The agent-based normalization is the same `apirun` path used elsewhere — no new infra.

Other adapters land behind the same interface as users ask.

### Open questions
- Where do the imported `AttachedRefs` live on the ticket? New `ticket_metadata` table or just stuff them into the description? Probably a `ticket_refs` table — also useful for linking back from PRs created by workflows.
- Authentication for `github_issue` — does the server use a project-level GitHub token (new setting) or the user's? Project-level token in `project_env_vars` (read-only).
- Do we want a "watch this issue and re-import on update" mode? No — out of scope. One-shot only.
- Is the normalizing agent a built-in system agent or a user-editable agent definition? Built-in system agent — users shouldn't have to author this to get import working, but they can override via the agent_definitions table by registering one with the canonical id.

