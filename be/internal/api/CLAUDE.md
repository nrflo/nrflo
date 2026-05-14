# API Package

HTTP API server providing REST endpoints and WebSocket for the web UI.

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

`requireAuth` reads the user ID from the SCS session context, loads the `model.User` from DB, stashes it in request context with `userKey`. Returns 401 if session missing, user not found, or user disabled. Returns without checking if `sessionMgr == nil` (test environments that create `*Server` directly).

`requireAuth` also accepts `Authorization: Bearer <agent_token>` (the spawned-agent path). The token is looked up via `AgentSessionRepo.GetByToken`, which only returns rows with `status IN ('running','user_interactive')`. On match, the session is stashed under `agentSessionKey` (helper: `getAgentSession(r)`). When `X-Project` is present it must equal the session's `project_id` (case-insensitive) — otherwise 403. The user context is **not** populated for bearer requests, so `requireAdmin` always 403s them.

`requireAdmin` wraps `requireAuth` and additionally 403s when `user.Role != admin`. Helpers `getUser(r)` / `getUserID(r)` retrieve the stashed user from context; both defined in `auth_middleware.go`.

### Admin-gated Routes

Write operations on configuration resources require admin role:
- `POST /api/v1/projects`, `DELETE /api/v1/projects/{id}`
- `GET|POST|PATCH|DELETE /api/v1/users/{...}` (all user management)
- `GET /api/v1/audit-log`
- `POST|PATCH|DELETE /api/v1/system-agents/{...}`
- `POST|PATCH|DELETE /api/v1/cli-models/{...}`
- `POST|PATCH|DELETE /api/v1/default-templates/{...}` (including `/restore`)
- `POST|PATCH|DELETE /api/v1/scheduled-tasks/{...}`
- `PUT|DELETE /api/v1/projects/{id}/env-vars/{name}` (project-scoped)
- `POST|PATCH|DELETE /api/v1/python-scripts/{...}` (project-scoped)
- `POST|PUT|DELETE /api/v1/tool-definitions/{...}` (api-mode only)
- `POST|PUT|DELETE /api/v1/api-credentials/{...}` (api-mode only)
- `PATCH /api/v1/settings`
- `PATCH /api/v1/providers/{name}`

All reads on those resources are `protected` (requireAuth only). All other routes are `protected`.

### Login Rate Limiter

`auth_ratelimit.go` implements a per-IP+email token bucket: 5 attempts per 5-minute sliding window. On limit exceeded, returns HTTP 429 with `Retry-After` header (seconds). Keys are `{ip}|{email}`.

### --insecure-cookies Flag

`nrflo_server serve --insecure-cookies` passes `dev=true` to `auth.NewManager`, disabling the `Secure` cookie flag. Use for local HTTP development without TLS.

### WS / PTY Auth

`GET /api/v1/ws` and `GET /api/v1/pty/{session_id}` are registered via `requireAuth`, so the 401 is returned before any WebSocket upgrade handshake. PTY upgrade, resize handling, and exit-interactive wiring are in `handlers_pty.go`.

## Handlers

Handlers live in `handlers_*.go` files. For the route table run:
```
grep -rn "protected\|admin(\|mux.HandleFunc" be/internal/api/server.go
```

Errors are returned as `{"error":"code","message":"..."}` for structured failures, or plain text on framework-level 4xx rejections.

## Endless Loop Mode

`POST /api/v1/projects/{id}/workflow/run` accepts `endless_loop: bool` (mutually exclusive with `interactive`/`plan_mode`; requires project-scope workflow). `POST .../stop-endless-loop` toggles the graceful-stop flag on an active instance without interrupting the in-flight iteration. See `handlers_project_workflow.go` for validation details.
