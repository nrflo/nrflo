# Pages

Route page components for the nrflo web UI. Uses React Router v6 for routing. This directory contains 59 files including page components and co-located tests.

## Routes

| Route | Component | Description |
|-------|-----------|-------------|
| `/` | `Dashboard.tsx` | Overview with ticket counts and status |
| `/tickets` | `TicketListPage.tsx` | Ticket list with filtering |
| `/tickets/new` | `CreateTicketPage.tsx` | Create new ticket form |
| `/tickets/:id/edit` | `EditTicketPage.tsx` | Edit existing ticket form |
| `/tickets/:id` | `TicketDetailPage.tsx` | Ticket detail with tabbed interface |
| `/workflows` | `WorkflowsPage.tsx` | Workflow definitions and agent definitions CRUD |
| `/project-workflows` | `ProjectWorkflowsPage.tsx` | Project-scoped workflows (5-tab layout: Run / Running / Failed / Completed / Findings) |
| `/git-status` | `GitStatusPage.tsx` | Standalone git commit status page (conditional on `default_branch`) |
| `/chains` | `ChainListPage.tsx` | Chain list with status filtering, create/edit dialog |
| `/chains/:id` | `ChainDetailPage.tsx` | Chain items table, start/cancel/edit, useTickingClock for 1s elapsed time updates + 10s refetchInterval fallback when running |
| `/errors` | `ErrorsPage.tsx` | Paginated error log table with type filter tabs (All/Agent/Workflow/System), server-side pagination, WS auto-refresh |
| `/logs` | `LogsPage.tsx` | Agent sessions page — two-tab shell: **Finished sessions** (`LogsFinishedTab`) paginated table (Finished/SID/Agent/Model/Mode/Workflow/Duration/Status/Result), WS auto-refresh on `agent.completed`; **Live processes** (`LogsLiveTab`) fetches `/agent-session-logs/live`, no auto-refresh, manual Refresh, per-row Kill via ConfirmDialog |
| `/schedules` | `SchedulesPage.tsx` | Scheduled tasks table; write controls (New/Edit/Delete/Run-now/Toggle) hidden for non-admins via `useIsAdmin()`; ReadOnlyHint shown at top |
| `/workflow-chains` | `WorkflowChainsPage.tsx` | Workflow chain list; New/Delete admin-only; ReadOnlyHint at top for non-admins; clicking row navigates to editor |
| `/workflow-chains/:id` | `WorkflowChainEditorPage.tsx` | Chain editor — chain metadata form + ordered step list with Up/Down reorder, per-step inline form, Add/Delete step |
| `/python-scripts` | `PythonScriptsPage.tsx` | Python script CRUD — list with edit/delete, New button (admin-only), ConfirmDialog for delete, PythonScriptForm dialog for create/edit, save-anyway flow for syntax errors, ReadOnlyHint for non-admins |
| `/settings` | `SettingsPage.tsx` | Tabbed settings page (General, Projects, System Agents, Default Templates, Providers, Logs, Administration) — admin-only, gated via `RequireAdmin` at route level; Administration tab has Users / Audit Log sub-tabs via `?sub=users\|audit`; Providers tab has Claude / OpenCode / Codex sub-tabs via `?sub=claude\|opencode\|codex`; section components in `src/components/settings/` |

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
| `TicketWorkflowTab.tsx` | Workflow tab with Running/Failed/Completed sub-tabs, three-way instance partitioning, `InstanceList` chips, `CompletedAgentsTable` for completed tab. Pushes interactive sessions into `interactiveSessionsStore`. Manages workflow mutations. |
| `HierarchyTabContent.tsx` | Blockers with TicketSearchDropdown for add/remove, blocks display, epic hierarchy (parent ticket link + title, sibling list with current ticket highlighted, children list for epics) |
| `DescriptionTabContent.tsx` | Ticket title as h2, metadata grid, description text |
| `DetailsTabContent.tsx` | Read-only dependency lists (blocked by / blocks with titles), description text, metadata grid |
| `GitStatusTabContent.tsx` | Paginated git commits list with refresh, opens CommitDetailDialog on click (used by `GitStatusPage`) |

**Real-time updates**: The page uses WebSocket exclusively for real-time updates. Subscribes to the current ticket on mount via `useWebSocket()` hook. No REST polling.

## ProjectWorkflowsPage

5-tab layout: Run Workflow / Running / Failed / Completed / Findings (project-level findings CRUD). Running tab uses `InstanceList` + `WorkflowTabContent`; Failed/Completed use `WorkflowInstanceTable` (paginated, PAGE_SIZE=10) + `WorkflowTabContent`. Sub-components in `ProjectWorkflowComponents.tsx` (`ProjectWorkflowTabBar`, `RunWorkflowForm`, `InstanceList`) and `WorkflowSubTabBar.tsx` (`WorkflowSubTabBar` — shared Running/Failed/Completed sub-tab switcher, used by `TicketWorkflowTab`).

## Real-Time Update Patterns

Pages receive real-time updates via WebSocket (no REST polling):
- Ticket pages subscribe to specific ticket ID
- Project workflow pages subscribe with empty `ticketId` for project-scoped events
- Layout.tsx subscribes to all project events for sidebar counts and dashboard updates
- Subscriptions must be gated on `projectsLoaded` (see [hooks/CLAUDE.md](../hooks/CLAUDE.md))

## Testing

Tests are co-located with page components using `.test.tsx` suffix. Some pages use a `__tests__/` subdirectory for additional test organization.

Run tests: `make test-ui ARGS="src/pages/"`
