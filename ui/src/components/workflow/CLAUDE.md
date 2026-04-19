# Workflow Components

Workflow visualization and interaction components for ticket and project-scoped workflow views. This is the largest component directory (42 files) covering phase timeline, agent display, findings, and workflow definition management.

## Component Tree

```
PhaseTimeline (PhaseTimeline.tsx)
├── Workflow metadata badges (version, current phase)
├── PhaseGraph (PhaseGraph/)
│   ├── PhaseGraph.tsx - Main container using React Flow (@xyflow/react)
│   ├── AgentFlowNode.tsx - Custom React Flow node for agents (clickable, opens modal)
│   ├── layout.ts - ELK.js-based auto-layout (layered/Sugiyama algorithm)
│   ├── PhaseFlowNode.tsx - Custom React Flow node for phases
│   ├── PhaseNode.tsx - Standalone phase node
│   ├── AgentCard.tsx - Running agent card with elapsed time
│   ├── HistoryAgentCard.tsx - Completed agent card for phase history
│   └── types.ts - TypeScript types for graph components
```

## PhaseGraph Features

- Uses React Flow library (`@xyflow/react`) for graph rendering
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
- Agent sessions always fetched for ticket (enables history messages), refreshed via WebSocket `messages.updated` events
- Custom node uses `nopan nodrag` classes and `pointerEvents: 'all'` for click handling in ReactFlow
- **Responsive mobile layout**: Nodes are 220px on mobile (<640px), 300px on desktop. ELK spacing reduced on mobile (30/60 vs 60/120). Touch interactions enabled: pinch-to-zoom always on, pan-on-drag on mobile only. Uses `useIsMobile` hook for JS-level detection and Tailwind `sm:` breakpoints for CSS.
- **Auto-center toggle**: The React Flow `Controls` toolbar (top-left) renders four custom `ControlButton`s: zoom-out, zoom-in, fit-view, and an "Auto center graph every 15s" checkbox (default checked, Tooltip on hover). While checked, `AutoCenterInterval` calls `fitView(FIT_VIEW_OPTIONS)` every 15s. The interval stores `fitView` in a ref and depends only on `enabled`, so it survives parent re-renders (e.g. WS-driven session updates on the ticket page) without being torn down and re-armed. Clicking any of the three zoom/fit buttons unchecks the toggle so the interval does not fight manual navigation. Checkbox state is local component state (no persistence). The PhaseGraph wrapper clamps its height to a min of 140px so the vertical 4-button controls panel stays fully visible on short layouts (e.g. single-layer project workflows). Implementation lives in `PhaseGraph/PhaseGraphControls.tsx`; shared `FIT_VIEW_OPTIONS` and the `performFitView(fitViewFn)` helper live in `PhaseGraph/fitViewOptions.ts`. All fit-view call sites (button, 15s interval, container-resize + nodeKey effects in `PhaseGraph.FitViewOnChange`) route through `performFitView` — it wraps the call in `requestAnimationFrame` so React Flow reads the latest node measurements before computing zoom, keeping the manual button and the auto-center tick at the same viewport.

## Agent Log Panel

The right-side panel (`AgentLogPanel.tsx`) always renders agents in full detail view via `AgentLogDetail`:

- **Multi-agent tabbed view** (default, no selection): When running agents exist, a horizontal tab bar appears at the top with one tab per running agent (phase name with underscore→space, Loader2 spinner). Only the selected tab's `AgentLogDetail` is rendered at a time. Tab auto-selects the first available agent when the current tab's agent completes. No back button in this mode.
- **Selected agent view**: When a specific agent is selected (e.g., clicking a completed agent in PhaseGraph), shows that single agent's `AgentLogDetail` with a back button that returns to the multi-agent view (`onAgentSelect(null)`).
- **Auto-switch**: When a selected agent completes and other agents are still running, automatically returns to multi-agent view.

The panel collapses to a thin bar (w-10) with "Agent Log" label. Uses `PanelShell` and `CollapsedBar` internal components for consistent layout.

