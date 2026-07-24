# Workflow Components

Workflow visualization and interaction components for ticket and project-scoped workflow views (42 files covering phase timeline, agent display, findings, and workflow definition management). Deep mechanics: [REFERENCE.md](REFERENCE.md) — read it before changing the phase graph, trace timeline, agent log panel, or agent definition forms.

Top-level rendering component: `PhaseTimeline.tsx` renders workflow metadata badges and hosts `PhaseGraph`. Workflow state flows in from `useWebSocketSubscription`/`useTickets` via props; real-time refresh via `messages.updated` WS events. Shared types: `ui/src/types/workflow.ts`.

Run `ls ui/src/components/workflow/` for the full file list.

## PhaseGraph

React Flow (`@xyflow/react`) graph with ELK.js auto-layout (layered/Sugiyama); shows all phases upfront with per-status styling. Implementation under `PhaseGraph/`. Details: [REFERENCE.md](REFERENCE.md#phasegraph) — read before changing node states, layout, or fit-view behaviour.

## Agent Definitions

`AgentDefForm.tsx` (+ sub-field components) edits an `AgentDef`, including optional `reasoning_effort` and admin-managed global (`__global__`) templates. `native_tools` (anthropic CLI defs) and `sandbox` (openai CLI defs) render provider-gated and are auto-cleared when the model/mode moves away, since the backend hard-rejects mismatches. Details: [REFERENCE.md](REFERENCE.md#agent-definitions) — read before changing the form fields or template scoping.

`AgentDefForm.tsx` also carries a `prompt_mode` (`full`|`stepwise`) toggle: `AgentDefStepwiseSection.tsx` renders an ordered, add/remove/reorder step list (`StepDefinitionEditor.tsx` + `StepRequiredFindingsEditor.tsx`/`StepChecksOverlapEditor.tsx`) validated client-side via `src/lib/stepDefinitions.ts` (mirrors `service.validateStepDefinitions`) before submit; switching `execution_mode` to `script` resets `prompt_mode` back to `full` since script agents can't be stepwise. `AgentDefCard.tsx` badges stepwise defs as "stepwise · N steps".

`AgentDefModelTierFields.tsx` adds a Tier 1-5 selector plus an "Override model (skip tier fallback chain)" toggle; when off, the model is resolved server-side from the tier's fallback chain (`resolveTierChain`/`useTierModels`) and `AgentDefCard.tsx` shows the chain-primary model as a badge. `TieringSection.tsx` reports current-tier → recommended-tier per worker role instead of raw model names.

## Plan Approval

`PlanApprovalBanner.tsx` (approve/revise/cancel, revision-pinned) renders `PlanManifestView.tsx` (read-only manifest) and opens `PlanReviseDialog.tsx` (feedback + open-question answers, submitted via `useRevisePlan`) when revising.

## Trace

`Trace/` — Gantt-style run timeline (`TraceView`) with tool-duration spans, lifecycle markers, zoom, and sub-workflow drill-down; data from `useTrace`, hosted by `TicketTraceSection`/`ProjectTraceSection`. Details: [REFERENCE.md](REFERENCE.md#trace) — read before changing timeline math, zoom, or marker bucketing. Each lane's header renders `TimeBreakdownBar` — a 4-segment thinking/tool-arg/text/tool-wait stacked bar from `TraceLaneData.time_buckets` — returning `null` (no bar) when the lane has no granular per-block timing data.

## Agent Log Panel

`AgentLogPanel.tsx` renders agents in full detail via `AgentLogDetail` (Messages / Context / Ledger / Findings / All Findings tabs), with a multi-agent tabbed view and collapse-to-bar. The Ledger tab (`ContextLedgerPanel.tsx`) shows per-kind token breakdown, superseded entries, and an optional budget bar, fed by `GET /api/v1/sessions/{id}/context-ledger` plus live `agent.context_ledger` WS totals via `useSessionContextLedger`. The Ledger tab also shows a collapsible Handoff digest section (`HandoffDigestSection.tsx`, content + fold telemetry) above the panel, fed by `GET /api/v1/sessions/{id}/handoff-digest` plus live `agent.handoff_digest` WS events via `useSessionHandoffDigest`. Details: [REFERENCE.md](REFERENCE.md#agent-log-panel) — read before changing tab selection or panel collapse behaviour.

## Stepwise progress

`StepProgressStrip.tsx` renders an "N/M" badge plus per-step pips (hover tooltip: title/state/timestamp), fed by `useStepCursors` (REST snapshot + live `step.advanced` WS patches). Mounted in `AgentFlowNode.tsx`'s card body (graph view) and in `AgentsTable.tsx`'s Agent cell (simplified/table view, guarded on an `instanceId` prop threaded from `PhaseTimeline.tsx`); self-contained, renders `null` without a step cursor.

## Findings

- `FindingsPanel.tsx` — project findings first, then agent findings grouped by `agent_type`; each key collapsible; filters internal keys (`_` prefix). Exports `FindingRow` and `isInternalKey`.
- `AllFindingsPanel.tsx` — consolidated view: workflow-level → project → all agents sorted by layer from `phaseLayers`.

## Testing

Tests co-located using `ComponentName.test.tsx`. Variant tests use descriptive suffixes (e.g., `AgentLogPanel.width.test.tsx`).

Run: `make test-ui ARGS="src/components/workflow/"`
