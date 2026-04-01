# Claude Code Instructions for nrworkflow UI

## Overview

This is the web UI for the nrworkflow ticket management system. It's a React + TypeScript application that communicates with the `nrworkflow_server serve` API.

## Key Directories

| Directory | Purpose |
|-----------|---------|
| `src/api/` | API client modules with X-Project header support (see [api/CLAUDE.md](src/api/CLAUDE.md)) |
| `src/types/` | TypeScript types matching Go models (see [types/CLAUDE.md](src/types/CLAUDE.md)) |
| `src/hooks/` | TanStack Query hooks, WebSocket hook, utility hooks (see [hooks/CLAUDE.md](src/hooks/CLAUDE.md)) |
| `src/stores/` | Zustand stores: project selection (`projectStore.ts`), theme preference (`themeStore.ts`) |
| `src/lib/` | Utility functions (`cn`, `formatDate`, `statusColor`, etc.) |
| `src/components/workflow/` | Workflow visualization components (see [workflow/CLAUDE.md](src/components/workflow/CLAUDE.md)) |
| `src/components/ui/` | Reusable UI components: Badge, Button, Card, ConfirmDialog (variant-based), Dialog (modal with backdrop/ESC/click-outside), Dropdown (generic custom dropdown with click-outside/Escape/Check icon, supports flat and grouped options via `DropdownOptionGroup`), Input, MarkdownEditor (CodeMirror 6), codemirror-theme.ts, ProjectSelect (uses Dropdown internally), RenderedMarkdown (react-markdown with Tailwind component overrides), Spinner, StatusCell (lucide-react icon + status text label with animate-pulse for running/in_progress), PriorityIcon (directional arrow icons for priority 1-4 with label), ResultIcon (check/x/minus icons for pass/fail/skip), Table (composable: Table, TableHeader, TableBody, TableRow, TableHead, TableCell with hover:bg-muted/50 on rows), Textarea, Toggle, Tooltip (portal-based positioning) |
| `src/components/layout/` | Layout components (Header, Sidebar, DailyStats, RunningAgentsIndicator) |
| `src/components/tickets/` | Ticket-specific components: IssueTypeIcon (Bug/Lightbulb/CheckSquare/Layers, sm/md sizes), TicketForm |
| `src/components/chains/` | Chain execution components (CreateChainDialog, ChainTicketSelector, ChainOrderList, AppendToChainDialog) |
| `src/components/git/` | Git commit detail dialog and diff viewer components |
| `src/components/settings/` | Settings page sections (tab layout): GlobalSettingsSection (low consumption mode toggle, session retention limit number input with tooltip, stall start/running timeout inputs), ProjectsSection (project CRUD), SystemAgentsSection (system agent definitions CRUD), DefaultTemplatesSection (default template CRUD with readonly/built-in support), DefaultTemplateForm (template form with MarkdownEditor), CLIModelsSection (CLI model CRUD with readonly/built-in support), CLIModelForm (CLI model form with cli_type warnings), LogsSection (BE/FE log viewer with sub-tabs, extracted from former LogsPage) |
| `src/pages/DocumentationPage.tsx` | Agent documentation page — fetches and renders markdown from `/api/v1/docs/agent-manual` via `react-markdown` |
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
npm run dev        # Start dev server (port 5175)
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
- **Client state**: Zustand (project selection via `projectStore.ts`, theme preference via `themeStore.ts`)
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
├── Header (project selector, search, navigation: Dashboard/Tickets/Workflows/Git Status/Documentation, daily stats, theme toggle, settings link)
├── Sidebar (navigation, status counts)
└── Outlet (page content via React Router)
```

### Styling

- Tailwind CSS v4 (uses `@theme` for CSS variables)
- Class-based dark mode via `@custom-variant dark (&:where(.dark, .dark *))` — `.dark` class on `<html>` controls all `dark:` utilities and CSS variable overrides in `.dark {}` selector
- Three-state theme toggle (light/dark/system) in Header, persisted to `localStorage` key `nrwf_theme`, with FOUC prevention inline script in `index.html`
- Custom utility `cn()` for conditional class merging

## UI Component Standards

All UI elements must use the shared components from `src/components/ui/`. Do not use raw HTML elements (`<select>`, `<input>`, `<button>`, `<textarea>`) directly — use the corresponding wrapper components instead.

### Standard Components

| HTML Element | Use Instead | File |
|---|---|---|
| `<button>` | `Button` | `src/components/ui/Button.tsx` |
| `<input>` | `Input` | `src/components/ui/Input.tsx` |
| `<textarea>` | `Textarea` | `src/components/ui/Textarea.tsx` |
| `<select>` | `Dropdown` | `src/components/ui/Dropdown.tsx` |

### Dropdown Pattern

All dropdowns use the custom `Dropdown` component (not native `<select>`). This renders a button trigger with a floating panel, matching the ProjectSelect style:
- Button with chevron icon, border, and hover state
- Floating panel with shadow and border
- Checkmark on selected option
- Click-outside and Escape key to close

For searchable dropdowns, use `TicketSearchDropdown` which follows the same panel styling.

### Adding New UI Components

When creating a new reusable UI component:
1. Place it in `src/components/ui/`
2. Use `cn()` for class merging
3. Follow existing component patterns (forwardRef, consistent prop naming)
4. Match the project's design tokens (CSS variables from `src/index.css`)

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
./scripts/test.sh                 # run with 15s wall-time check (mirrors BE constraint)
```

