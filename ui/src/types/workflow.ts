export type PhaseStatus = 'pending' | 'in_progress' | 'completed' | 'skipped' | 'error'
export type PhaseResult = 'pass' | 'fail' | 'skipped' | null

export interface PhaseState {
  status: PhaseStatus
  result?: PhaseResult | string
  started_at?: string
  ended_at?: string
  error?: string
}

// Parallel agents (v4 format)
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
  context_left?: number
  restart_count?: number
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
  context_left?: number
  restart_count?: number
}

// Findings structure: agent_type -> findings (field -> value)
export type WorkflowFindings = Record<string, Record<string, unknown>>

export interface WorkflowState {
  workflow?: string
  version?: number
  current_phase?: string
  category?: string
  status?: string
  completed_at?: string
  total_duration_sec?: number
  total_tokens_used?: number
  phases?: Record<string, PhaseState>
  phase_order?: string[]
  active_agents?: Record<string, ActiveAgentV4>  // key is "agent_type:cli:model"
  findings?: WorkflowFindings
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
  issue_id: string
  depends_on_id: string
  created_by?: string
}

// Agent session types for real-time monitoring
export type AgentSessionStatus = 'running' | 'completed' | 'failed' | 'timeout' | 'continued'

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
  raw_output_size: number
  context_left?: number
  restart_count: number
  ancestor_session_id?: string
  started_at?: string
  ended_at?: string
  created_at: string
  updated_at: string
}

export interface MessageWithTime {
  content: string
  created_at: string
}

export interface SessionMessagesResponse {
  session_id: string
  messages: MessageWithTime[]
  total: number
}

export interface SessionRawOutputResponse {
  session_id: string
  raw_output: string
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

export interface RestartAgentRequest {
  workflow: string
  session_id: string
}
