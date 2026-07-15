# API Reference

Deep mechanics for this package. The auto-loaded map lives in [CLAUDE.md](CLAUDE.md); read this file when changing the flows below.

## Auth Middleware Chain

`requireAuth` reads the user ID from the SCS session context, loads the `model.User` from DB, stashes it in request context with `userKey`. Returns 401 if session missing, user not found, or user disabled. Returns without checking if `sessionMgr == nil` (test environments that create `*Server` directly).

`requireAuth` also accepts `Authorization: Bearer <agent_token>` (the spawned-agent path). The token is looked up via `AgentSessionRepo.GetByToken`, which only returns rows with `status IN ('running','user_interactive')`. On match, the session is stashed under `agentSessionKey` (helper: `getAgentSession(r)`). When `X-Project` is present it must equal the session's `project_id` (case-insensitive) — otherwise 403. The user context is **not** populated for bearer requests, so `requireAdmin` always 403s them.

`requireAuth` additionally accepts long-lived **service tokens** minted via Settings → Administration → Service Tokens (`service_tokens` table, sha256-hashed at rest). Lookup is `ServiceTokenService.LookupByPlaintext`; on match the principal is stashed under `servicePrincipalKey` (helper: `getServicePrincipal(r)`) and `last_used_at` is touched in a background goroutine. Tokens carry a `scope` (migration `000147`): **project** (bound to one project — X-Project mismatch is 403, satisfies `requireProjectAdmin` only for that project) or **global** (no project, `project_id` NULL — exempt from the X-Project match so it may target any project via X-Project, and satisfies `requireProjectAdmin` for every project). Neither scope satisfies the human-only `requireAdmin` or `denyNonAdminGlobalWrite`. Global tokens are admin-minted (the create route is `admin()`-gated).

`requireAdmin` wraps `requireAuth` and additionally 403s when `user.Role != admin`. Helpers `getUser(r)` / `getUserID(r)` retrieve the stashed user from context; both defined in `auth_middleware.go`.

## Admin-gated Routes

Write operations on global configuration resources require admin role (`admin`):
- `POST /api/v1/projects`, `DELETE /api/v1/projects/{id}`
- `GET|POST|PATCH|DELETE /api/v1/users/{...}` (all user management)
- `GET /api/v1/audit-log`
- `POST|PATCH|DELETE /api/v1/system-agents/{...}`
- `POST|PATCH|DELETE /api/v1/cli-models/{...}`
- `POST|PATCH|DELETE /api/v1/api-models/{...}`
- `POST|PATCH|DELETE /api/v1/default-templates/{...}` (including `/restore`)
- `POST|PATCH|DELETE /api/v1/scheduled-tasks/{...}`
- `PATCH /api/v1/settings`

Project-scoped writes use `projectAdmin` (admin user **or** a service token whose project matches the resolved project — for path-scoped routes via `{id}`, otherwise via `getProjectID` = X-Project header / `?project=` query):
- `PUT|DELETE /api/v1/projects/{id}/env-vars/{name}`
- `PUT /api/v1/projects/{id}/settings/{cleanup,artifact-storage,observer,capture-thinking}`
- `DELETE /api/v1/artifacts/{aid}`
- `POST|PATCH|DELETE /api/v1/python-scripts/{scriptId}` (scope from request, not the path; `{scriptId}` is the script ID)

All reads on those resources are `protected` (requireAuth only). All other routes are `protected`.

Workflow-def / agent-def / import writes are `protected`, but mutating the reserved `__global__` project's definitions additionally requires an admin user via `denyNonAdminGlobalWrite` (`auth_middleware.go`), called in each mutating handler — a per-request check because the requirement is conditional on `projectID == "__global__"` (per-project workflow CRUD is unaffected). Bearer/service principals are denied. The agent socket path enforces the same invariant via `socket.denyGlobalWorkflowMutation`.

## External MCP proxy (`agent mcp-external`)

Any standalone MCP client (not spawned by nrflo, not Claude-specific) drives the server via `nrflo_server agent mcp-external` (`cli/agent_mcp_external.go`) — a dumb JSON-RPC stdio bridge with zero per-tool code. Auth is a long-lived **service token** (`NRFLO_MCP_TOKEN`, sent `Authorization: Bearer`); base URL `NRFLO_SERVER_URL` (default `http://127.0.0.1:6587`).

