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
  restart_threshold?: number
}

export interface AgentHistoryEntry {
  agent_id: string
  agent_type: string
  session_id?: string
  model_id?: string
  phase: string
  started_at?: string
  ended_at?: string
  result?: string
  duration_sec?: number
  context_left?: number
  restart_count?: number
  restart_threshold?: number
}

export interface CompletedAgentRow extends AgentHistoryEntry {
  workflow_label: string
}

// Findings structure: agent_type -> findings (field -> value)
export type WorkflowFindings = Record<string, Record<string, unknown>>

export type ScopeType = 'ticket' | 'project'

export interface WorkflowState {
  workflow?: string
  instance_id?: string
  version?: number
  scope_type?: ScopeType
  current_phase?: string
  status?: string
  completed_at?: string
  total_duration_sec?: number
  total_tokens_used?: number
  phases?: Record<string, PhaseState>
  phase_order?: string[]
  phase_layers?: Record<string, number>
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

export interface ProjectWorkflowResponse {
  project_id: string
  has_workflow: boolean
  state: WorkflowState
  workflows?: string[]                           // deduplicated workflow names
  all_workflows?: Record<string, WorkflowState>  // keyed by instance_id
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
export type AgentSessionStatus = 'running' | 'completed' | 'failed' | 'timeout' | 'continued' | 'user_interactive' | 'interactive_completed'

export interface TakeControlRequest {
  workflow: string
  session_id: string
  instance_id?: string
}

export interface TakeControlResponse {
  status: string
  session_id: string
}

export interface ExitInteractiveRequest {
  workflow: string
  session_id: string
  instance_id?: string
}

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
  restart_count: number
  ancestor_session_id?: string
  started_at?: string
  ended_at?: string
  created_at: string
  updated_at: string
}

export type MessageCategory = 'text' | 'tool' | 'subagent' | 'skill'

export interface MessageWithTime {
  content: string
  category: MessageCategory
  created_at: string
}

export interface SessionMessagesResponse {
  session_id: string
  messages: MessageWithTime[]
  total: number
}

export interface AgentSessionsResponse {
  ticket_id: string
  sessions: AgentSession[]
}

export interface ProjectAgentSessionsResponse {
  project_id: string
  sessions: AgentSession[]
}

// Workflow definition types (DB-stored)

export interface PhaseDef {
  id: string
  agent: string
  layer: number
}

/** WorkflowDef as returned by the list endpoint (no id/project_id/timestamps) */
export interface WorkflowDefSummary {
  description: string
  scope_type?: ScopeType
  groups?: string[]
  phases: PhaseDef[]
}

/** Full WorkflowDef as returned by get/create endpoints */
export interface WorkflowDef {
  id: string
  project_id: string
  description: string
  scope_type?: ScopeType
  groups?: string[]
  phases: PhaseDef[]
  created_at: string
  updated_at: string
}

export interface WorkflowDefCreateRequest {
  id: string
  description?: string
  scope_type?: ScopeType
  groups?: string[]
  phases: PhaseDef[]
}

export interface WorkflowDefUpdateRequest {
  description?: string
  scope_type?: ScopeType
  groups?: string[]
  phases?: PhaseDef[]
}

// Agent definition types (DB-stored)

export interface AgentDef {
  id: string
  project_id: string
  workflow_id: string
  model: string
  timeout: number
  prompt: string
  restart_threshold?: number
  max_fail_restarts?: number
  tag?: string
  low_consumption_model?: string
  created_at: string
  updated_at: string
}

export interface AgentDefCreateRequest {
  id: string
  model?: string
  timeout?: number
  prompt: string
  restart_threshold?: number
  max_fail_restarts?: number
  tag?: string
  low_consumption_model?: string
}

export interface AgentDefUpdateRequest {
  model?: string
  timeout?: number
  prompt?: string
  restart_threshold?: number
  max_fail_restarts?: number
  tag?: string
  low_consumption_model?: string
}

// Orchestration types

export interface RunWorkflowRequest {
  workflow: string
  instructions?: string
  interactive?: boolean
  plan_mode?: boolean
}

export interface RunWorkflowResponse {
  instance_id: string
  status: string
  session_id?: string
}

export interface StopWorkflowRequest {
  workflow?: string
  instance_id?: string
}

export interface RestartAgentRequest {
  workflow: string
  session_id: string
  instance_id?: string
}

export interface ResumeSessionRequest {
  session_id: string
}

export interface ProjectWorkflowRunRequest {
  workflow: string
  instructions?: string
  interactive?: boolean
  plan_mode?: boolean
}
