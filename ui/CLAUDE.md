# Claude Code Instructions for nrworkflow UI

## Overview

This is the web UI for the nrworkflow ticket management system. It's a React + TypeScript application that communicates with the `nrworkflow serve` API.

## Key Files

| File/Directory | Purpose |
|----------------|---------|
| `src/api/client.ts` | API client with X-Project header support |
| `src/api/projects.ts` | Project API functions |
| `src/api/tickets.ts` | Ticket and workflow API functions |
| `src/api/workflows.ts` | Workflow definition and orchestration API functions |
| `src/api/projectWorkflows.ts` | Project-scoped workflow API functions (run/stop/get/restart) |
| `src/api/agentDefs.ts` | Agent definition API client |
| `src/types/workflow.ts` | Workflow types (WorkflowState, AgentHistoryEntry, etc.) |
| `src/types/ticket.ts` | Ticket types (Ticket, Dependency, Status, etc.) |
| `src/types/` | TypeScript types matching Go models |
| `src/hooks/useTickets.ts` | TanStack Query hooks for data fetching |
| `src/hooks/useProjects.ts` | TanStack Query hook for projects |
| `src/hooks/useWebSocket.ts` | WebSocket hook for real-time updates |
| `src/hooks/useElapsedTime.ts` | Elapsed time hooks (useElapsedTime, useTickingClock) |
| `src/stores/projectStore.ts` | Zustand store for project selection (loads from API) |
| `src/lib/utils.ts` | Utility functions (cn, formatDate, statusColor, etc.) |
| `src/components/ui/MarkdownEditor.tsx` | CodeMirror 6 markdown editor (used in AgentDefForm/Card) |
| `src/components/ui/codemirror-theme.ts` | CodeMirror theme using CSS variables (auto dark/light) |
| `src/components/ui/Dialog.tsx` | Modal dialog with backdrop, ESC key, click-outside-to-close |
| `src/components/ui/` | Reusable UI components (Badge, Button, Card, Input, ProjectSelect, Select, Spinner, Textarea, Toggle) |
| `src/components/layout/` | Layout components (Header, Sidebar) |
| `src/components/tickets/` | Ticket-specific components |
| `src/components/workflow/PhaseTimeline.tsx` | Main workflow timeline wrapper (uses PhaseGraph) |
| `src/components/workflow/PhaseGraph/` | Vertical graph visualization of workflow phases |
| `src/components/workflow/PhaseCard.tsx` | Phase card with agent history and findings |
| `src/components/workflow/FindingsViewer.tsx` | Simple KEY: VALUE findings display |
| `src/components/workflow/WorkflowFindings.tsx` | Workflow-level findings grouped by agent |
| `src/components/workflow/ActiveAgentsPanel.tsx` | Active agents display panel |
| `src/components/workflow/AgentDefForm.tsx` | Agent definition create/edit form |
| `src/components/workflow/AgentDefCard.tsx` | Agent definition card with edit/delete |
| `src/components/workflow/AgentDefsSection.tsx` | Agent definitions list within a workflow |
| `src/components/workflow/PhaseListEditor.tsx` | Layer-aware phase list editor with skip_for tags and fan-in validation |
| `src/components/workflow/WorkflowDefForm.tsx` | Workflow definition create/edit form |
| `src/components/workflow/RunWorkflowDialog.tsx` | Dialog for starting orchestrated ticket workflow runs |
| `src/components/workflow/RunProjectWorkflowDialog.tsx` | Dialog for starting project-scoped workflow runs (filters to scope_type=project) |
| `src/components/workflow/AgentSessionCard.tsx` | Reusable agent session card component |
| `src/components/workflow/AgentMessagesPanel.tsx` | Agent sessions panel for ticket view |
| `src/components/workflow/AgentLogPanel.tsx` | Collapsible right-side panel: overview of running agents or single-agent detail view |
| `src/components/workflow/AgentLogDetail.tsx` | Single-agent detail view with message table (timestamp, tool, message columns), used by AgentLogPanel |
| `src/components/workflow/LogMessage.tsx` | Log message component with tool name color highlighting. Exports parseToolName and ToolBadge for table rendering |
| `src/components/workflow/` | Workflow visualization components |
| `src/pages/Dashboard.tsx` | Dashboard overview page |
| `src/pages/TicketListPage.tsx` | Ticket list with filtering |
| `src/pages/CreateTicketPage.tsx` | Create new ticket form page |
| `src/pages/EditTicketPage.tsx` | Edit existing ticket form page |
| `src/pages/TicketDetailPage.tsx` | Ticket detail with tabbed interface |
| `src/pages/WorkflowsPage.tsx` | Workflow definitions CRUD and agent definition management |
| `src/pages/ProjectWorkflowsPage.tsx` | Project-scoped workflow execution page (run/stop/view state) |
| `src/pages/SettingsPage.tsx` | Project management (create/update/delete) |
| `src/pages/` | Route page components |

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
- Query keys are in `src/hooks/useTickets.ts` - invalidate appropriately on mutations
- Projects are loaded from API on startup (see `projectStore.ts`)

