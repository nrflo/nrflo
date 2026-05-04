export interface WorkflowChain {
  id: string
  project_id: string
  name: string
  description: string
  created_at: string
  updated_at: string
}

export interface WorkflowChainStep {
  id: string
  project_id: string
  chain_id: string
  position: number
  workflow_name: string
  scope_type: 'project' | 'ticket'
  base_instructions: string
  require_ticket_handoff: boolean
  created_at: string
  updated_at: string
}

export interface WorkflowChainWithSteps extends WorkflowChain {
  steps: WorkflowChainStep[]
}

export interface WorkflowChainCreateRequest {
  id?: string
  name: string
  description?: string
  steps: WorkflowChainStepRequest[]
}

export interface WorkflowChainUpdateRequest {
  name?: string
  description?: string
}

export interface WorkflowChainStepRequest {
  id?: string
  workflow_name: string
  scope_type: 'project' | 'ticket'
  base_instructions?: string
  require_ticket_handoff?: boolean
}

export interface WorkflowChainStepUpdateRequest {
  workflow_name?: string
  scope_type?: 'project' | 'ticket'
  base_instructions?: string
  require_ticket_handoff?: boolean
}

export interface ReorderStepsRequest {
  ordered_step_ids: string[]
}

export interface WorkflowChainRun {
  id: string
  project_id: string
  chain_id: string
  status: 'pending' | 'running' | 'completed' | 'failed' | 'canceled'
  initial_instructions: string
  triggered_by: string
  current_position: number
  started_at: string | null
  completed_at: string | null
  created_at: string
  updated_at: string
}

export interface WorkflowChainRunStep {
  id: string
  chain_run_id: string
  position: number
  workflow_name: string
  scope_type: 'project' | 'ticket'
  instructions_used: string
  status: 'pending' | 'running' | 'completed' | 'failed' | 'skipped' | 'canceled'
  created_at: string
  updated_at: string
}

export interface WorkflowChainRunDetail extends WorkflowChainRun {
  steps: WorkflowChainRunStep[]
}

export interface StartChainRunRequest {
  instructions?: string
  triggered_by?: string
}