## Key Components

| Component | Purpose |
|-----------|---------|
| `AgentSessionCard.tsx` | Reusable agent session card |
| `AgentMessagesPanel.tsx` | Agent sessions panel for ticket view |
| `AgentLogDetail.tsx` | Single-agent detail with top-level Messages/Context/Findings/All Findings tabs; Messages tab shows message table (timestamp, tool, message columns), Context tab shows session prompt markdown, Findings tab shows FindingsPanel filtered to selected agent, All Findings tab shows AllFindingsPanel with all workflow/project/agent findings |
| `FindingsPanel.tsx` | Reusable findings display. Props: `projectFindings`, `agentFindings` (WorkflowFindings), `selectedAgentType`. Shows project findings first, then agent findings grouped by agent_type. Each finding key is collapsible (collapsed by default). Values displayed as pretty JSON or plain text. Filters out internal keys starting with `_`. Exports `FindingRow` and `isInternalKey` for reuse by AllFindingsPanel. |
| `AllFindingsPanel.tsx` | Consolidated findings view across entire workflow. Shows workflow-level findings at top, project findings next, then all agents' findings sorted by layer number from `phaseLayers`. Strips model suffix from agent keys for layer lookup. Filters internal keys (starting with `_`). |
| `CompletedAgentsTable.tsx` | Pageable table of completed agents sorted by `ended_at` DESC with client-side pagination (20 rows/page). Duration uses `formatElapsedTime` from timestamps with `formatDuration` fallback. Used by `ProjectWorkflowsPage` (per-instance) and `TicketWorkflowTab` (merged across instances). |
| `LogMessage.tsx` | Log message with tool name color highlighting. Exports `parseToolName` and `ToolBadge` |
| `ActiveAgentsPanel.tsx` | Active agents display panel |
## Workflow Definition Management

| Component | Purpose |
|-----------|---------|
| `WorkflowDefForm.tsx` | Workflow definition create/edit form (includes groups chip input) |
| `AgentDefForm.tsx` | Agent definition create/edit form (includes layer input, tag dropdown when groups available, "Apply Template" button opens TemplatePickerDialog) |
| `TemplatePickerDialog.tsx` | Dialog for selecting and applying a default template to an agent's prompt. Fetches from default-templates API, shows dropdown + preview, warns on non-empty prompt replacement. |
| `AgentDefCard.tsx` | Agent definition card with edit/delete (shows tag badge) |
| `AgentDefsSection.tsx` | Agent definitions list within a workflow (passes groups to children) |
| `RunWorkflowDialog.tsx` | Dialog for starting orchestrated ticket workflow runs. Supports "Start Interactive" and "Plan Before Execution" checkboxes (mutually exclusive). "Start Interactive" requires L0 to have exactly 1 Claude agent AND the workflow must have multiple layers. "Plan Before Execution" requires L0 to have exactly 1 Claude agent. Props: `onInteractiveStart?(sessionId, agentType)` callback for opening PTY terminal. Shows inline concurrent-workflow warning on 409 when worktrees disabled, with "Proceed Anyway" to re-submit with `force: true`. |
| `RunEpicWorkflowDialog.tsx` | Dialog for epic workflow execution: two-step flow (create chain preview, then start) |
| `AgentTerminalDialog.tsx` | Dialog wrapper for interactive agent terminal: non-dismissable backdrop, lazy-loads XTerminal, Exit Session footer button |
| `XTerminal.tsx` | xterm.js terminal connected to PTY WebSocket (`/api/v1/pty/{sessionId}`). Relays keystrokes to WS, output to terminal. FitAddon for auto-resize, debounced resize events, dark theme |

## Testing

Tests are co-located with source files using the naming convention `ComponentName.test.tsx`. Variant tests use descriptive suffixes:
- `AgentLogPanel.width.test.tsx` — width/resize behavior
- `CategoryRemoval.regression.test.tsx` — regression tests

Run tests: `make test-ui ARGS="src/components/workflow/"`
