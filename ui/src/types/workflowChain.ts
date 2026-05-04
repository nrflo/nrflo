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
