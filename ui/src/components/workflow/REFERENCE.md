# Workflow Components Reference

Deep mechanics for this directory. The auto-loaded map lives in [CLAUDE.md](CLAUDE.md); read the relevant section here before changing the components below.

Contents: [PhaseGraph](#phasegraph) · [Agent Definitions](#agent-definitions) · [Trace](#trace) · [Agent Log Panel](#agent-log-panel)

## PhaseGraph

- Shows ALL phases from workflow config upfront: pending (dashed/clock), skipped (faded), running (yellow glow), completed (green/red).
- Phases ordered by `phase_order` from backend; edges color-coded by source result.
- Clicking an agent node opens it in `AgentLogPanel` (right-side detail view with message table).
- Responsive: 220px nodes on mobile (<640px), 300px on desktop; touch/pinch-zoom on mobile via `useIsMobile`.
- Auto-center toggle (default on): `PhaseGraphControls.tsx` calls `fitView` every 15s; all fit-view paths route through `performFitView` (`fitViewOptions.ts`) via `requestAnimationFrame`.
- Height clamped to min 140px so the 4-button controls panel stays fully visible on short layouts.
- `AgentsTable.tsx` provides a flat table view for simplified-graph mode.

## Agent Definitions

`AgentDefForm.tsx` (+ `AgentDefEffortField.tsx`, `AgentDefAPIModeFields.tsx`, `AgentDefNodeRoleFields.tsx` sub-fields) edits an `AgentDef`, including its optional `reasoning_effort` (gated Dropdown options shared with the model forms via `src/components/settings/effortOptions.ts`; empty = inherit from the model row). `AgentDefsSection`/`AgentDefCard` accept an optional `project` scope prop (defaults to the active project) so `WorkflowsPage` can manage global (`is_global`) workflow templates in the reserved `__global__` namespace when the viewer is admin.

## Trace

`Trace/` — Gantt-style run timeline (`TraceView`): percentage-positioned divs on a linear time scale (pure math in `timeScale.ts`), one lane per agent with relaunch-chain segments, pixel-bucketed event markers with category filter chips (closed tool spans render as duration bars via `splitSpans`; narrow/overflow spans degrade to dots), child sub-workflow rows with breadcrumb drill-down. `lifecycle` markers (segment-end reasons, rate-limit waits) come from session columns; nudge/stop-block counters render as lane badges. Zoom (`useTraceZoom`, 1–32×): Ctrl/Cmd+wheel anchored at the cursor or ± buttons; zoom widens the inner plot so panning is native horizontal scroll and marker bucketing de-clusters automatically; tick density scales with zoom. Data from `useTrace` (`GET /workflow-instances/{iid}/trace`); no timers — the running edge advances on WS-driven refetches (`dataUpdatedAt`). Clicks open `AgentLogPanel`. Hosted by `TicketTraceSection` (ticket Trace sub-tab) and `ProjectTraceSection` (project runs).

## Agent Log Panel

`AgentLogPanel.tsx` renders agents in full detail via `AgentLogDetail`:

- **Multi-agent tabbed view**: when running agents exist, a tab bar shows one tab per running agent; auto-selects first agent when current tab's agent completes.
- **Selected agent view**: single agent detail with a back button returning to multi-agent view.
- Collapses to a thin bar (`w-10`) with "Agent Log" label via `PanelShell`/`CollapsedBar`.

`AgentLogDetail` tabs: Messages (timestamp/tool/message table), Context (user prompt + system prompt suffix), Findings (filtered to selected agent via `FindingsPanel`), All Findings (`AllFindingsPanel` across entire workflow).
