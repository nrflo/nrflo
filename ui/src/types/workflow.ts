export type PhaseStatus = 'pending' | 'in_progress' | 'completed' | 'skipped' | 'error'
export type PhaseResult = 'pass' | 'fail' | 'skipped' | null

export interface PhaseState {
  status: PhaseStatus
  result?: PhaseResult | string
  started_at?: string
  ended_at?: string
  error?: string
}

// v3 format - single active agent
export interface ActiveAgent {
  type: string
  pid?: number
  session_id?: string
  started_at?: string
}

// v4 format - parallel agents
export interface ActiveAgentV4 {
  agent_id?: string
  agent_type: string
  phase?: string
  model_id?: string
  cli?: string
  model?: string
  pid?: number
  session_id?: string
  started_at?: string
  ended_at?: string
  result?: string
}

export interface HistoryEntry {
  type: string
  phase: string
  status: string
  started_at?: string
  ended_at?: string
}

export interface AgentHistoryEntry {
  agent_id: string
  agent_type: string
  model_id?: string
  phase: string
  started_at?: string
  ended_at?: string
  result?: string
  duration_sec?: number
}

// Findings structure: agent_type -> findings (field -> value)
export type WorkflowFindings = Record<string, Record<string, unknown>>

export interface WorkflowState {
  workflow?: string
  version?: number
  current_phase?: string
  category?: string
  phases?: Record<string, PhaseState>
  phase_order?: string[]
  active_agent?: ActiveAgent | null      // v3 compat
  active_agents?: Record<string, ActiveAgentV4>  // v4 format: key is "agent_type:cli:model"
  findings?: WorkflowFindings
  history?: HistoryEntry[]
  agent_history?: AgentHistoryEntry[]
}

export interface WorkflowResponse {
  ticket_id: string
  has_workflow: boolean
  state: WorkflowState
  workflows?: string[]              // list of workflow names
  all_workflows?: Record<string, WorkflowState>  // all workflows for multi-workflow tickets
  agent_history?: AgentHistoryEntry[]  // agent execution history
  raw?: string                      // raw JSON string for debugging
  parse_error?: string
}

export interface UpdateWorkflowRequest {
  state?: Record<string, unknown>
  phase?: string
  phase_update?: Record<string, unknown>
  current_phase?: string
}

export interface DependenciesResponse {
  ticket_id: string
  blockers: import('./ticket').Dependency[]
  blocks: import('./ticket').Dependency[]
}

export interface DependencyRequest {
  child_id: string
  parent_id: string
  created_by?: string
}

// Agent session types for real-time monitoring
export type AgentSessionStatus = 'running' | 'completed' | 'failed' | 'timeout'

export interface AgentSession {
  id: string
  project_id: string
  ticket_id: string
  workflow_instance_id: string
  phase: string
  workflow: string
  agent_type: string
  model_id?: string
  status: AgentSessionStatus
  result?: string
  result_reason?: string
  pid?: number
  findings?: Record<string, unknown>
  last_messages?: string[]
  message_count: number
  context_left?: number
  ancestor_session_id?: string
  started_at?: string
  ended_at?: string
  created_at: string
  updated_at: string
}

export interface SessionMessagesResponse {
  session_id: string
  messages: string[]
  total: number
}

export interface AgentSessionsResponse {
  ticket_id: string
  sessions: AgentSession[]
}

// Workflow definition types (DB-stored)

export interface PhaseDef {
  id: string
  agent: string
  order?: number
  skip_for?: string[]
  parallel?: {
    enabled: boolean
    models: string[]
  }
}

/** WorkflowDef as returned by the list endpoint (no id/project_id/timestamps) */
export interface WorkflowDefSummary {
  description: string
  categories: string[]
  phases: PhaseDef[]
}

/** Full WorkflowDef as returned by get/create endpoints */
export interface WorkflowDef {
  id: string
  project_id: string
  description: string
  categories: string[]
  phases: (string | Record<string, unknown>)[]
  created_at: string
  updated_at: string
}

export interface WorkflowDefCreateRequest {
  id: string
  description?: string
  categories?: string[]
  phases: (string | Record<string, unknown>)[]
}

export interface WorkflowDefUpdateRequest {
  description?: string
  categories?: string[]
  phases?: (string | Record<string, unknown>)[]
}

// Agent definition types (DB-stored)

export interface AgentDef {
  id: string
  project_id: string
  workflow_id: string
  model: string
  timeout: number
  prompt: string
  created_at: string
  updated_at: string
}

export interface AgentDefCreateRequest {
  id: string
  model?: string
  timeout?: number
  prompt: string
}

export interface AgentDefUpdateRequest {
  model?: string
  timeout?: number
  prompt?: string
}

// Orchestration types

export interface RunWorkflowRequest {
  workflow: string
  category?: string
  instructions?: string
}

export interface RunWorkflowResponse {
  instance_id: string
  status: string
}

export interface StopWorkflowRequest {
  workflow?: string
}
