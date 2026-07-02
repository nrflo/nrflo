import type { ScopeType, FindingSchema } from './workflow'

// Workflow-definition create/update request bodies (split out of workflow.ts
// for the 300-line file cap; re-exported from './workflow' so import paths
// stay stable).

export interface WorkflowDefCreateRequest {
  id: string
  description?: string
  scope_type?: ScopeType
  groups?: string[]
  close_ticket_on_complete?: boolean
  purge_on_completion?: boolean
  callable_as_subworkflow?: boolean
  next_workflow_on_success?: string
  observer_context?: string
  observer_provider?: string | null
  observer_model?: string | null
  finalize_success_command?: string
  finalize_success_script_id?: string
  finalize_failure_command?: string
  finalize_failure_script_id?: string
  pause_event_command?: string
  pause_event_script_id?: string
  finding_schemas?: FindingSchema[]
}

export interface WorkflowDefUpdateRequest {
  description?: string
  scope_type?: ScopeType
  groups?: string[]
  close_ticket_on_complete?: boolean
  purge_on_completion?: boolean
  callable_as_subworkflow?: boolean
  next_workflow_on_success?: string
  observer_context?: string
  observer_provider?: string | null
  observer_model?: string | null
  finalize_success_command?: string
  finalize_success_script_id?: string
  finalize_failure_command?: string
  finalize_failure_script_id?: string
  pause_event_command?: string
  pause_event_script_id?: string
  finding_schemas?: FindingSchema[]
}