### Real-Time Updates

The UI uses WebSocket exclusively for real-time updates (no REST polling):

- **WebSocket hook**: `src/hooks/useWebSocket.ts`
- **Connection**: Connects to `ws://host/api/v1/ws`
- **Subscriptions**: Per-ticket or project-wide
- **Auto-reconnect**: Exponential backoff (3s, 6s, 9s...), max 5 attempts
- **Query invalidation**: Events automatically invalidate relevant TanStack Query caches
- **No polling**: All updates (ticket state, agent sessions, messages) arrive via WebSocket events
- **messages.updated**: The spawner broadcasts `messages.updated` every ~2s while agents run, triggering agent session and recent agents cache invalidation

**Important:** Subscriptions must be gated on `projectsLoaded` to avoid subscribing with the wrong project ID. The hook stores only `ticketId`s internally and resolves the project ID fresh via `getProject()` each time it sends a message.

Usage:
```typescript
import { useProjectStore } from '@/stores/projectStore'

const { subscribe, unsubscribe } = useWebSocket()
const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
const currentProject = useProjectStore((s) => s.currentProject)

// Subscribe to specific ticket (gated on project readiness)
useEffect(() => {
  if (ticketId && projectsLoaded) {
    subscribe(ticketId)
    return () => unsubscribe(ticketId)
  }
}, [ticketId, projectsLoaded, currentProject, subscribe, unsubscribe])

// No refetchInterval needed - WebSocket handles all updates
useTicket(id)
useAgentSessions(ticketId, undefined, { enabled: !!ticketId })
```

Event types: `agent.started`, `agent.completed`, `phase.started`, `phase.completed`, `findings.updated`, `messages.updated`, `workflow.updated`, `ticket.updated`

**Project-wide subscription:** Layout.tsx subscribes to all project events (empty ticketId) so that Sidebar status counts, ticket lists, and Dashboard receive real-time updates (e.g., `ticket.updated` events).

**Project-scoped workflow events:** Events from project-scoped workflows have empty `ticket_id` and non-empty `project_id`. The WebSocket hook detects this and invalidates `projectWorkflowKeys` instead of `ticketKeys`. ProjectWorkflowsPage subscribes with empty ticketId to receive these events.

### Component Structure

```
Layout
├── Header (project selector, search, navigation: Dashboard/Tickets/Workflows, settings link)
├── Sidebar (navigation, status counts)
└── Outlet (page content via React Router)
```

### Pages

- **Dashboard** (`/`): Overview with ticket counts and status
- **Tickets** (`/tickets`): Ticket list with filtering
- **Create Ticket** (`/tickets/new`): Create new ticket form
- **Edit Ticket** (`/tickets/:id/edit`): Edit existing ticket form
- **Ticket Detail** (`/tickets/:id`): Workflow timeline, description, details tabs
- **Workflows** (`/workflows`): Workflow definitions and agent definitions CRUD
- **Project Workflows** (`/project-workflows`): Run and monitor project-scoped workflows
- **Settings** (`/settings`): Project management

### Ticket Detail Page

The ticket detail page (`src/pages/TicketDetailPage.tsx`) uses a tabbed interface:
- **Workflow tab** (default): Shows phase timeline with agent history
- **Description tab**: Ticket description and dependencies
- **Details tab**: Metadata (priority, type, dates, etc.)

