// Mirror of be/internal/types/session_flow.go — the GET /api/v1/sessions,
// /api/v1/sessions/global, /api/v1/sessions/{sid}/flow and /{sid}/stats
// contracts. Everything optional is tolerant so older sessions (predating
// this audit) render without crashing.

export type SessionKind = string

export interface SessionListRow {
  session_id: string
  project_id?: string
  kind: SessionKind
  agent_type?: string
  model_id?: string
  status: string
  result?: string
  workflow_instance_id?: string
  workflow?: string
  ticket_id?: string
  started_at?: string
  ended_at?: string
  cost_estimate?: number
}

export interface SessionListResponse {
  sessions: SessionListRow[]
}

// Discriminates why two sessions are connected in the flow graph.
export type SessionFlowEdgeKind = 'delegate' | 'consult' | 'subworkflow' | 'sibling' | 'origin' | string

export interface SessionFlowNode {
  session_id: string
  kind: SessionKind
  agent_type?: string
  status: string
  result?: string
  workflow_instance_id?: string
  // Hops from the root session (0 = root).
  depth: number
}

export interface SessionFlowEdge {
  from_session_id: string
  to_session_id: string
  kind: SessionFlowEdgeKind
}

export interface SessionFlowResponse {
  root_session_id: string
  nodes: SessionFlowNode[]
  edges: SessionFlowEdge[]
  truncated: boolean
}

export interface ToolCallDistributionEntry {
  tool_name: string
  success: number
  error: number
}

export interface SessionStatsResponse {
  root_session_id: string
  tool_calls: ToolCallDistributionEntry[]
  self_cost_usd: number
  subtree_cost_usd: number
  self_tokens: number
  subtree_tokens: number
}
