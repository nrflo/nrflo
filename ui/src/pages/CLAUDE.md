# Pages

Route page components for the nrflo web UI. Uses React Router v6 for routing. This directory contains 59 files including page components and co-located tests. Deep mechanics: [REFERENCE.md](REFERENCE.md) — read it before changing routes, ticket tabs, or the project workflows layout.

## Routes

Routes are defined in `src/App.tsx`; one page component per route (run `ls ui/src/pages/`). Full route → component table with per-page behaviour notes: [REFERENCE.md](REFERENCE.md#routes) — read before adding, renaming, or changing a route's page. `/console` (admin-only) → `ConsolePage.tsx`.

## Ticket Detail Page

`TicketDetailPage.tsx` is a tabbed interface — Workflow (default, with Running/Failed/Completed/Trace sub-tabs) / Hierarchy / Description / Details — with WebSocket-only real-time updates. Tab bodies live in dedicated content components (`TicketWorkflowTab.tsx`, `HierarchyTabContent.tsx`, …). Details: [REFERENCE.md](REFERENCE.md#ticket-detail-page) — read before changing tabs or their content components.

## ProjectWorkflowsPage

5-tab layout (Run Workflow / Running / Failed / Completed / Findings) with per-row trace drill-in and staged-artifact upload state for launches. Details: [REFERENCE.md](REFERENCE.md#projectworkflowspage) — read before changing tab composition, sub-components, or artifact staging.

## Real-Time Update Patterns

Pages receive real-time updates via WebSocket (no REST polling):
- Ticket pages subscribe to specific ticket ID
- Project workflow pages subscribe with empty `ticketId` for project-scoped events
- Layout.tsx subscribes to all project events for sidebar counts and dashboard updates
- Subscriptions must be gated on `projectsLoaded` (see [hooks/CLAUDE.md](../hooks/CLAUDE.md))

## Testing

Tests are co-located with page components using `.test.tsx` suffix. Some pages use a `__tests__/` subdirectory for additional test organization.

Run tests: `make test-ui ARGS="src/pages/"`
