# Workflow Components

Workflow visualization and interaction components for ticket and project-scoped workflow views (42 files covering phase timeline, agent display, findings, and workflow definition management).

Top-level rendering component: `PhaseTimeline.tsx` renders workflow metadata badges and hosts `PhaseGraph`. Workflow state flows in from `useWebSocketSubscription`/`useTickets` via props; real-time refresh via `messages.updated` WS events. Shared types: `ui/src/types/workflow.ts`.

Run `ls ui/src/components/workflow/` for the full file list.

## PhaseGraph

React Flow (`@xyflow/react`) graph with ELK.js auto-layout (layered/Sugiyama). Implementation under `PhaseGraph/`.

- Shows ALL phases from workflow config upfront: pending (dashed/clock), skipped (faded), running (yellow glow), completed (green/red).
- Phases ordered by `phase_order` from backend; edges color-coded by source result.
- Clicking an agent node opens it in `AgentLogPanel` (right-side detail view with message table).
- Responsive: 220px nodes on mobile (<640px), 300px on desktop; touch/pinch-zoom on mobile via `useIsMobile`.
- Auto-center toggle (default on): `PhaseGraphControls.tsx` calls `fitView` every 15s; all fit-view paths route through `performFitView` (`fitViewOptions.ts`) via `requestAnimationFrame`.
- Height clamped to min 140px so the 4-button controls panel stays fully visible on short layouts.
- `AgentsTable.tsx` provides a flat table view for simplified-graph mode.

## Agent Log Panel

`AgentLogPanel.tsx` renders agents in full detail via `AgentLogDetail`:

- **Multi-agent tabbed view**: when running agents exist, a tab bar shows one tab per running agent; auto-selects first agent when current tab's agent completes.
- **Selected agent view**: single agent detail with a back button returning to multi-agent view.
- Collapses to a thin bar (`w-10`) with "Agent Log" label via `PanelShell`/`CollapsedBar`.

`AgentLogDetail` tabs: Messages (timestamp/tool/message table), Context (user prompt + system prompt suffix), Findings (filtered to selected agent via `FindingsPanel`), All Findings (`AllFindingsPanel` across entire workflow).

## Findings

- `FindingsPanel.tsx` — project findings first, then agent findings grouped by `agent_type`; each key collapsible; filters internal keys (`_` prefix). Exports `FindingRow` and `isInternalKey`.
- `AllFindingsPanel.tsx` — consolidated view: workflow-level → project → all agents sorted by layer from `phaseLayers`.

## Testing

Tests co-located using `ComponentName.test.tsx`. Variant tests use descriptive suffixes (e.g., `AgentLogPanel.width.test.tsx`).

Run: `make test-ui ARGS="src/components/workflow/"`