**Startup**: resolve the session project — **cwd auto-detect** (the bridge's working directory matched against project `root_path`s, longest-prefix; `cli/agent_mcp_external_cwd.go`) → `NRFLO_PROJECT` default → the hidden global project (never errors) — then exchange the service token for a console session via `POST /api/v1/console/sessions` (service token, `X-Project: <resolved project>`) → `{session_id, token}`. A **global** service token can open a session for any project; a **project** token only its own (403 otherwise, surfaced with the resolved project id for diagnosis). The resolved project is fixed for the life of the connection — console tool schemas carry no `project` arg.

**Adopted-session path**: when `NRFLO_CONSOLE_TOKEN`/`NRFLO_CONSOLE_SESSION_ID` are both set (injected by `nrflo_server console`), startup skips `POST /api/v1/console/sessions` entirely — `adoptConsoleSession` installs the session id/bearer directly and pins the project from `NRFLO_PROJECT`, with `cwdResolved` forced true so cwd auto-detect/the projects listing never runs. `ownsSession` stays false, so shutdown does not close it — the parent `console` process, which minted the session, owns that. A 401 re-exchange still requires `NRFLO_MCP_TOKEN` (the service token): if absent, `reopenSession` errors instead of retrying with an empty bearer; if present, the re-exchange opens a NEW session that this client DOES own (`openConsoleSession` sets `ownsSession=true`), so shutdown closes that one normally.

**Steady state**: `tools/list` → `GET /api/v1/console/tools`, mapped `{name, description, input_schema}` → MCP `{name, description, inputSchema}` (empty schema defaults to `{"type":"object"}`). `tools/call` → `POST /api/v1/console/tools/{name}/call` with the CONSOLE token, body `{"arguments": <raw args, "{}" when empty>}` → `{output, is_error}` becomes the MCP tool result; a transport/HTTP error (incl. unlisted-tool 404) surfaces as `isError: true` tool text, not a protocol error. A 401 (console row swept idle server-side, `ConsoleService.SweepIdle`) triggers one re-exchange of a fresh console session and one retry — never loops. `runMCPStdioLoopWithCancel` dispatches every request in its own goroutine against the one shared client, so the session state (`consoleToken`/`sessionID`/`sessionProject` + the cwd cache) is mutex-guarded and the re-exchange is **single-flighted** — concurrent 401s reuse the first goroutine's new session instead of each opening one and leaking all but the last; the re-exchange reuses the pinned project, so a connection never migrates. The tool catalogue is entirely server-owned: whatever the server's console profile serves (`internal/console/`, see [console/CLAUDE.md](../console/CLAUDE.md)) is what the bridge exposes; `dynamic_workflow`/`revise_plan`/`approve_plan` are NOT served (the console profile excludes the plan builtins — they are parent-instance-ownership-guarded, `orchestrator.RevisePlan`/`ApprovePlan` call `assertChildOwnership`, `StartDynamicWorkflow` needs a parent instance id). The plan lifecycle stays reachable from the UI, spawned agents, and the REST plan routes directly (below).

**Shutdown**: on stdio EOF or cancellation, `closeConsoleSession` best-effort calls `POST /api/v1/console/sessions/{sid}/close` with the CONSOLE token, using its own short `context.Background()` timeout (the loop's parent ctx is already cancelled by then) — but only when `ownsSession` is true (see the adopted-session path above; an adopted session is a no-op here). Cancellation propagates as an HTTP client disconnect: `notifications/cancelled` (or stdin EOF) cancels the in-flight request's context (`runMCPStdioLoopWithCancel`), which cancels `r.Context()` server-side, which makes a blocking console handler (e.g. `console.deepResearchHandler`) stop its run — with zero bridge-side stop logic. Deep-research can take minutes, so the MCP tool-call timeout must be raised on the client; the bridge's own HTTP client has no `Timeout`. Connect: `claude mcp add nrflo --env NRFLO_MCP_TOKEN=… [--env NRFLO_PROJECT=…] -- nrflo_server agent mcp-external`.

## WS / PTY Auth

`GET /api/v1/ws` and `GET /api/v1/pty/{session_id}` use `requireAuthWith(true, ...)`: 401 before the WS upgrade, plus a `?token=<bearer>` query fallback (browsers can't set `Authorization` on WebSocket constructors). All other endpoints use `requireAuth` (header-only). PTY upgrade/resize/exit-interactive live in `handlers_pty.go`.

CORS `Access-Control-Allow-Headers` includes `Authorization` so cross-origin REST preflight succeeds when the UI sends a Bearer token. `Access-Control-Allow-Credentials` is not set.

## Pause-Continue-Fail Routes

`POST /api/v1/tickets/{id}/workflow/continue` — body `{workflow, instructions?}`: resume a waiting ticket-scoped instance (resolves most-recent waiting instance for that workflow). `POST /api/v1/tickets/{id}/workflow/fail` — body `{workflow, reason}` (reason required): fail an active or waiting ticket-scoped instance.

`POST /api/v1/projects/{id}/workflow/continue` — body `{instance_id, instructions?}`: resume a waiting project-scoped instance by ID. `POST /api/v1/projects/{id}/workflow/fail` — body `{instance_id, reason}`: fail an active or waiting project-scoped instance by ID.

All four routes are `protected` (accept SCS sessions, spawn tokens, and service tokens).

## Plan Routes

`GET /api/v1/workflow-instances/{iid}/plan` (draft + latest manifest + template library), `GET .../plan/revisions` (full history), `POST .../plan/revise` (edited manifest OR feedback+answers → planner re-run; body `types.PlanReviseRequest`), `POST .../plan/approve` (`types.PlanApproveRequest`), `POST .../plan/cancel`. All `protected` — plan revise/approve/cancel are per-instance RUNTIME operations, not `__global__` definition mutations, so (unlike workflow/agent-def CRUD) `denyNonAdminGlobalWrite` is deliberately NOT called here: a spawned agent's spawn token or a service token must be able to drive a `dynamic_workflow` run living in the hidden `__global__` project. Revise/approve are revision-pinned — a stale `revision` is 409.

`POST /api/v1/projects/{id}/dynamic-workflow` (`protected`, body `{instructions, mode?}`) starts the bundled plan-driven `dynamic` workflow; `mode="auto"` 400s unless `dynamic_workflow_auto_enabled`. These lifecycle operations are NOT exposed to external MCP clients — `agent mcp-external`'s console profile does not serve `dynamic_workflow`/`revise_plan`/`approve_plan` (see [console/CLAUDE.md](../console/CLAUDE.md)); the routes above remain the only way to drive them from outside the UI/spawned agents.

Approve now materializes in the same request (`PlanService.Approve` → `Materialize`, DYNWF-5): a materialization failure surfaces as the same 4xx as any other approve error. `handleApprovePlan` then broadcasts `ws.EventPlanMaterialized` and, if the instance parked at the plan boundary (`model.IsPlanSuspended`), calls `PlanResumer.ResumeAfterPlanApproval` (interface declared in `handlers_plan.go`, satisfied by `s.orchestrator`) to relaunch the run at the first materialized layer — a no-op if the run is still active (its own `runLoop` will materialize inline). See `handlers_plan.go` + `service/plan.go` + [orchestrator/CLAUDE.md](../orchestrator/CLAUDE.md#plan-boundary--materialization).

## Console sessions

`POST /api/v1/console/sessions` (`projectAdmin`; project from X-Project) creates a `kind='console'` `agent_sessions` row (`status=user_interactive`, `ticket_id=''`, NULL `workflow_instance_id`) and returns `{session_id, token}` once. `POST /api/v1/console/sessions/{sid}/close` (`protected`) authorizes in-handler (admin, OR service principal Global/project-match, OR the session's own bearer) then flips `status=interactive_completed`, killing the token via `GetByToken`'s status filter. Idle rows sweep every 20 min via `ConsoleService.SweepIdle` (`console_idle_ttl_hours`, default 12).

#### Console tools

`GET /api/v1/console/tools` and `POST /api/v1/console/tools/{name}/call` (both `protected`) expose the `internal/console/` tool profile to a console bearer. Both handlers call `requireConsoleSession(r)` first: `getAgentSession(r) == nil || sess.Kind != model.AgentSessionKindConsole` → 401 — this also covers a closed session's token, since `GetByToken`'s status filter already makes it resolve to `nil` (auth_middleware.go:80-90), and a non-console bearer (service token, project-scoped agent session).

**Catalogue** — `GET /api/v1/console/tools` → `{"tools":[{"name","description","input_schema":<raw JSON schema>}]}`, sorted by name. Built per request from `console.BuildRegistry(consoleDeps())` (stateless, cheap — mirrors `handlers_available_tools.go`'s per-call `tools_builtin.Builtins()`).

**Dispatch** — `POST /api/v1/console/tools/{name}/call` with body `{"arguments":{...}}` (absent/empty body → `{}`; malformed JSON → 400) → 200 `{"output":"...","is_error":bool,"duration_ms":N}`. An unlisted `{name}` dispatches to `console.ErrToolNotFound` → 404. The tool's `env.ProjectID` is `consoleToolProject(r, sess)`: the session's own project by default; an `X-Project`/`?project=` override is honored only when the session is global-scope (`sess.ProjectID == service.GlobalProjectID`) — a project-scoped session's auth middleware already forces `X-Project` to match, so there is nothing to override.

**Profile** — `console.BuildRegistry` composes an explicit allowlist of session-independent `tools_builtin.Builtins()` entries (`project_findings_*`, `workflow_continue`, `workflow_fail`, `ticket_create`, `ticket_add_dependency`, `web_search`, `web_fetch`) with console-only handlers that take an explicit `instance_id`/`ticket_id` instead of reading it off the (session-less) `ToolEnv`: `workflow_run`/`workflow_stop`/`workflow_retry_failed`/`workflow_get`/`workflow_list`, `project_list`/`project_status`, `ticket_list`/`ticket_get`, `artifact_list`/`artifact_get`, `deep_research`. Full profile invariant + `ArtifactSvc`-nil rationale: [console/CLAUDE.md](../console/CLAUDE.md).

**Audit** — every call (regardless of outcome) writes one `audit_log` row via `appendConsoleToolAudit`: `action="console.tool.call"`, `resource_type="agent_session"`, `resource_id=<console session id>`, `metadata={"tool","args_digest":sha256-hex(args)[:16],"duration_ms","outcome":"ok"|"tool_error"|"error"|"not_found","project"}`. `metadata.project` is the project the call **acted on** (`consoleToolProject(r, sess)`), not `sess.ProjectID` — a global-scope session that retargets via `X-Project` must be audited against the project it actually touched. `GET /api/v1/audit-log?resource_type=agent_session&resource_id=<id>` (indexed by migration `000163`) returns one session's full tool-call trail.
