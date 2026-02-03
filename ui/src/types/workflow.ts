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
  phase: string
  workflow: string
  agent_type: string
  model_id?: string
  status: AgentSessionStatus
  last_messages: string[]
  message_stats: Record<string, number>  // { "tool:Read": 15, "skill:commit -m \"msg\"": 1, "text": 8 }
  created_at: string
  updated_at: string
}

export interface AgentSessionsResponse {
  ticket_id: string
  sessions: AgentSession[]
}

export interface RecentAgentsResponse {
  sessions: AgentSession[]
  projects: Record<string, string> // project_id -> project_name
}

export interface AgentsByProject {
  project_id: string
  project_name: string
  agents: AgentSession[]
}
