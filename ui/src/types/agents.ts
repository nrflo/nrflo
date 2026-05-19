export interface RunningAgent {
  session_id: string
  project_id: string
  project_name: string
  ticket_id: string
  workflow_id: string
  agent_type: string
  model_id: string
  phase: string
  started_at: string
  elapsed_sec: number
  waiting_for_rate_limit?: boolean
  rate_limit_until_ts?: string
  rate_limit_retry_count?: number
}

export interface RunningAgentsResponse {
  agents: RunningAgent[]
  count: number
}
