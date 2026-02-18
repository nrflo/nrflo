# Claude Code Instructions for nrworkflow UI

## Overview

This is the web UI for the nrworkflow ticket management system. It's a React + TypeScript application that communicates with the `nrworkflow_server serve` API.

## Key Directories

| Directory | Purpose |
|-----------|---------|
| `src/api/` | API client modules with X-Project header support (see [api/CLAUDE.md](src/api/CLAUDE.md)) |
| `src/types/` | TypeScript types matching Go models (see [types/CLAUDE.md](src/types/CLAUDE.md)) |
| `src/hooks/` | TanStack Query hooks, WebSocket hook, utility hooks (see [hooks/CLAUDE.md](src/hooks/CLAUDE.md)) |
| `src/stores/` | Zustand store for project selection (`projectStore.ts`) |
| `src/lib/` | Utility functions (`cn`, `formatDate`, `statusColor`, etc.) |
| `src/components/workflow/` | Workflow visualization components (see [workflow/CLAUDE.md](src/components/workflow/CLAUDE.md)) |
| `src/components/ui/` | Reusable UI components: Badge, Button, Card, ConfirmDialog (variant-based), Dialog (modal with backdrop/ESC/click-outside), Dropdown (generic custom dropdown with click-outside/Escape/Check icon), Input, MarkdownEditor (CodeMirror 6), codemirror-theme.ts, ProjectSelect (uses Dropdown internally), Select, Spinner, Textarea, Toggle, Tooltip (portal-based positioning) |
| `src/components/layout/` | Layout components (Header, Sidebar, DailyStats) |
| `src/components/tickets/` | Ticket-specific components: IssueTypeIcon (Bug/Lightbulb/CheckSquare/Layers, sm/md sizes), TicketForm |
| `src/components/chains/` | Chain execution components (CreateChainDialog, ChainTicketSelector, AppendToChainDialog) |
| `src/components/git/` | Git commit detail dialog and diff viewer components |
| `src/pages/DocumentationPage.tsx` | Agent documentation page — fetches and renders markdown from `/api/v1/docs/agent-manual` via `react-markdown` |
| `src/pages/LogsPage.tsx` | Server logs page — displays BE/FE log files with sub-tab switching, fetched via `useLogs` hook with 5s polling |
| `src/pages/` | Route page components (see [pages/CLAUDE.md](src/pages/CLAUDE.md)) |
| `src/assets/` | Static assets |
| `src/test/` | Test infrastructure (`setup.ts`, `utils.tsx`) |

## Module Documentation

Detailed documentation for each major module is in its own CLAUDE.md:

| Module | Documentation | Key Content |
|--------|--------------|-------------|
| `src/components/workflow/` | [workflow/CLAUDE.md](src/components/workflow/CLAUDE.md) | Component tree, PhaseGraph features, findings display, agent panels, workflow definition management |
| `src/pages/` | [pages/CLAUDE.md](src/pages/CLAUDE.md) | Routes, ticket detail tabs, ProjectWorkflowsPage layout, real-time patterns |
| `src/hooks/` | [hooks/CLAUDE.md](src/hooks/CLAUDE.md) | WebSocket protocol, subscription patterns, event types, state management |
| `src/api/` | [api/CLAUDE.md](src/api/CLAUDE.md) | REST endpoint listing, API client architecture, live tracking, message format |
| `src/types/` | [types/CLAUDE.md](src/types/CLAUDE.md) | Key ticket/workflow/chain types, type safety notes |

## Source File Size Limit

Keep source files under 300 lines. If a newly created or modified file exceeds 300 lines, refactor it by splitting into logical sub-files before committing. This applies to all TypeScript/TSX source files.

## Development Commands

```bash
npm run dev        # Start dev server (port 5173)
npm run build      # Production build (includes tsc)
npm run lint       # ESLint
npx tsc --noEmit   # TypeScript check only
```

## Architecture Patterns

### API Communication

- All API calls go through `src/api/client.ts`
- Project selection is managed via `X-Project` header
- TanStack Query handles caching and refetching
- Vite proxies `/api` to the backend in development (including WebSocket via `ws: true`)
- Projects are loaded from `/api/v1/projects` endpoint

### State Management

- **Server state**: TanStack Query (useQuery, useMutation)
- **Client state**: Zustand (project selection only)
- Query keys are in `src/hooks/useTickets.ts` — invalidate appropriately on mutations
- Projects are loaded from API on startup (see `projectStore.ts`)

### Real-Time Updates (Protocol v2)

WebSocket-based, no REST polling. See [hooks/CLAUDE.md](src/hooks/CLAUDE.md) for full protocol, event types, and subscription patterns.

- **Single socket per tab**: `WebSocketProvider` in `src/providers/WebSocketProvider.tsx` owns the sole WebSocket connection. Wrapped at `App.tsx` level.
- **Consumer hook**: Components use `useWebSocketSubscription(ticketId)` to subscribe/unsubscribe. Project-wide subscription is automatic.
- **No polling**: All `refetchInterval` has been removed. Updates arrive exclusively via WebSocket events.
- **Protocol v2**: Events include `sequence` and `protocol_version` fields. Subscribe messages include optional `since_seq` for cursor-based replay on reconnect.
- **Reducer dispatch**: Events are routed through `useWSReducer.ts` which tracks per-subscription seq for idempotency and cursor resume.
- **Snapshot support**: Server can send `snapshot.begin/chunk/end` control events for full state hydration. Live events arriving during snapshot are buffered and replayed after.
- **Heartbeat liveness**: If no message received in 60s, triggers reconnect.
- **Cursor resume**: On reconnect, subscribe includes `since_seq` from last applied seq. Server replays missed events or sends snapshot if cursor is too old.
- **sessionStorage persistence**: Last applied seq per subscription persisted across tab refresh.

