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

Any standalone MCP client (not spawned by nrflo, not Claude-specific) drives the server via `nrflo_server agent mcp-external` — a service-token-authed (`NRFLO_MCP_TOKEN`) dumb JSON-RPC stdio bridge (`cli/agent_mcp_external.go`) that opens a console session at connect and forwards `tools/list`/`tools/call` to `GET/POST /api/v1/console/tools*` with the console bearer, closing the session at disconnect. Console-chat Claude/Codex engines instead inject a pre-minted session (`NRFLO_CONSOLE_TOKEN`/`NRFLO_CONSOLE_SESSION_ID`) for the bridge to adopt; `ChatService` owns its close. The tool catalogue is entirely server-owned (the console profile); the bridge contains zero per-tool code. The project resolves once, at session creation (cwd auto-detect → `NRFLO_PROJECT` → hidden global project). Mechanics: [REFERENCE.md](REFERENCE.md#external-mcp-proxy-agent-mcp-external) — read before changing the bridge, auth, or project resolution.

### Login Rate Limiter

`auth_ratelimit.go`: per-IP+email token bucket, 5 attempts/5min. Over limit → 429 with `Retry-After`. Keys are `{ip}|{email}`.

`serve --insecure-cookies` passes `dev=true` to `auth.NewManager` (drops the `Secure` cookie flag; local HTTP dev).

### WS / PTY Auth

`GET /api/v1/ws` and `GET /api/v1/pty/{session_id}` authenticate **before** the WS upgrade and accept a `?token=<bearer>` query fallback; all other endpoints are header-only `requireAuth`. Mechanics + CORS header details: [REFERENCE.md](REFERENCE.md#ws--pty-auth) — read before changing WS/PTY auth.

### Resume Session

`pty.Manager` has **no default command**: `Create()` fails with `no PTY launch registered` unless a `pty.Launch` was registered for the session id. So both resume-session routes (`POST /api/v1/{tickets,projects}/{id}/workflow/resume-session`) go through `startResumeSession` (`handlers_resume_launch.go`), which registers the adapter-built resume launch via `spawner.GetCLIAdapter` **before** flipping the session to `user_interactive` — registering late (or not at all) would strand the PTY attach the UI opens next.

## Handlers

Handlers live in `handlers_*.go` files. For the route table run:
```
grep -rn "protected\|admin(\|mux.HandleFunc" be/internal/api/server*.go
```

Errors are returned as `{"error":"code","message":"..."}` for structured failures, or plain text on framework-level 4xx rejections.

`GET /api/v1/sessions/{id}/context-ledger` (`protected`) is a read-only debug snapshot of the spawner's in-memory context ledger for that session (`spawner.LedgerSnapshot`); 404 once the session's ledger is dropped.

Global model administration uses `/api/v1/models` CRUD plus `POST /api/v1/models/{id}/test`; the test route probes CLI mode only and rejects API-only rows. Writes are admin-only; reads and the test route are protected.

`/api/v1/custom-providers` CRUD (registry of BYO OpenAI-compatible providers) is entirely admin-only, including reads: the row's `api_key` serializes in plaintext, so unlike models a non-admin/bearer caller must never see it.

`POST /api/v1/custom-providers/check` (admin-only) probes a base URL's `/models` endpoint and returns `{ok, models, error}`; connectivity/upstream failures are a 200 body, not an HTTP error — only a malformed `base_url` 400s.

`GET /api/v1/import/jira/search` and `GET /api/v1/import/github/search` return 400 when `X-Project` is missing (matching `POST /api/v1/import/spec`).

`GET /api/v1/tier-models` (`protected`) returns all tier fallback-chain rows (tier 1-5, ordered by position); `PUT /api/v1/tier-models/{tier}` (`admin`) replaces one tier's ordered chain (`handlers_tier_models.go`), validating each entry via `TierModelService.SetTierChain` and broadcasting `tier_models.updated`.

`GET /api/v1/system-agent-runs` (`admin`, `server_routes_observability.go`/`handlers_system_agent_runs.go`) merges recent SYSTEM-agent `agent_sessions` (resolved tier/provider/effort + fallback chain) with recent `refinery_runs` rows, newest-first; supports `limit` (default 50, clamp 1-200) and `since` (RFC3339).

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

#### Console tools

`GET/POST /api/v1/console/tools*` expose a server-owned tool catalogue + dispatcher (`internal/console/`) to a console bearer, authenticated in-handler by `requireConsoleSession`. Mechanics: [REFERENCE.md](REFERENCE.md#console-tools).

`GET /api/v1/console/skills` follows the same `protected` route + in-handler auth pattern: a console/console_chat bearer is served its own session's project (`X-Project`/`?project=` ignored); admin users and service tokens keep `getProjectID`-based scoping.

`GET /api/v1/console/history` returns the project's recent `console_chat` `user_input` message contents (`?limit`, default/clamp ≤100, oldest→newest), same `protected` route + in-handler auth as `/console/skills`.

#### Console chats

`GET /api/v1/console/catalog` discovers enabled engines/models and live resumable chats; `POST /api/v1/console/chats` starts a `ChatService`-owned engine. Path-scoped routes cover paginated history, approval, interruption, reconnect detail (including bounded in-flight output), and close under the shared chat authorization predicate. Live events stream over the WS session channel — see [ws/CLAUDE.md](../ws/CLAUDE.md). `GET /api/v1/pty/{sid}` on a `kind='console_chat'` row routes to the viewer-only relay (`handlers_pty_console.go`): raw terminal onto a live claude chat's PTY, detach-on-disconnect, never completes or kills anything. `GET .../tools` (the chat's own catalogue) and `POST .../invoke` (deterministic server-side dispatch, transcript rows + optional inform-model) live under the same shared chat auth predicate as the routes above.
