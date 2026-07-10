// Mirror of be/internal/types/trace.go — the GET /workflow-instances/{iid}/trace
// contract. All timestamps are RFC3339 strings; everything optional is tolerant
// so partial/older payloads render without crashing.

export interface TraceSegment {
  session_id: string
  status: string
  result?: string
  started_at?: string | null
  ended_at?: string | null
}

export interface TraceRestart {
  reason: string
  duration_sec: number
  context_left?: number
  message_count: number
}

export type TraceMarkerType =
  | 'tool'
  | 'subagent'
  | 'skill'
  | 'user_input'
  | 'error'
  | 'thinking'
  | 'finding'
  | string

export interface TraceMarker {
  type: TraceMarkerType
  at: string
  session_id?: string
  label: string
}

export interface TraceLaneData {
  lane_id: string
  phase: string
  layer: number // -1 when phase is absent from the current workflow def
  agent_type: string
  model_id?: string
  status: string
  result?: string
  segments?: TraceSegment[]
  restarts?: TraceRestart[]
  markers?: TraceMarker[]
}

export interface TraceLayer {
  layer: number
  phases?: string[]
  started_at?: string | null
  ended_at?: string | null
}

export interface TraceChild {
  instance_id: string
  workflow: string
  status: string
  started_at: string
  ended_at?: string | null
  parent_session_id?: string
}

export interface WorkflowTraceResponse {
  instance_id: string
  project_id: string
  ticket_id?: string
  workflow: string
  status: string
  started_at: string
  ended_at?: string | null
  layers?: TraceLayer[]
  lanes?: TraceLaneData[]
  children?: TraceChild[]
  root_markers?: TraceMarker[]
  truncated?: boolean
}
