// Agent definition create/update request bodies (split out of workflow.ts
// for the 300-line file cap; re-exported from './workflow' so import paths
// stay stable).

// Stepwise step schema, transcribed 1:1 from be/internal/model/agent_step.go.
export interface RequiredFinding {
  key: string
  schema: string
}

export interface PathOverlap {
  left: string[]
  right: string[]
}

export interface StepDefinition {
  step_id: string
  title: string
  instruction: string
  required_findings?: RequiredFinding[]
  checks?: string[]
  rotation_allowed?: boolean
  path_overlap?: PathOverlap
}

// steps read/write asymmetry: GET returns agent_definitions.steps as a raw
// JSON string (model.Steps is *string) — parse before use. POST/PATCH send
// steps as a structured StepDefinition[] — never a string.
export type PromptMode = 'full' | 'stepwise'

export interface AgentDef {
  id: string
  project_id: string
  workflow_id: string
  layer: number
  model: string
  // null/undefined = untiered; a non-empty `model` always wins over the tier chain.
  tier?: number | null
  timeout: number
  prompt: string
  restart_threshold?: number
  max_fail_restarts?: number
  tag?: string
  low_consumption_model?: string
  execution_mode: 'cli_interactive' | 'api' | 'script'
  tools: string
  native_tools: string
  sandbox: '' | 'read-only' | 'workspace-write' | 'danger-full-access'
  api_max_iterations?: number
  api_max_tokens?: number
  python_script_id?: string
  validation_commands?: string
  consultant?: boolean
  node_role?: 'static' | 'planner' | 'fanout_template'
  description?: string
  reasoning_effort?: string | null
  system_template_id?: string
  prompt_mode?: PromptMode
  steps?: string
  created_at: string
  updated_at: string
}

export interface AgentDefCreateRequest {
  id: string
  layer: number
  model?: string
  // null/undefined = untiered; a non-empty `model` always wins over the tier chain.
  tier?: number | null
  timeout?: number
  prompt?: string
  restart_threshold?: number
  max_fail_restarts?: number
  tag?: string
  low_consumption_model?: string
  execution_mode?: 'cli_interactive' | 'api' | 'script'
  tools?: string
  native_tools?: string
  sandbox?: '' | 'read-only' | 'workspace-write' | 'danger-full-access'
  api_max_iterations?: number
  api_max_tokens?: number
  python_script_id?: string
  validation_commands?: string[]
  consultant?: boolean
  node_role?: 'static' | 'planner' | 'fanout_template'
  description?: string
  reasoning_effort?: string | null
  system_template_id?: string
  prompt_mode?: PromptMode
  steps?: StepDefinition[]
}

export interface AgentDefUpdateRequest {
  layer?: number
  model?: string
  // null/undefined = untiered; a non-empty `model` always wins over the tier chain.
  tier?: number | null
  timeout?: number
  prompt?: string
  restart_threshold?: number
  max_fail_restarts?: number
  tag?: string
  low_consumption_model?: string
  execution_mode?: 'cli_interactive' | 'api' | 'script'
  tools?: string
  native_tools?: string
  sandbox?: '' | 'read-only' | 'workspace-write' | 'danger-full-access'
  api_max_iterations?: number
  api_max_tokens?: number
  python_script_id?: string
  validation_commands?: string[]
  consultant?: boolean
  node_role?: 'static' | 'planner' | 'fanout_template'
  description?: string
  reasoning_effort?: string | null
  system_template_id?: string
  prompt_mode?: PromptMode
  steps?: StepDefinition[]
}