**Real-time updates**: The page uses WebSocket exclusively for real-time updates. The page subscribes to the current ticket on mount via `useWebSocket()` hook. No REST polling is used.

**Agent Log Panel**: The right-side panel (`AgentLogPanel`) has two modes:
- **Overview mode** (default): Shows running agents with compact messages. Visible when agents are running.
- **Detail mode**: Shows a single agent's messages in a table (timestamp|tool|message columns). Activated when clicking an agent in the PhaseGraph or in the overview. Includes a back button to return to overview. Raw output is stored in DB for debug only, not displayed in UI.
The panel also shows when a completed agent is selected from PhaseGraph (even after all agents finish). Uses `AgentLogDetail` for the detail view. The panel collapses to a thin bar (w-10) with vertical label.

### Workflow Components

```
PhaseTimeline (src/components/workflow/PhaseTimeline.tsx)
├── Workflow metadata badges (version, category, current phase)
├── PhaseGraph (src/components/workflow/PhaseGraph/)
│   ├── PhaseGraph.tsx - Main container using React Flow (@xyflow/react)
│   ├── AgentFlowNode.tsx - Custom React Flow node for agents (clickable, opens modal)
│   ├── layout.ts - Auto-layout helper for vertical positioning
│   ├── PhaseFlowNode.tsx - Custom React Flow node for phases
│   ├── PhaseNode.tsx - Standalone phase node
│   ├── AgentCard.tsx - Running agent card with elapsed time
│   ├── HistoryAgentCard.tsx - Completed agent card for phase history
│   └── types.ts - TypeScript types for graph components
└── WorkflowFindings (all workflow findings at bottom)
    ├── WorkflowLevelFindings (findings['workflow'] - blue styling)
    └── AgentFindings (other keys, empty findings filtered out - purple styling)
```

**PhaseGraph Features:**
- Uses React Flow library (@xyflow/react) for graph rendering
- Vertical (top-to-bottom) flow with automatically routed edges
- **Shows ALL phases from workflow config upfront** (not just started phases)
  - Pending phases: dashed border, clock icon, "pending" label
  - Skipped phases: dashed border, skip icon, faded appearance
  - Running phases: yellow border with glow animation
  - Completed phases: green (pass) or red (fail) border
- Phases ordered by `phase_order` from backend (preserves config.json order)
- Edges connect all phases with colors based on source result (gray default, green pass, red fail, yellow running)
- Animated edges for in_progress phases
- Running agents display with model name and elapsed time
- Completed agents show model, result badge, and duration
- Clicking any agent node shows it in the right-side AgentLogPanel (detail view with message table)
- Agent detail messages sorted with latest first (newest at top)
- Detail view shows live updates when agent is running (session lookup from props, not captured at click time)
- Session lookup for history entries uses fallback matching (exact model_id match first, then agent_type+phase only)
- Agent sessions always fetched for ticket (enables history messages), refreshed via WebSocket messages.updated events
- Custom node uses `nopan nodrag` classes and `pointerEvents: 'all'` for click handling in ReactFlow

### Findings Display

Findings use a simple KEY: VALUE format with minimal parsing:
- **First level only**: Each key is shown with its value directly
- **No truncation**: Full content is always displayed
- **JSON formatting**: Objects/arrays are pretty-printed with `JSON.stringify(value, null, 2)`
- **String values**: If a string is valid JSON, it's parsed and pretty-printed; otherwise shown as-is

**Workflow vs Agent Findings:**
- Findings under the `'workflow'` key are displayed separately at the top with blue styling (Workflow icon)
- Agent findings (all other keys) are displayed below with purple styling (Cpu icon)
- Empty agent findings (`{}`) are filtered out and not displayed
- `WorkflowFindings` component handles this separation and filtering automatically

Components: `SimpleFindingValue` in PhaseCard.tsx, WorkflowFindings.tsx, FindingsViewer.tsx

Key ticket types (`src/types/ticket.ts`):
- `Ticket`: Base ticket with `parent_ticket_id?: string | null`
- `PendingTicket`: Extends `Ticket` with `is_blocked` and `blocked_by` fields
- `TicketListResponse`: `{ tickets: PendingTicket[] }` — list endpoint returns PendingTicket
- `SearchResponse`: `{ tickets: PendingTicket[], query: string }` — search also returns PendingTicket
- `StatusResponse`: Includes `counts.blocked` for sidebar badge

