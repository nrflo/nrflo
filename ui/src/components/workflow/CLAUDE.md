# Workflow Components

Workflow visualization and interaction components for ticket and project-scoped workflow views (42 files covering phase timeline, agent display, findings, and workflow definition management). Deep mechanics: [REFERENCE.md](REFERENCE.md) — read it before changing the phase graph, trace timeline, agent log panel, or agent definition forms.

Top-level rendering component: `PhaseTimeline.tsx` renders workflow metadata badges and hosts `PhaseGraph`. Workflow state flows in from `useWebSocketSubscription`/`useTickets` via props; real-time refresh via `messages.updated` WS events. Shared types: `ui/src/types/workflow.ts`.

Run `ls ui/src/components/workflow/` for the full file list.

## PhaseGraph

React Flow (`@xyflow/react`) graph with ELK.js auto-layout (layered/Sugiyama); shows all phases upfront with per-status styling. Implementation under `PhaseGraph/`. Details: [REFERENCE.md](REFERENCE.md#phasegraph) — read before changing node states, layout, or fit-view behaviour.

## Agent Definitions

`AgentDefForm.tsx` (+ sub-field components) edits an `AgentDef`, including optional `reasoning_effort` and admin-managed global (`__global__`) templates. `native_tools` (anthropic CLI defs) and `sandbox` (openai CLI defs) render provider-gated and are auto-cleared when the model/mode moves away, since the backend hard-rejects mismatches. Details: [REFERENCE.md](REFERENCE.md#agent-definitions) — read before changing the form fields or template scoping.

## Plan Approval

`PlanApprovalBanner.tsx` (approve/revise/cancel, revision-pinned) renders `PlanManifestView.tsx` (read-only manifest) and opens `PlanReviseDialog.tsx` (feedback + open-question answers, submitted via `useRevisePlan`) when revising.

## Trace

`Trace/` — Gantt-style run timeline (`TraceView`) with tool-duration spans, lifecycle markers, zoom, and sub-workflow drill-down; data from `useTrace`, hosted by `TicketTraceSection`/`ProjectTraceSection`. Details: [REFERENCE.md](REFERENCE.md#trace) — read before changing timeline math, zoom, or marker bucketing.

## Agent Log Panel

`AgentLogPanel.tsx` renders agents in full detail via `AgentLogDetail` (Messages / Context / Findings / All Findings tabs), with a multi-agent tabbed view and collapse-to-bar. Details: [REFERENCE.md](REFERENCE.md#agent-log-panel) — read before changing tab selection or panel collapse behaviour.

## Findings

- `FindingsPanel.tsx` — project findings first, then agent findings grouped by `agent_type`; each key collapsible; filters internal keys (`_` prefix). Exports `FindingRow` and `isInternalKey`.
- `AllFindingsPanel.tsx` — consolidated view: workflow-level → project → all agents sorted by layer from `phaseLayers`.

## Testing

Tests co-located using `ComponentName.test.tsx`. Variant tests use descriptive suffixes (e.g., `AgentLogPanel.width.test.tsx`).

Run: `make test-ui ARGS="src/components/workflow/"`
