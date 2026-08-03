export interface RestartDetail {
  reason: string
  duration_sec: number
  context_left?: number
  message_count: number
}

// Parallel agents (v4 format)
export interface ActiveAgentV4 {
  agent_type: string
  phase?: string
  model_id?: string
  pid?: number
  session_id?: string
  started_at?: string
  ended_at?: string
  result?: string
  context_left?: number
  restart_count?: number
  restart_threshold?: number
  restart_details?: RestartDetail[]
  nudge_count?: number
  tag?: string
  effective_mode?: 'cli_interactive' | 'api' | 'script'
  resolved_effort?: string
  waiting_for_rate_limit?: boolean
  rate_limit_until_ts?: string
  rate_limit_retry_count?: number
}

export interface AgentHistoryEntry {
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
  restart_details?: RestartDetail[]
  nudge_count?: number
  tag?: string
  effective_mode?: 'cli_interactive' | 'api' | 'script'
  resolved_effort?: string
}

export interface CompletedAgentRow extends AgentHistoryEntry {
  workflow_label: string
}
