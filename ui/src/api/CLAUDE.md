# API Client

API client modules for communicating with the nrflo backend.

## Architecture

- All API calls go through `client.ts`: fetch wrapper that resolves baseURL/project/auth at call time from the active `connectionsStore` connection (`requestConfig()` in `client.ts`). Local connections use cookie auth (`credentials: 'include'`); remote connections use `Authorization: Bearer <token>` with `credentials: 'omit'`. Exposes `UnauthenticatedError`/`ForbiddenError` subclasses and a global 401 handler.
- Project scope via `X-Project` header or `?project=` query parameter; project ID lives on the active connection (`connectionsStore.active().activeProject`).
- TanStack Query handles caching and refetching in the hooks layer; see [hooks/CLAUDE.md](../hooks/CLAUDE.md).
- Vite proxies `/api` to the backend in development (including WebSocket via `ws: true`).

Modules under `src/api/` wrap `apiFetch`; one file per resource group, mirroring the backend route grouping. Run `ls ui/src/api/` for the full list.

All responses use `{error: {code, message}}` for error bodies. Backend route enumeration: [be/internal/api/CLAUDE.md](../../../be/internal/api/CLAUDE.md).

## Request / Response Conventions

- All mutating requests include `Content-Type: application/json` (except `putConfigFile` which sends `text/plain`).
- `X-Project` header is required for project-scoped endpoints; omitted for global endpoints (users, audit log, system agents, default templates, CLI models, settings, docs).
- Path segments containing special characters are encoded via `encodeURIComponent` (env var names, config file paths).
- `apiFetchWith412` detects 412 and throws `NotConfiguredError` instead of `ApiError` (spec import, GitHub/Jira search).

## Error Classes

Both exported from `client.ts`:

- `UnauthenticatedError` (extends `ApiError`) — thrown on 401 for any endpoint except `POST /api/v1/auth/login`.
- `ForbiddenError` (extends `ApiError`) — thrown on 403; does not trigger the 401 handler.

## Global 401 Handler

`client.ts` exports `set401Handler(fn)`. Signature: `(path, { isLocal, connectionId }) => void`. When any request (except `POST /api/v1/auth/login`) returns 401, it throws `UnauthenticatedError` then calls the registered handler with `pathname + search` plus the active connection context.

`AuthGate` registers this handler on first mount. For local connections: calls `useAuthStore.getState().clear()` and navigates to `/login?next=<encoded path>` via `window.history.pushState` + popstate event, unless already on `/login`. For remote connections: calls `useConnectionsStore.getState().markAuthFailed(connectionId)` without navigating (the `AuthFailedBanner` surfaces the failure).

`testConnection(conn)` in `client.ts` — bypasses the 401 handler and targets a specific `Connection` directly (used by `ConnectionsPage` and `ConnectionAddDialog` for inline connectivity checks).

## Connections Store

`src/stores/connectionsStore.ts` (Zustand + persist key `nrf_connections`) holds the list of nrflo server connections (one implicit `Local` entry plus zero-or-more remotes) and the active one. The active project is owned by the connection record (`activeProject`); `projectStore.ts` keeps the projects-list cache and write-throughs to `connectionsStore.setActiveProject`.
