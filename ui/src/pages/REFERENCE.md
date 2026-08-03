# Pages Reference

Deep mechanics for this directory. The auto-loaded map lives in [CLAUDE.md](CLAUDE.md); read the relevant section here before changing the pages below.

Contents: [Routes](#routes) · [Ticket Detail Page](#ticket-detail-page) · [ProjectWorkflowsPage](#projectworkflowspage)

## Routes

| Route | Component | Description |
|-------|-----------|-------------|
| `/` | `Dashboard.tsx` | Overview with ticket counts and status |
| `/tickets` | `TicketListPage.tsx` | Ticket list with filtering |
| `/tickets/new` | `CreateTicketPage.tsx` | Create new ticket form |
| `/tickets/:id/edit` | `EditTicketPage.tsx` | Edit existing ticket form |
| `/tickets/:id` | `TicketDetailPage.tsx` | Ticket detail with tabbed interface |
| `/workflows` | `WorkflowsPage.tsx` | Workflow CRUD + per-row Export, header Export All / Import (`WorkflowImportDialog`). Cards in `WorkflowCard.tsx`. |
| `/project-workflows` | `ProjectWorkflowsPage.tsx` | Project-scoped workflows (5-tab layout: Run / Running / Failed / Completed / Findings) |
| `/git-status` | `GitStatusPage.tsx` | Standalone git commit status page (conditional on `default_branch`) |
| `/chains` | `ChainListPage.tsx` | Chain list with status filtering, create/edit dialog |
| `/chains/:id` | `ChainDetailPage.tsx` | Chain items table, start/cancel/edit, useTickingClock for 1s elapsed time updates + 10s refetchInterval fallback when running |
| `/errors` | `ErrorsPage.tsx` | Paginated error log table with type filter tabs (All/Agent/Workflow/System), server-side pagination, WS auto-refresh |
| `/logs` | `LogsPage.tsx` | Agent sessions page — three-tab shell, tab + selected session driven by `?tab=sessions\|finished\|live&sid=…` (default `sessions`): **Sessions** (`LogsSessionsTab`, default) project/global scope toggle over `GET /api/v1/sessions[/global]`, `SessionsTable` row click sets `sid`, opening `SessionDetail` (flow graph via `GET /sessions/{sid}/flow` + elkjs layout, tool distribution and cost rollup via `GET /sessions/{sid}/stats`); **Finished sessions** (`LogsFinishedTab`) paginated table (Finished/SID/Agent/Model/Mode/Workflow/Duration/Status/Result), WS auto-refresh on `agent.completed`; **Live processes** (`LogsLiveTab`) fetches `/agent-session-logs/live`, no auto-refresh, manual Refresh, per-row Kill via ConfirmDialog |
| `/schedules` | `SchedulesPage.tsx` | Scheduled tasks table; write controls (New/Edit/Delete/Run-now/Toggle) hidden for non-admins via `useIsAdmin()`; ReadOnlyHint shown at top |
| `/workflow-chains` | `WorkflowChainsPage.tsx` | Workflow chain list; New/Delete admin-only; ReadOnlyHint at top for non-admins; clicking row navigates to editor |
| `/workflow-chains/:id` | `WorkflowChainEditorPage.tsx` | Chain editor — chain metadata form + ordered step list with Up/Down reorder, per-step inline form, Add/Delete step |
| `/python-scripts` | `PythonScriptsPage.tsx` | Python scripts CRUD with Agents/Tools tab toggle (`?kind=agent\|tool`, default `agent`). Tab selection drives `usePythonScripts(kind)` and routes create/edit to `PythonScriptForm` (agent) or `PythonToolForm` (tool). New/Edit/Delete admin-only; ConfirmDialog for delete; save-anyway flow for syntax errors; ReadOnlyHint for non-admins |
| `/settings` | `SettingsPage.tsx` | Tabbed settings page (General, Menu Panel, Projects, System Agents, Default Templates, Models, Logs, Connections, Administration) — admin-only, gated via `RequireAdmin` at route level; Models has Anthropic / OpenAI provider sub-tabs via `?tab=models&sub=anthropic\|openai` and shows each row's CLI/API mode support; Administration has Users / Audit Log / Service Tokens sub-tabs via `?sub=users\|audit\|tokens`; Connections renders `ConnectionsSection`; Projects is lifecycle-only, with per-project editing on `/project-settings` |
| `/project-settings` | `ProjectSettingsPage.tsx` | Standalone per-project settings page — admin-only, gated via `RequireAdmin`; resolves active project from `useProjectStore`; renders `ProjectForm` (identity + Safety Hook + env vars/artifact/cleanup/observer editors); save orchestration via `useSaveProjectSettings` hook |
| `/documentation` | `DocumentationPage.tsx` | Agent docs viewer with Common/CLI/Python/API sub-tabs via `?sub=<common\|cli\|python\|api>` (default `common`); content served from backend embedded `doc/` |
| `/console` | `ConsolePage.tsx` | Admin-only, gated via `RequireAdmin`; console-chat session list + New chat form (left) and transcript/composer (right, `src/components/console/`); selected session id in `?session=` |

Routes are defined in `src/App.tsx`.

## Ticket Detail Page

The ticket detail page (`TicketDetailPage.tsx`) uses a tabbed interface:

- **Workflow tab** (default): Renders `TicketWorkflowTab` with Running/Failed/Completed/Trace sub-tabs (bar shown whenever any instance exists), instance chips via `InstanceList`, `TicketCompletedSection` (merged `CompletedAgentsTable`) for completed, `TicketTraceSection` (Gantt timeline, see workflow/CLAUDE.md → Trace) for trace
- **Hierarchy tab**: Blockers (add/remove), blocks, epic hierarchy (parent + siblings/children)
- **Description tab**: Ticket title heading, all metadata (priority, type, status, timestamps, close reason), description text
- **Details tab**: Read-only dependency lists, description text, metadata

### Tab Content Components

| Component | Content |
|-----------|---------|
| `TicketWorkflowTab.tsx` | Workflow tab with Running/Failed/Completed/Trace sub-tabs, three-way instance partitioning, `InstanceList` chips; completed/trace bodies extracted to `TicketCompletedSection.tsx`/`TicketTraceSection.tsx`. Pushes interactive sessions into `interactiveSessionsStore`. Manages workflow mutations. |
| `HierarchyTabContent.tsx` | Blockers with TicketSearchDropdown for add/remove, blocks display, epic hierarchy (parent ticket link + title, sibling list with current ticket highlighted, children list for epics) |
| `DescriptionTabContent.tsx` | Ticket title as h2, metadata grid, description text |
| `DetailsTabContent.tsx` | Read-only dependency lists (blocked by / blocks with titles), description text, metadata grid |
| `GitStatusTabContent.tsx` | Paginated git commits list with refresh, opens CommitDetailDialog on click (used by `GitStatusPage`) |

**Real-time updates**: The page uses WebSocket exclusively for real-time updates. Subscribes to the current ticket on mount via `useWebSocket()` hook. No REST polling.

## ProjectWorkflowsPage

5-tab layout: Run Workflow / Running / Failed / Completed / Findings (project-level findings CRUD). Running tab uses `InstanceList` + `WorkflowTabContent`; Failed/Completed render `ProjectTerminalTab.tsx` (`WorkflowInstanceTable`, paginated PAGE_SIZE=10, + read-only `WorkflowTabContent`); a per-row Gantt button and a Running-tab Trace button open `ProjectTraceSection.tsx` in place of the tab content. Sub-components in `ProjectWorkflowComponents.tsx` (`ProjectWorkflowTabBar`, `InstanceList`), `RunWorkflowForm.tsx` (Run tab form with embedded `ArtifactUploader` for `input_artifacts`), and `WorkflowSubTabBar.tsx` (`WorkflowSubTabBar` — shared Running/Failed/Completed/Trace sub-tab switcher, used by `TicketWorkflowTab`). The page holds `stagedArtifacts` state + `launchedRef` to cancel orphaned uploads on tab-leave / unmount and skip cancellation after a successful launch.
