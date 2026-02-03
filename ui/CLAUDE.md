# Claude Code Instructions for nrworkflow UI

## Overview

This is the web UI for the nrworkflow ticket management system. It's a React + TypeScript application that communicates with the `nrworkflow serve` API.

## Key Files

| File/Directory | Purpose |
|----------------|---------|
| `src/api/client.ts` | API client with X-Project header support |
| `src/api/projects.ts` | Project API functions |
| `src/api/tickets.ts` | Ticket and workflow API functions |
| `src/types/workflow.ts` | Workflow types (WorkflowState, AgentHistoryEntry, etc.) |
| `src/types/` | TypeScript types matching Go models |
| `src/hooks/useTickets.ts` | TanStack Query hooks for data fetching |
| `src/stores/projectStore.ts` | Zustand store for project selection (loads from API) |
| `src/components/ui/` | Reusable UI components |
| `src/components/layout/` | Layout components (Header, Sidebar) |
| `src/components/tickets/` | Ticket-specific components |
| `src/components/workflow/PhaseTimeline.tsx` | Main workflow timeline with phase sorting |
| `src/components/workflow/PhaseCard.tsx` | Phase card with agent history and findings |
| `src/components/workflow/FindingsViewer.tsx` | Simple KEY: VALUE findings display |
| `src/components/workflow/WorkflowFindings.tsx` | Workflow-level findings grouped by agent |
| `src/components/workflow/AgentSessionCard.tsx` | Reusable agent session card component |
| `src/components/workflow/AgentMessagesPanel.tsx` | Agent sessions panel for ticket view |
| `src/components/workflow/StatsTooltip.tsx` | Message stats tooltip (tool/skill/text counts) |
| `src/components/ui/Tooltip.tsx` | Reusable tooltip with hover delay |
| `src/components/workflow/` | Workflow visualization components |
| `src/pages/TicketDetailPage.tsx` | Ticket detail with tabbed interface |
| `src/pages/AgentsPage.tsx` | Recent agents across all projects with live polling |
| `src/pages/SettingsPage.tsx` | Project management (create/update/delete) |
| `src/pages/` | Route page components |

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
- Vite proxies `/api` to the backend in development
- Projects are loaded from `/api/v1/projects` endpoint

### State Management

- **Server state**: TanStack Query (useQuery, useMutation)
- **Client state**: Zustand (project selection only)
- Query keys are in `src/hooks/useTickets.ts` - invalidate appropriately on mutations
- Projects are loaded from API on startup (see `projectStore.ts`)

### Component Structure

```
Layout
├── Header (project selector, search, navigation: Dashboard/Tickets/Agents, settings link)
├── Sidebar (navigation, status counts)
└── Outlet (page content via React Router)
```

### Pages

- **Dashboard** (`/`): Overview with ticket counts and status
- **Tickets** (`/tickets`): Ticket list with filtering
- **Ticket Detail** (`/tickets/:id`): Workflow timeline, description, details tabs
- **Agents** (`/agents`): Recent agents across all projects, grouped by project, 5s polling
- **Settings** (`/settings`): Project management

### Ticket Detail Page

The ticket detail page (`src/pages/TicketDetailPage.tsx`) uses a tabbed interface:
- **Workflow tab** (default): Shows phase timeline with agent history
- **Description tab**: Ticket description and dependencies
- **Details tab**: Metadata (priority, type, dates, etc.)

### Workflow Components

```
PhaseTimeline (src/components/workflow/PhaseTimeline.tsx)
├── Sorts phases by agent_history start times
├── PhaseCard (for each phase)
│   ├── Phase header (name, status, result badges)
│   ├── Active agents (if running)
│   └── Agent history cards (expandable with findings)
└── Legacy history section (if workflow.history exists)
```

### Findings Display

Findings use a simple KEY: VALUE format with minimal parsing:
- **First level only**: Each key is shown with its value directly
- **No truncation**: Full content is always displayed
- **JSON formatting**: Objects/arrays are pretty-printed with `JSON.stringify(value, null, 2)`
- **String values**: If a string is valid JSON, it's parsed and pretty-printed; otherwise shown as-is

Components: `SimpleFindingValue` in PhaseCard.tsx, WorkflowFindings.tsx, FindingsViewer.tsx

Key workflow types (`src/types/workflow.ts`):
- `WorkflowState`: Phase states, findings, active agents
- `WorkflowResponse`: API response with agent_history at top level
- `AgentHistoryEntry`: Agent execution record (agent_id, agent_type, model_id, phase, duration, result)
- `AgentSession`: Session record with `message_stats: Record<string, number>` for tool/skill/text counts
- `WorkflowFindings`: `Record<string, Record<string, unknown>>` (agent_type → field → value)

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
- API responses are typed - check `src/api/tickets.ts`

## Important Notes

- The backend (`nrworkflow serve`) must be running for the UI to work
- Default port is 6587 for API, 5173 for UI
- Projects are loaded from `/api/v1/projects` endpoint
- Multi-project support uses `X-Project` header
- Database is at `~/projects/2026/nrworkflow/nrworkflow.data` (global)