Key workflow types (`src/types/workflow.ts`):
- `ScopeType`: `'ticket' | 'project'` — workflow scope type
- `WorkflowState`: Phase states, phase_order, scope_type, findings, active_agents map (constructed server-side from `workflow_instances` + `agent_sessions` tables)
- `WorkflowResponse`: API response with agent_history at top level (ticket-scoped)
- `ProjectWorkflowResponse`: API response for project-scoped workflows (project_id instead of ticket_id)
- `AgentHistoryEntry`: Agent execution record (agent_id, agent_type, model_id, phase, duration, result, context_left)
- `AgentSession`: Session record with fields from `agent_sessions` table: `workflow_instance_id`, `result`, `result_reason`, `pid`, `findings`, `started_at`, `ended_at`, `last_messages`, `message_count`, `raw_output_size`, `context_left`
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

## Starting Servers

```bash
# Quick start - restart both servers (kills existing, rebuilds, starts in background)
./restart.sh

# Or start manually:
nrworkflow serve              # Start API server (port 6587)
cd ui && npm run dev          # Start UI dev server (port 5173)

# Stop servers
./stop.sh
```

| Script | Purpose |
|--------|---------|
| `restart.sh` | Kill existing servers, rebuild BE + UI, start both in background |
| `stop.sh` | Stop running BE + UI servers |
| `ui/start-server.sh` | Start both servers in foreground (interactive mode) |

Logs are written to `logs/backend.log` and `logs/ui.log` when using `restart.sh`.

## Web UI Features

- Dashboard with ticket counts and status overview
- Ticket list with filtering and search
- Ticket detail view with workflow timeline
- Live tracking with real-time agent stdout messages
- Findings display with workflow-level and agent findings separated
- Create/edit/close tickets
- Multi-project support via project selector
- Settings page for project management (create/update/delete)

## WebSocket Protocol

The WebSocket endpoint (`/api/v1/ws`) provides real-time updates for agent state changes. Clients can subscribe to specific tickets or all tickets in a project.

**Connect:**
```javascript
const ws = new WebSocket('ws://localhost:6587/api/v1/ws')
```

**Subscribe to a ticket:**
```json
{
  "action": "subscribe",
  "project_id": "myproject",
  "ticket_id": "TICKET-123"
}
```

**Subscribe to all tickets in project:**
```json
{
  "action": "subscribe",
  "project_id": "myproject",
  "ticket_id": ""
}
```

**Unsubscribe:**
```json
{
  "action": "unsubscribe",
  "project_id": "myproject",
  "ticket_id": "TICKET-123"
}
```

### Event Types

| Event | Data Fields | Description |
|-------|-------------|-------------|
| `agent.started` | agent_id, agent_type, model_id, session_id, phase | Agent spawned |
| `agent.completed` | agent_id, result, result_reason, model_id | Agent finished |
| `agent.continued` | agent_id, model_id | Agent relaunched for continuation |
| `phase.started` | phase | Phase began |
| `phase.completed` | phase, result | Phase finished |
| `findings.updated` | agent_type, key, action | Findings changed |
| `messages.updated` | session_id, agent_type, model_id | Agent messages changed (~2s interval) |
| `workflow.updated` | action (init, set) | Workflow state changed |
| `workflow_def.created` | workflow_id | Workflow definition created |
| `workflow_def.updated` | workflow_id | Workflow definition updated |
| `workflow_def.deleted` | workflow_id | Workflow definition deleted |
| `agent_def.created` | workflow_id, agent_id | Agent definition created |
| `agent_def.updated` | workflow_id, agent_id | Agent definition updated |
| `agent_def.deleted` | workflow_id, agent_id | Agent definition deleted |
| `orchestration.started` | instance_id, category | Orchestrated workflow run started |
| `orchestration.completed` | instance_id | Orchestrated workflow run completed |
| `orchestration.failed` | instance_id, reason | Orchestrated workflow run failed |
| `ticket.updated` | | Ticket state changed (status, priority, etc.) |