### Performance Constraint

The full test suite (`vitest run`) must complete in **≤15 seconds wall time**. Enforced by `./scripts/test.sh`.

**Never introduce:**
- `setTimeout` in test bodies or mock implementations — use a never-resolving promise `new Promise(() => {})` to keep a mutation in-flight for `isPending` tests
- `vi.advanceTimersByTime()` with real timer dependencies — use `vi.useFakeTimers()` instead
- Arbitrary delays or polling loops with sleeps

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
- Default port is 6587 (single-process serves API + embedded UI); port 5175 is used only for `npm run dev` hot-reload during development
- Projects are loaded from `/api/v1/projects` endpoint
- Multi-project support uses `X-Project` header
- Database is at `~/projects/2026/nrworkflow/nrworkflow.data` (global)

## Starting Servers

The server binary embeds the UI and serves everything on a single port (default 6587).

```bash
# Quick start - rebuild and restart (kills existing, rebuilds, starts in background)
./restart.sh

# Or start manually:
nrworkflow_server serve       # Start server (port 6587, serves API + embedded UI)

# For UI hot-reload development (optional):
cd ui && npm run dev          # Start Vite dev server (port 5175, proxies API to 6587)

# Stop server
./stop.sh
```

| Script | Purpose |
|--------|---------|
| `restart.sh` | Kill server, rebuild binary (including UI), start in background |
| `stop.sh` | Stop running server |
| `rebuild-cli.sh` | Rebuild and re-symlink CLI binary without restarting server |
| `ui/start-server.sh` | Start server in foreground (uses `nrworkflow_server serve`) |

Logs are written to `/tmp/nrworkflow/logs/be.log` when using `restart.sh`.

## Web UI Features

- Dashboard with ticket counts and status overview
- Ticket list with filtering and search
- Ticket detail view with workflow timeline
- Live tracking with real-time agent stdout messages
- Findings display with workflow-level and agent findings separated
- Create/edit/close tickets
- Multi-project support via project selector
- Settings page with tabbed layout (General, Projects, System Agents, Default Templates, CLI Models, Logs): project management (create/update/delete, Toggle for git worktrees), system agent definitions CRUD (global agents like conflict-resolver), default template CRUD (readonly built-in templates + user-created templates with MarkdownEditor), CLI model CRUD (readonly built-in models + user-created models with cli_type badges and warnings)
- Documentation page with agent manual (rendered markdown from API)
- Running agents indicator: `RunningAgentsIndicator` in header shows animated spinner with count badge when agents are running across any project. Hover popover lists agents grouped by project with clickable links. Data fetched via `useRunningAgents` hook (`GET /api/v1/agents/running`, not project-scoped). WS event `global.running_agents` invalidates the query for real-time updates. Types in `src/types/agents.ts`, API in `src/api/agents.ts`.
- Conflict resolver banner: `ConflictResolverBanner` component displays merge conflict resolution status in workflow detail view. Shows amber spinner while resolver is running, green note on pass, orange warning on fail. Reacts to `merge.conflict_resolving`, `merge.conflict_resolved`, `merge.conflict_failed` WS events. Suppresses the "Completed" banner while resolver is actively running. Clickable to open resolver session in AgentLogPanel.
- Interactive agent control: "Take Control" button kills a running Claude agent and opens an xterm.js terminal dialog connected to the PTY WebSocket (`/api/v1/pty/{sessionId}`). Components: `XTerminal` (lazy-loaded xterm.js + WebSocket relay), `AgentTerminalDialog` (Dialog wrapper). Hooks: `useTakeControl`, `useExitInteractive` (+ project-scoped variants). WS event: `agent.take_control`. Agent status `user_interactive` shows blue glow in AgentCard and blue "User controlling" badge in AgentLogDetail.
- Interactive start & plan mode: `RunWorkflowDialog` and `RunWorkflowForm` support "Start Interactive" and "Plan Before Execution" checkboxes (mutually exclusive). Interactive mode sends `interactive: true` in the run request; plan mode sends `plan_mode: true`. Both return a `session_id` in `RunWorkflowResponse` that triggers `AgentTerminalDialog` via `onInteractiveStart` callback. "Start Interactive" is only available when L0 has exactly 1 Claude-based agent. "Plan Before Execution" hides the instructions textarea.
