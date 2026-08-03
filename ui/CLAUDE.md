# Claude Code Instructions for nrflo UI

## Overview

Web UI for the nrflo ticket management system: React 18 + TypeScript + Vite, TanStack Query, Zustand, Tailwind CSS v4, React Router v6. Communicates with `nrflo_server serve` API.

Source lives under `ui/src/`. Run `ls ui/src/` for top-level directories. Deep documentation: [api/CLAUDE.md](src/api/CLAUDE.md), [hooks/CLAUDE.md](src/hooks/CLAUDE.md), [workflow/CLAUDE.md](src/components/workflow/CLAUDE.md), [pages/CLAUDE.md](src/pages/CLAUDE.md), [types/CLAUDE.md](src/types/CLAUDE.md).

## Development Commands

```bash
make build-ui                         # Production build (includes tsc)
make test-ui                          # Run UI tests (60s wall-time constraint)
make test-ui ARGS="src/components/"   # Directory filter
npm run dev                           # Vite dev server (port 5175, hot-reload)
npx tsc --noEmit                      # TypeScript check only
```

## Architecture Patterns

- All API calls go through `src/api/client.ts`; project scope injected via `X-Project` header.
- **Server state**: TanStack Query (`useQuery`, `useMutation`); query keys in `src/hooks/useTickets.ts`.
- **Client state**: Zustand — project selection (`projectStore.ts`), theme (`themeStore.ts`), auth (`authStore.ts`), interactive PTY sessions (`interactiveSessionsStore.ts`).
- Vite proxies `/api` to the backend in development (`ws: true` for WebSocket).
- Projects loaded from `/api/v1/projects` when auth status becomes `authed` (see `App.tsx`).

## Real-Time Updates

WebSocket-based; no REST polling. Single socket per tab: `WebSocketProvider` in `src/providers/WebSocketProvider.tsx:1`. Components subscribe via `useWebSocketSubscription(ticketId)`. Core connection hook with reconnect, cursor resume, snapshot hydration, and heartbeat: `ui/src/hooks/useWebSocket.ts`. Full protocol v2 specification and event-type list: [hooks/CLAUDE.md](src/hooks/CLAUDE.md).

## Auth

`AuthGate` (`src/components/auth/`) wraps `<Routes>`; registers the apiFetch 401 handler then calls `refresh()`. `RequireAuth` redirects anonymous users to `/login`; also handles `must_change_password` → `/account?force=1`. `RequireAdmin` extends `RequireAuth` with a role check and is applied at route level for `/settings`. Public routes are `/login` and `/forbidden`; `/account` is inside `RequireAuth`. Selector hooks: `useIsAdmin()`, `useIsAuthed()`, `useMustChangePassword()` from `authStore.ts`.

## Source File Size Limit

Keep source files under 300 lines. Split files exceeding 300 lines into logical sub-files before committing (TypeScript/TSX source).

Enforced repo-wide by `make filesize` (CI); see [be/CLAUDE.md](../be/CLAUDE.md) for the `filesize.baseline` ratchet and split procedure.

## UI Component Standards

All UI elements must use the shared components from `src/components/ui/`. Do not use raw HTML elements (`<select>`, `<input>`, `<button>`, `<textarea>`) directly.

### Standard Components

| HTML Element | Use Instead | File |
|---|---|---|
| `<button>` | `Button` | `src/components/ui/Button.tsx` |
| `<input>` | `Input` | `src/components/ui/Input.tsx` |
| `<textarea>` | `Textarea` | `src/components/ui/Textarea.tsx` |
| `<select>` | `Dropdown` | `src/components/ui/Dropdown.tsx` |

### Dropdown Pattern

All dropdowns use the custom `Dropdown` component (not native `<select>`): button trigger with floating panel, checkmark on selected option, click-outside and Escape to close. For searchable dropdowns, use `TicketSearchDropdown`.

### Adding New UI Components

1. Place in `src/components/ui/`
2. Use `cn()` for class merging
3. Follow existing component patterns (forwardRef, consistent prop naming)
4. Match design tokens (CSS variables from `src/index.css`)

## Testing

Vitest uses a Node project for pure API/lib tests and a jsdom project for React/browser tests. Run: `make test-ui`. Must complete in ≤60 s.
Test helpers: `src/test/setup.node.ts` (env stubs), `src/test/setup.ts` (jsdom, React Testing Library, browser stubs), `src/test/utils.tsx` (`renderWithQuery`, wrappers).

## Styling

Tailwind CSS v4 with `@theme` CSS variables. Class-based dark mode via `.dark` on `<html>`. Three-state theme toggle (light/dark/system) in Header, persisted to `localStorage` key `nrf_theme`, with FOUC prevention inline script in `index.html`.

## Feature Index

Features are documented where their primary component or hook lives:

- Console chat (nrflo-rendered claude/codex/API-direct chat sessions; the new-chat picker is driven by `GET /console/catalog` — the same server-owned engine/model discovery the native TUI uses, with disabled engines carrying `disabled_reason`; the composer's '/' skill-suggestion dropdown is backed by `GET /console/skills`, fetched once per project; typing '/invoke ' switches it to the chat's tool list (`GET /console/chats/{sid}/tools`) and opens a schema-driven arg form that POSTs `/console/chats/{sid}/invoke`; the transcript hides `system_notice` rows entirely and collapses `task_notification` rows into a `<details>` summary card via `ChatTaskNotification`) → `src/components/console/`, `src/pages/ConsolePage.tsx`
- Interactive PTY sessions tray (docked, multi-tab, route-persistent) → `src/components/interactive/`, `src/stores/interactiveSessionsStore.ts`
- Workflow visualization, phase graph, findings, agent panels → [workflow/CLAUDE.md](src/components/workflow/CLAUDE.md)
- WebSocket protocol v2, event types, subscription patterns → [hooks/CLAUDE.md](src/hooks/CLAUDE.md)
- REST API modules and client conventions → [api/CLAUDE.md](src/api/CLAUDE.md)
- Unified model administration and mode-aware agent model/effort selectors → `src/components/settings/ModelsList.tsx`, `ModelForm.tsx`, and `src/hooks/useModels.ts`
- Settings → System Agents → Activity: live table of tier/fallback/refinery-fold/step-rotation runs from `GET /api/v1/system-agent-runs`, invalidated on `agent.handoff_digest`/`refinery.fold_failed` WS events, on `step.advanced` events with `data.rotated === true`, and on `delegate.*`/`consult.*` events (`useWSReducerAgents.ts`); delegate workers sharing a `delegation_id` are grouped into a collapsible row (tier, `n of fanout` workers, caller link, summed tokens/cost, status) via `lib/systemAgentRunGroups.ts`, row expand reuses `HandoffDigestSection` → `src/components/settings/SystemAgentRunsSection.tsx`, `SystemAgentRunRow.tsx`, `SystemAgentRunDelegationGroup.tsx`, `src/hooks/useSystemAgentRuns.ts`
- Stepwise progress strip on the phase-graph agent card ("N/M" + per-step pips, REST + live `step.advanced`) → `src/components/workflow/StepProgressStrip.tsx`, `src/hooks/useStepCursors.ts`
- Sessions tab on `/logs` (session list, downstream flow graph, tool distribution, cost rollup) → `src/components/sessions/`, `src/pages/LogsSessionsTab.tsx`
- Page routes, ticket tabs, project workflows layout → [pages/CLAUDE.md](src/pages/CLAUDE.md)
- Shared TypeScript types → [types/CLAUDE.md](src/types/CLAUDE.md)
