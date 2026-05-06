export interface AgentSessionLogEntry {
  session_id: string
  project_id: string
  agent_type: string
  model_id?: string
  status: string
  started_at?: string
  ended_at?: string
  duration_sec?: number
  workflow_id: string
  workflow_instance_id: string
  scheduled: boolean
  execution_mode?: 'cli' | 'cli_interactive' | 'api' | 'script' | string
  workflow_final_result?: string
}

export interface AgentSessionLogsResponse {
  sessions: AgentSessionLogEntry[]
  total: number
  page: number
  per_page: number
  total_pages: number
}
