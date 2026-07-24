import type { InputArtifactRef } from '@/types/artifact'
import type { PlanQuestion, PlanStatus } from '@/types/plan'

export type PhaseStatus = 'pending' | 'in_progress' | 'completed' | 'skipped' | 'error' | 'rate_limited'
export type PhaseResult = 'pass' | 'fail' | 'skipped' | null

export interface PhaseState {
  status: PhaseStatus
  result?: PhaseResult | string
  started_at?: string
  ended_at?: string
  error?: string
  rate_limit_until_ts?: string
  rate_limit_retry_count?: number
}

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
}

export interface CompletedAgentRow extends AgentHistoryEntry {
  workflow_label: string
}

// Findings structure: agent_type -> findings (field -> value)
export type WorkflowFindings = Record<string, Record<string, unknown>>

export interface CallbackPlanStep {
  layer: number
  mode: 'agents' | 'whole_layer'
  agents?: string[]
  instructions?: string
}

export interface CallbackRequest {
  from_agent: string
  mode: 'level' | 'agent' | 'chain'
  [k: string]: unknown
}

export interface CallbackInfo {
  level?: number
  instructions?: string
  from_layer?: number
  from_agent?: string
  to_layer?: number
  resume_layer?: number
  plan?: CallbackPlanStep[]
  requests?: CallbackRequest[]
}

export type ScopeType = 'ticket' | 'project'

export interface FinalizeResult {
  slot: 'success' | 'failure'
  kind: 'command' | 'script'
  target: string
  exit_code: number
  status: 'ok' | 'failed' | 'timeout'
  output_tail?: string
  timestamp?: string
}

export interface PauseEvent {
  kind: 'command' | 'script'
  target: string
  exit_code: number
  status: 'ok' | 'failed' | 'timeout'
  output_tail?: string
}

export interface PauseResult {
  paused_after_layer: number
  resume_layer: number
  event?: PauseEvent
  timestamp?: string
}

// Instance status set (workflow_instances.status). The four plan-boundary
// statuses (DYNWF-5) are mutually exclusive with 'waiting' — the engine is
// parked on the plan lifecycle, not the pause_after machinery.
export type WorkflowInstanceStatus =
  | 'active'
  | 'completed'
  | 'failed'
  | 'project_completed'
  | 'waiting'
  | 'planning'
  | 'plan_ready'
  | 'waiting_input'
  | 'waiting_approval'

export const PLAN_SUSPENDED_STATUSES: readonly WorkflowInstanceStatus[] = [
  'planning',
  'plan_ready',
  'waiting_input',
  'waiting_approval',
]

// Plan lifecycle read model emitted by buildV4State — see
// be/internal/service/workflow_v4_state.go.
export interface PlanBlock {
  status: PlanStatus
  latest_revision: number
  approved_revision: number
  materialized_revision: number
  questions?: PlanQuestion[]
}

export interface WorkflowState {
  workflow?: string
  instance_id?: string
  version?: number
  scope_type?: ScopeType
  worktree_path?: string
  branch_name?: string
  current_phase?: string
  status?: WorkflowInstanceStatus
  plan?: PlanBlock
  initialized_at?: string
  completed_at?: string
  total_duration_sec?: number
  total_tokens_used?: number
  workflow_final_result?: string
  finalize_result?: FinalizeResult
  pause_result?: PauseResult
  phases?: Record<string, PhaseState>
  phase_order?: string[]
  phase_layers?: Record<string, number>
  active_agents?: Record<string, ActiveAgentV4>  // key is "node_id" or "node_id:model_id"
  findings?: WorkflowFindings
  workflow_findings?: Record<string, unknown>
  callback?: CallbackInfo
  agent_history?: AgentHistoryEntry[]
  endless_loop?: boolean
  stop_endless_loop_after_iteration?: boolean
  layer_policies?: Record<number, LayerPassPolicy>
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
  nudge_count?: number
  ancestor_session_id?: string
  started_at?: string
  ended_at?: string
  created_at: string
  updated_at: string
}

export type MessageCategory = 'text' | 'tool' | 'subagent' | 'skill' | 'user_input' | 'error' | 'result' | 'validation' | 'thinking'

export interface MessageWithTime {
  content: string
  category: MessageCategory
  created_at: string
  payload?: Record<string, unknown>
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

/** Pass policy for a workflow layer: any | all | quorum:N | percent:P */
export type LayerPassPolicy = string

export interface WorkflowLayerPolicy {
  project_id: string
  workflow_id: string
  layer: number
  pass_policy: LayerPassPolicy
}

export interface PhaseDef {
  id: string
  agent: string
  layer: number
}

export interface LayerPoliciesResponse {
  layer_policies: Record<number, LayerPassPolicy>
  layer_pause: Record<number, boolean>
}

/** WorkflowDef as returned by the list endpoint (no id/project_id/timestamps) */
/** Workflow-scoped validation contract for a finding key. `schema` is a JSON
 *  Schema (Draft 2020); `example` is a known-good value shown to agents on a
 *  validation failure. */
export interface FindingSchema {
  key: string
  schema: unknown
  example: unknown
}

export interface WorkflowDefSummary {
  description: string
  scope_type?: ScopeType
  is_global?: boolean // true → reserved global namespace; run-only from a project
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
  phases: PhaseDef[]
  layer_policies?: Record<number, LayerPassPolicy>
}

/** Full WorkflowDef as returned by get/create endpoints */
export interface WorkflowDef {
  id: string
  project_id: string
  description: string
  scope_type?: ScopeType
  is_global?: boolean // true → reserved global namespace; run-only from a project
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
  phases: PhaseDef[]
  layer_policies?: Record<number, LayerPassPolicy>
  created_at: string
  updated_at: string
}

export type { WorkflowDefCreateRequest, WorkflowDefUpdateRequest } from './workflow.defs'
export type { AgentDef, AgentDefCreateRequest, AgentDefUpdateRequest, StepDefinition, RequiredFinding, PathOverlap, PromptMode } from './workflow.agentDefs'

export interface ContinueWorkflowRequest {
  workflow?: string
  instance_id?: string
  instructions?: string
}

export interface FailWorkflowRequest {
  workflow?: string
  instance_id?: string
  reason: string
}

// Orchestration types

export interface RunWorkflowRequest {
  workflow: string
  instructions?: string
  interactive?: boolean
  plan_mode?: boolean
  force?: boolean
  input_artifacts?: { upload_id: string; name?: string }[]
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
  endless_loop?: boolean
  input_artifacts?: InputArtifactRef[]
}
