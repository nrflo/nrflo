# API Client

API client modules for communicating with the nrflo backend.

## Architecture

- All API calls go through `client.ts`: configured fetch wrapper with `credentials: 'include'`, `X-Project` header injection, and `UnauthenticatedError`/`ForbiddenError` subclasses.
- Project scope via `X-Project` header or `?project=` query parameter.
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

`client.ts` exports `set401Handler(fn)`. When any request (except `POST /api/v1/auth/login`) returns 401, it throws `UnauthenticatedError` then calls the registered handler with `pathname + search`.

`AuthGate` registers this handler on first mount: calls `useAuthStore.getState().clear()` and navigates to `/login?next=<encoded path>` via `window.history.pushState` + popstate event, unless already on `/login`.