All events include: `type`, `project_id`, `ticket_id`, `workflow`, `timestamp`

### Event Sources

- Events from **agent CLI commands** (workflow.init, phase.start/complete, findings.*, agent.complete/fail/kill) are received via the socket handler and broadcast via the in-process WebSocket hub.
- Events from **spawner/orchestrator** (agent.started, messages.updated, agent.completed, phase.started, phase.completed) are broadcast directly via the in-process WebSocket hub.

## REST API Endpoints

```bash
# Projects
GET /api/v1/projects
GET /api/v1/projects/:id
POST /api/v1/projects
PATCH /api/v1/projects/:id
DELETE /api/v1/projects/:id

# Tickets (require X-Project header or ?project= param)
GET /api/v1/tickets
GET /api/v1/tickets/:id
POST /api/v1/tickets
PATCH /api/v1/tickets/:id
DELETE /api/v1/tickets/:id
POST /api/v1/tickets/:id/close

# Dependencies
GET /api/v1/tickets/:id/dependencies

# Workflow state
GET /api/v1/tickets/:id/workflow
PATCH /api/v1/tickets/:id/workflow

# Workflow orchestration
POST /api/v1/tickets/:id/workflow/run      # Start orchestrated run
POST /api/v1/tickets/:id/workflow/stop     # Stop running orchestration
POST /api/v1/tickets/:id/workflow/restart  # Restart a running agent (save context, relaunch)

# Project-scoped workflows
GET  /api/v1/projects/:id/workflow         # Get project workflow state
POST /api/v1/projects/:id/workflow/run     # Start project-scoped workflow run
POST /api/v1/projects/:id/workflow/stop    # Stop running project workflow
POST /api/v1/projects/:id/workflow/restart # Restart agent in project workflow

# Workflow definitions (require X-Project header)
GET    /api/v1/workflows
POST   /api/v1/workflows
GET    /api/v1/workflows/:id
PATCH  /api/v1/workflows/:id
DELETE /api/v1/workflows/:id

# Agent definitions (nested under workflows, require X-Project header)
GET    /api/v1/workflows/:wid/agents
POST   /api/v1/workflows/:wid/agents
GET    /api/v1/workflows/:wid/agents/:id
PATCH  /api/v1/workflows/:wid/agents/:id
DELETE /api/v1/workflows/:wid/agents/:id

# Agent sessions
GET /api/v1/tickets/:id/agents
GET /api/v1/tickets/:id/agents?phase=investigation

# Recent agents (cross-project)
GET /api/v1/agents/recent
GET /api/v1/agents/recent?limit=10

# Session messages (paginated)
GET /api/v1/sessions/:id/messages
GET /api/v1/sessions/:id/messages?limit=100&offset=0

# Session raw output (raw stdout/stderr)
GET /api/v1/sessions/:id/raw-output

# Other
GET /api/v1/search?q=              # Full-text search
POST /api/v1/dependencies          # Add dependency
DELETE /api/v1/dependencies        # Remove dependency
GET /api/v1/status                 # Dashboard summary
GET /api/v1/ws                     # WebSocket
```

Project is specified via `X-Project` header or `?project=` query parameter.

## Live Tracking

When agents are running, the UI shows real-time messages via WebSocket:

- Agent sessions display: status (running/completed/failed/timeout/continued), model ID, messages loaded from API (newest first)
- Clicking any agent node in PhaseGraph opens a modal with full message history
- The spawner broadcasts `messages.updated` every ~2s during agent execution

### Message Format

The spawner parses JSON stream output and formats messages with tool details:

```
[Bash] git status
[Read] /path/to/file.ts
[Grep] pattern in src/api/
[Skill] skill:jira-ticket REF-12425
[Task] codebase-explorer: Find error handlers
[Edit] /src/main.ts
[stderr] API rate limit warning
```

Both Claude CLI and Opencode output formats are supported with automatic normalization:
- Tool names normalized to title case (`bash` → `Bash`)
- Long messages truncated as `START (300 chars) ... [N chars] ... END (150 chars)`
- Stderr captured with `[stderr]` prefix
- Scanner buffer: 10MB limit for large JSON outputs
