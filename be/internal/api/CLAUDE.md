# API Package

HTTP API server providing REST endpoints and WebSocket for the web UI. Deep flow mechanics: [REFERENCE.md](REFERENCE.md) — read it before changing auth middleware, route gating, the external MCP proxy, plan routes, or console sessions.

## Server Architecture

`nrflo_server serve` provides:
- **HTTP API** on port 6587 — web UI, REST API, WebSocket
- **CORS** disabled by default (same-origin serving); configurable via `cors_origins` in config file. `X-Request-ID` is exposed and allowed via CORS headers
- **Request ID** middleware generates a trx (`logger.NewTrx()`) per HTTP request, injects it into context via `logger.WithTrx()`, and sets `X-Request-ID` response header
- **WebSocket** at `/api/v1/ws` for real-time updates

## Authentication

### Middleware Chain

`Start()` assembles: `cors → requestID → projectMiddleware → LoadAndSave (for /api/* only) → mux`

Per-route auth is applied at registration time via three helpers in `registerRoutes`:
- `protected(pat, h)` — wraps with `requireAuth` (valid session required)
- `admin(pat, h)` — wraps with `requireAdmin` (admin role required)
- Plain `mux.HandleFunc(pat, h)` — public (no auth); used only for `POST /api/v1/auth/login`

`requireAuth` accepts SCS cookie sessions plus two bearer forms: spawned-agent tokens (valid only while the session is `running`/`user_interactive`) and long-lived service tokens (**project** or **global** scope; only these satisfy `requireProjectAdmin`). Bearer requests never populate the user context, so the human-only `requireAdmin` always 403s them. Mechanics: [REFERENCE.md](REFERENCE.md#auth-middleware-chain) — read before changing token acceptance, context keys, or X-Project matching.

### Admin-gated Routes

Writes on global configuration resources require an admin user (`admin()`); project-scoped writes use `projectAdmin` (admin user **or** a service token matching the resolved project); all reads on those resources — and all other routes — are `protected`. Mutating the reserved `__global__` project's workflow/agent definitions additionally requires an admin user via `denyNonAdminGlobalWrite` (per-request check in each mutating handler; socket path mirrors it via `socket.denyGlobalWorkflowMutation`). Full route lists: [REFERENCE.md](REFERENCE.md#admin-gated-routes) — read before changing route gating.

### External MCP proxy (`agent mcp-external`)

A **standalone** Claude Code session (not spawned by nrflo) drives the server via `nrflo_server agent mcp-external` — a service-token-authed JSON-RPC stdio MCP proxy over this REST API (`cli/agent_mcp_external.go`; token in `NRFLO_MCP_TOKEN`). The target project resolves per tool call (explicit `project` arg → cwd auto-detect → `NRFLO_PROJECT` → hidden global project) and never errors. Mechanics + tool list: [REFERENCE.md](REFERENCE.md#external-mcp-proxy-agent-mcp-external) — read before changing tools, auth, or project resolution.

### Login Rate Limiter

`auth_ratelimit.go`: per-IP+email token bucket, 5 attempts/5min. Over limit → 429 with `Retry-After`. Keys are `{ip}|{email}`.

`serve --insecure-cookies` passes `dev=true` to `auth.NewManager` (drops the `Secure` cookie flag; local HTTP dev).

### WS / PTY Auth

`GET /api/v1/ws` and `GET /api/v1/pty/{session_id}` authenticate **before** the WS upgrade and accept a `?token=<bearer>` query fallback; all other endpoints are header-only `requireAuth`. Mechanics + CORS header details: [REFERENCE.md](REFERENCE.md#ws--pty-auth) — read before changing WS/PTY auth.

## Handlers

Handlers live in `handlers_*.go` files. For the route table run:
```
grep -rn "protected\|admin(\|mux.HandleFunc" be/internal/api/server*.go
```

Errors are returned as `{"error":"code","message":"..."}` for structured failures, or plain text on framework-level 4xx rejections.

`GET /api/v1/import/jira/search` and `GET /api/v1/import/github/search` return 400 when `X-Project` is missing (matching `POST /api/v1/import/spec`).

## Pause-Continue-Fail Routes

Four `protected` routes resume (`continue`, optional instructions) or fail (`fail`, reason required) a waiting/active instance: ticket-scoped resolves by workflow name, project-scoped by `instance_id`. Bodies + resolution rules: [REFERENCE.md](REFERENCE.md#pause-continue-fail-routes).

## Endless Loop Mode

`POST /api/v1/projects/{id}/workflow/run` accepts `endless_loop: bool` (mutually exclusive with `interactive`/`plan_mode`; requires project-scope workflow). `POST .../stop-endless-loop` toggles the graceful-stop flag on an active instance without interrupting the in-flight iteration. See `handlers_project_workflow.go` for validation details.

## Plan Routes

Plan lifecycle endpoints (`/api/v1/workflow-instances/{iid}/plan*`, `POST /api/v1/projects/{id}/dynamic-workflow`) are all `protected` per-instance RUNTIME operations — deliberately NOT gated by `denyNonAdminGlobalWrite`, so spawn/service tokens can drive `dynamic_workflow` runs in `__global__`; revise/approve are revision-pinned (stale `revision` → 409). Approve materializes in the same request and resumes a plan-suspended run via `PlanResumer.ResumeAfterPlanApproval`. Mechanics: [REFERENCE.md](REFERENCE.md#plan-routes) — read before changing plan auth or the approve flow; see also [orchestrator/CLAUDE.md](../orchestrator/CLAUDE.md#plan-boundary--materialization).

## Observers

`POST /api/v1/observers` accepts `{scope: "workflow"|"project"|"global", project_id?, workflow_id?}` and returns `{session_id}` (handlers_observer.go:13). Returns 404 when `experimental_observer_enabled=false`. `GET /api/v1/observers` returns active observer sessions filtered by X-Project (handlers_observer.go:57).

## Console sessions

`POST /api/v1/console/sessions` (`projectAdmin`) mints a `kind='console'` `agent_sessions` row and returns `{session_id, token}` once; `POST .../close` kills the token. That row shape is identical to a project-scoped agent, so `kind` alone excludes console rows from kill/resume, `GetByProjectScope`, `ListFinished` counts, and daily stats. Close authorization + idle sweep: [REFERENCE.md](REFERENCE.md#console-sessions).
