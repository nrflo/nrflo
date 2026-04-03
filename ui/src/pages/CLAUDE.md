# Pages

Route page components for the nrflow web UI. Uses React Router v6 for routing. This directory contains 59 files including page components and co-located tests.

## Routes

| Route | Component | Description |
|-------|-----------|-------------|
| `/` | `Dashboard.tsx` | Overview with ticket counts and status |
| `/tickets` | `TicketListPage.tsx` | Ticket list with filtering |
| `/tickets/new` | `CreateTicketPage.tsx` | Create new ticket form |
| `/tickets/:id/edit` | `EditTicketPage.tsx` | Edit existing ticket form |
| `/tickets/:id` | `TicketDetailPage.tsx` | Ticket detail with tabbed interface |
| `/workflows` | `WorkflowsPage.tsx` | Workflow definitions and agent definitions CRUD |
| `/project-workflows` | `ProjectWorkflowsPage.tsx` | Project-scoped workflows (3-tab layout) |
| `/git-status` | `GitStatusPage.tsx` | Standalone git commit status page (conditional on `default_branch`) |
| `/chains` | `ChainListPage.tsx` | Chain list with status filtering, create/edit dialog |
| `/chains/:id` | `ChainDetailPage.tsx` | Chain items table, start/cancel/edit, useTickingClock for 1s elapsed time updates + 10s refetchInterval fallback when running |
| `/errors` | `ErrorsPage.tsx` | Paginated error log table with type filter tabs (All/Agent/Workflow/System), server-side pagination, WS auto-refresh |
| `/settings` | `SettingsPage.tsx` | Tabbed settings page (General, Projects, System Agents, Default Templates, CLI Models, Logs) composing GlobalSettingsSection + ProjectsSection + SystemAgentsSection + DefaultTemplatesSection + CLIModelsSection + LogsSection |

Routes are defined in `src/App.tsx`.

## Ticket Detail Page

The ticket detail page (`TicketDetailPage.tsx`) uses a tabbed interface:

- **Workflow tab** (default): Renders `TicketWorkflowTab` with Running/Failed/Completed sub-tabs (three-way partition matching `ProjectWorkflowsPage`), instance chips via `InstanceList`, and `CompletedAgentsTable` for completed instances
- **Hierarchy tab**: Blockers (add/remove), blocks, epic hierarchy (parent + siblings/children)
- **Description tab**: Ticket title heading, all metadata (priority, type, status, timestamps, close reason), description text
- **Details tab**: Read-only dependency lists, description text, metadata

### Tab Content Components

| Component | Content |
|-----------|---------|
| `TicketWorkflowTab.tsx` | Workflow tab with Running/Failed/Completed sub-tabs, three-way instance partitioning, `InstanceList` chips, `CompletedAgentsTable` for completed tab, `AgentTerminalDialog`. Manages workflow mutations. |
| `HierarchyTabContent.tsx` | Blockers with TicketSearchDropdown for add/remove, blocks display, epic hierarchy (parent ticket link + title, sibling list with current ticket highlighted, children list for epics) |
| `DescriptionTabContent.tsx` | Ticket title as h2, metadata grid, description text |
| `DetailsTabContent.tsx` | Read-only dependency lists (blocked by / blocks with titles), description text, metadata grid |
| `GitStatusTabContent.tsx` | Paginated git commits list with refresh, opens CommitDetailDialog on click (used by `GitStatusPage`) |

**Real-time updates**: The page uses WebSocket exclusively for real-time updates. Subscribes to the current ticket on mount via `useWebSocket()` hook. No REST polling.

## ProjectWorkflowsPage

4-tab layout for project-scoped workflows with multi-instance support:

- **Run Workflow**: Inline workflow selector + instructions form
- **Running**: Instance list chips with status (uses `InstanceList` + `WorkflowTabContent`)
- **Failed**: `WorkflowInstanceTable` with delete column, plus `WorkflowTabContent` for phase timeline
- **Completed**: `WorkflowInstanceTable` with delete column, plus `WorkflowTabContent` for phase timeline (same pattern as Failed tab)

Sub-components in `ProjectWorkflowComponents.tsx`:
- `ProjectWorkflowTabBar` — tab bar component
- `RunWorkflowForm` — inline workflow selector + instructions
- `InstanceList` — instance selector chips (used by Running tab and `TicketWorkflowTab`)

Instance table in `WorkflowInstanceTable.tsx`:
- `WorkflowInstanceTable` — paginated table (PAGE_SIZE=10) with Workflow, Instance, Status, Duration, Completed At, Delete columns (used by Completed and Failed tabs)

Shared sub-component in `WorkflowSubTabBar.tsx`:
- `WorkflowSubTabBar` — Running/Failed/Completed sub-tab switcher with counts (used by `TicketWorkflowTab`)

Supports multiple concurrent instances keyed by `instance_id`.

## Real-Time Update Patterns

Pages receive real-time updates via WebSocket (no REST polling):
- Ticket pages subscribe to specific ticket ID
- Project workflow pages subscribe with empty `ticketId` for project-scoped events
- Layout.tsx subscribes to all project events for sidebar counts and dashboard updates
- Subscriptions must be gated on `projectsLoaded` (see [hooks/CLAUDE.md](../hooks/CLAUDE.md))

## Testing

Tests are co-located with page components using `.test.tsx` suffix. Some pages use a `__tests__/` subdirectory for additional test organization.

Run tests: `make test-ui ARGS="src/pages/"`