### Component Structure

```
Layout
├── Header (project selector, search, navigation: Dashboard/Tickets/Workflows/Git Status/Documentation/Logs, daily stats, settings link)
├── Sidebar (navigation, status counts)
└── Outlet (page content via React Router)
```

### Styling

- Tailwind CSS v4 (uses `@theme` for CSS variables)
- Dark mode support via `prefers-color-scheme`
- Custom utility `cn()` for conditional class merging

## Common Tasks

### Adding a New API Endpoint

1. Add types in `src/types/`
2. Add API function in `src/api/tickets.ts` or new file
3. Add query hook in `src/hooks/` if needed
4. Use in components with the hook

### Adding a New Page

1. Create page component in `src/pages/`
2. Add route in `src/App.tsx`
3. Add navigation link in `src/components/layout/Sidebar.tsx` if needed

### Adding a New UI Component

1. Create in `src/components/ui/`
2. Use `cn()` for class merging
3. Use CSS variables for theming (see `src/index.css`)

## Type Safety

- Types in `src/types/` must match the Go API models
- Use `z.infer<typeof schema>` for form types (see TicketForm)
- API responses are typed — check `src/api/tickets.ts`

## Testing

### Infrastructure

- **Framework:** Vitest + jsdom + React Testing Library
- **Setup:** `src/test/setup.ts` — auto-cleanup after each test, `window.location` mock, `VITE_API_URL` env stub
- **Utilities:** `src/test/utils.tsx` — `renderWithQuery()`, `createTestQueryClient()`, `createWrapper()`
- **Matchers:** `@testing-library/jest-dom/vitest` (toBeInTheDocument, toHaveTextContent, etc.)
- **User events:** `@testing-library/user-event` — always use `const user = userEvent.setup()`

### Commands

```bash
npx vitest run                    # all tests
npx vitest run src/components/    # directory
npx vitest run path/to/file.test.tsx  # single file
npx vitest run --reporter=verbose # with timing per test
```

### File Organization

- Co-located with source: `Component.test.tsx` next to `Component.tsx`
- Variant tests use descriptive suffixes: `Component.feature.test.tsx`
- Some directories use `__tests__/` subdirectories
- 300-line max per test file — split by feature area if exceeded

### Patterns

- **Factory functions** with override pattern: `makeAgent({status: "failed"})`
- **QueryClient isolation:** fresh `createTestQueryClient()` per test (retry: false, gcTime: 0)
- **Mock at boundaries:** mock hooks/API modules (`vi.mock('@/hooks/useTickets')`), not internals
- **Queries:** prefer `getByRole` / `getByText` over `getByTestId`
- **Async:** use `findBy*` queries (auto-wait) over `waitFor` with `getBy*`

### What NOT to Test

- Third-party library internals (React Router, TanStack Query, Zustand)
- Implementation details (internal state, CSS class names)
- Trivial renders without meaningful assertions
- Simple pass-through props or basic HTML structure
- Every prop combination — use representative samples

## Important Notes

- The backend (`nrworkflow_server serve`) must be running for the UI to work
- Default port is 6587 for API, 5173 for UI
- Projects are loaded from `/api/v1/projects` endpoint
- Multi-project support uses `X-Project` header
- Database is at `~/projects/2026/nrworkflow/nrworkflow.data` (global)

## Starting Servers

```bash
# Quick start - restart both servers (kills existing, rebuilds, starts in background)
./restart.sh

# Or start manually:
nrworkflow_server serve       # Start API server (port 6587)
cd ui && npm run dev          # Start UI dev server (port 5173)

# Stop servers
./stop.sh
```

| Script | Purpose |
|--------|---------|
| `restart.sh` | Kill existing servers, rebuild both binaries, start both in background |
| `stop.sh` | Stop running BE + UI servers |
| `rebuild-cli.sh` | Rebuild and re-symlink CLI binary without restarting server |
| `ui/start-server.sh` | Start both servers in foreground (uses `nrworkflow_server serve`) |

Logs are written to `/tmp/nrworkflow/logs/be.log` and `/tmp/nrworkflow/logs/fe.log` when using `restart.sh`.

## Web UI Features

- Dashboard with ticket counts and status overview
- Ticket list with filtering and search
- Ticket detail view with workflow timeline
- Live tracking with real-time agent stdout messages
- Findings display with workflow-level and agent findings separated
- Create/edit/close tickets
- Multi-project support via project selector
- Settings page for project management (create/update/delete, Toggle for git worktrees, Toggle for docker isolation)
- Documentation page with agent manual (rendered markdown from API)
- Logs page with BE/FE sub-tabs, 5s polling via `useLogs` hook (`GET /api/v1/logs?type={be|fe}`)
