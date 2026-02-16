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
└── WorkflowFindings (all workflow findings at bottom)
    ├── WorkflowLevelFindings (findings['workflow'] - blue styling)
    └── AgentFindings (other keys, empty findings filtered out - purple styling)
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

## Agent Log Panel

The right-side panel (`AgentLogPanel.tsx`) has two modes:

- **Overview mode** (default): Shows running agents with compact messages. Visible when agents are running.
- **Detail mode**: Shows a single agent's messages in a table (timestamp|tool|message columns). Activated when clicking an agent in the PhaseGraph or in the overview. Includes a back button to return to overview.

The panel also shows when a completed agent is selected from PhaseGraph (even after all agents finish). Uses `AgentLogDetail.tsx` for the detail view. The panel collapses to a thin bar (w-10) with vertical label.

## Key Components

| Component | Purpose |
|-----------|---------|
| `AgentSessionCard.tsx` | Reusable agent session card |
| `AgentMessagesPanel.tsx` | Agent sessions panel for ticket view |
| `AgentLogDetail.tsx` | Single-agent detail with message table (timestamp, tool, message columns) |
| `CompletedAgentsTable.tsx` | Unified pageable table of completed agents sorted by `ended_at` DESC. Supports optional Workflow column (`showWorkflowColumn` prop) and client-side pagination (20 rows/page). Duration uses `formatElapsedTime` from timestamps with `formatDuration` fallback. Used directly by `ProjectWorkflowsPage` for the completed tab (bypasses `WorkflowTabContent`). |
| `LogMessage.tsx` | Log message with tool name color highlighting. Exports `parseToolName` and `ToolBadge` |
| `ActiveAgentsPanel.tsx` | Active agents display panel |
| `FindingsViewer.tsx` | Simple KEY: VALUE findings display |
| `WorkflowFindings.tsx` | Workflow-level findings grouped by agent |
| `PhaseCard.tsx` | Phase card with agent history and findings |

## Findings Display Rules

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
- Components: `SimpleFindingValue` in PhaseCard.tsx, WorkflowFindings.tsx, FindingsViewer.tsx

## Workflow Definition Management

| Component | Purpose |
|-----------|---------|
| `WorkflowDefForm.tsx` | Workflow definition create/edit form |
| `PhaseListEditor.tsx` | Layer-aware phase list editor with fan-in validation |
| `AgentDefForm.tsx` | Agent definition create/edit form |
| `AgentDefCard.tsx` | Agent definition card with edit/delete |
| `AgentDefsSection.tsx` | Agent definitions list within a workflow |
| `RunWorkflowDialog.tsx` | Dialog for starting orchestrated ticket workflow runs |
| `RunEpicWorkflowDialog.tsx` | Dialog for epic workflow execution: two-step flow (create chain preview, then start) |

## Testing

Tests are co-located with source files using the naming convention `ComponentName.test.tsx`. Variant tests use descriptive suffixes:
- `AgentLogPanel.width.test.tsx` — width/resize behavior
- `CategoryRemoval.regression.test.tsx` — regression tests

Run tests: `npx vitest run src/components/workflow/`
