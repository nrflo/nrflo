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

A **standalone** Claude Code session (not spawned by nrflo) drives the server via `nrflo_server agent mcp-external` (`cli/agent_mcp_external.go`) — a token-authed JSON-RPC stdio MCP server that proxies tool calls to this REST API. Auth is a long-lived **service token** (`NRFLO_MCP_TOKEN`, sent `Authorization: Bearer`); base URL `NRFLO_SERVER_URL` (default `http://127.0.0.1:6587`). The target project is resolved per tool call, in order: explicit `project` arg → **cwd auto-detect** (the proxy's working directory matched against project `root_path`s, longest-prefix; `cli/agent_mcp_external_cwd.go`) → `NRFLO_PROJECT` default → the hidden global project. It never errors — project-agnostic tools (`deep_research`) run in the global project from any directory with no config; a project-specific workflow run there just 404s. A **global** token can drive every project; a **project** token only its own. Tools: `deep_research` (runs the global `deep-research` workflow then blocks-polls until terminal and returns `state.workflow_findings.report`; optional `context` arg is forwarded as the run's `external_context` so the scope agent can ground the angles via `${EXTERNAL_CONTEXT}` — empty = project-agnostic web research), `run_workflow`, `get_workflow`, `list_workflows` — thin wrappers over `POST .../workflow/run`, `GET .../workflow?instance_id=`, `GET /api/v1/workflows` — plus `dynamic_workflow`/`revise_plan`/`approve_plan` (plan lifecycle, below; `dynamic_workflow` blocks-polls to the plan boundary and folds the full draft into the returned `state.plan`). Connect: `claude mcp add nrflo --env NRFLO_MCP_TOKEN=… [--env NRFLO_PROJECT=…] -- nrflo_server agent mcp-external`. Deep-research can take minutes, so the MCP tool-call timeout must be raised on the client. A cancelled or timed-out `deep_research` (MCP `notifications/cancelled`, or the proxy's stdin closing because the client killed it) best-effort calls `POST .../workflow/stop` for the in-flight instance so the run doesn't orphan and keep billing (`runMCPStdioLoopWithCancel` threads a per-request context into the dispatch; the session-bound `agent mcp` bridge keeps the plain `runMCPStdioLoop`).

## WS / PTY Auth

`GET /api/v1/ws` and `GET /api/v1/pty/{session_id}` use `requireAuthWith(true, ...)`: 401 before the WS upgrade, plus a `?token=<bearer>` query fallback (browsers can't set `Authorization` on WebSocket constructors). All other endpoints use `requireAuth` (header-only). PTY upgrade/resize/exit-interactive live in `handlers_pty.go`.

CORS `Access-Control-Allow-Headers` includes `Authorization` so cross-origin REST preflight succeeds when the UI sends a Bearer token. `Access-Control-Allow-Credentials` is not set.

## Pause-Continue-Fail Routes

`POST /api/v1/tickets/{id}/workflow/continue` — body `{workflow, instructions?}`: resume a waiting ticket-scoped instance (resolves most-recent waiting instance for that workflow). `POST /api/v1/tickets/{id}/workflow/fail` — body `{workflow, reason}` (reason required): fail an active or waiting ticket-scoped instance.

`POST /api/v1/projects/{id}/workflow/continue` — body `{instance_id, instructions?}`: resume a waiting project-scoped instance by ID. `POST /api/v1/projects/{id}/workflow/fail` — body `{instance_id, reason}`: fail an active or waiting project-scoped instance by ID.

All four routes are `protected` (accept SCS sessions, spawn tokens, and service tokens).

## Plan Routes

`GET /api/v1/workflow-instances/{iid}/plan` (draft + latest manifest + template library), `GET .../plan/revisions` (full history), `POST .../plan/revise` (edited manifest OR feedback+answers → planner re-run; body `types.PlanReviseRequest`), `POST .../plan/approve` (`types.PlanApproveRequest`), `POST .../plan/cancel`. All `protected` — plan revise/approve/cancel are per-instance RUNTIME operations, not `__global__` definition mutations, so (unlike workflow/agent-def CRUD) `denyNonAdminGlobalWrite` is deliberately NOT called here: a spawned agent's spawn token or an `mcp-external` service token must be able to drive a `dynamic_workflow` run living in the hidden `__global__` project. Revise/approve are revision-pinned — a stale `revision` is 409.

`POST /api/v1/projects/{id}/dynamic-workflow` (`protected`, body `{instructions, mode?}`) starts the bundled plan-driven `dynamic` workflow; `mode="auto"` 400s unless `dynamic_workflow_auto_enabled`. Same three lifecycle operations are also exposed as `dynamic_workflow`/`revise_plan`/`approve_plan` on `agent mcp-external`, backed by these same routes.

Approve now materializes in the same request (`PlanService.Approve` → `Materialize`, DYNWF-5): a materialization failure surfaces as the same 4xx as any other approve error. `handleApprovePlan` then broadcasts `ws.EventPlanMaterialized` and, if the instance parked at the plan boundary (`model.IsPlanSuspended`), calls `PlanResumer.ResumeAfterPlanApproval` (interface declared in `handlers_plan.go`, satisfied by `s.orchestrator`) to relaunch the run at the first materialized layer — a no-op if the run is still active (its own `runLoop` will materialize inline). See `handlers_plan.go` + `service/plan.go` + [orchestrator/CLAUDE.md](../orchestrator/CLAUDE.md#plan-boundary--materialization).

## Console sessions

`POST /api/v1/console/sessions` (`projectAdmin`; project from X-Project) creates a `kind='console'` `agent_sessions` row (`status=user_interactive`, `ticket_id=''`, NULL `workflow_instance_id`) and returns `{session_id, token}` once. `POST /api/v1/console/sessions/{sid}/close` (`protected`) authorizes in-handler (admin, OR service principal Global/project-match, OR the session's own bearer) then flips `status=interactive_completed`, killing the token via `GetByToken`'s status filter. Idle rows sweep every 20 min via `ConsoleService.SweepIdle` (`console_idle_ttl_hours`, default 12).
