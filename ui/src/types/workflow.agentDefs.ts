// Agent definition create/update request bodies (split out of workflow.ts
// for the 300-line file cap; re-exported from './workflow' so import paths
// stay stable).

export interface AgentDef {
  id: string
  project_id: string
  workflow_id: string
  layer: number
  model: string
  timeout: number
  prompt: string
  restart_threshold?: number
  max_fail_restarts?: number
  tag?: string
  low_consumption_model?: string
  execution_mode: 'cli_interactive' | 'api' | 'script'
  tools: string
  api_max_iterations?: number
  api_max_tokens?: number
  python_script_id?: string
  validation_commands?: string
  consultant?: boolean
  node_role?: 'static' | 'planner' | 'fanout_template'
  description?: string
  created_at: string
  updated_at: string
}

export interface AgentDefCreateRequest {
  id: string
  layer: number
  model?: string
  timeout?: number
  prompt?: string
  restart_threshold?: number
  max_fail_restarts?: number
  tag?: string
  low_consumption_model?: string
  execution_mode?: 'cli_interactive' | 'api' | 'script'
  tools?: string
  api_max_iterations?: number
  api_max_tokens?: number
  python_script_id?: string
  validation_commands?: string[]
  consultant?: boolean
  node_role?: 'static' | 'planner' | 'fanout_template'
  description?: string
}

export interface AgentDefUpdateRequest {
  layer?: number
  model?: string
  timeout?: number
  prompt?: string
  restart_threshold?: number
  max_fail_restarts?: number
  tag?: string
  low_consumption_model?: string
  execution_mode?: 'cli_interactive' | 'api' | 'script'
  tools?: string
  api_max_iterations?: number
  api_max_tokens?: number
  python_script_id?: string
  validation_commands?: string[]
  consultant?: boolean
  node_role?: 'static' | 'planner' | 'fanout_template'
  description?: string
}
