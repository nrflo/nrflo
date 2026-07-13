// TS mirrors of the Go plan contracts — see be/internal/model/plan.go,
// be/internal/service/plan_manifest.go, be/internal/service/plan_templates.go,
// be/internal/service/plan.go (PlanDraft), be/internal/types/plan.go.

export type PlanStatus = 'draft' | 'approved' | 'cancelled'
export type PlanAuthor = 'planner' | 'caller'

export interface PlanRevision {
  instance_id: string
  revision: number
  manifest: string
  hash: string
  author: PlanAuthor
  planner_session_id?: string
  created_at: string
}

export interface WorkflowPlan {
  instance_id: string
  status: PlanStatus
  latest_revision: number
  approved_revision: number
  goal: string
  materialized_revision: number
  created_at: string
  updated_at: string
}

export interface PlanNode {
  id: string
  template: string
  instructions: string
}

export interface PlanLayer {
  layer: number
  policy: string
  nodes: PlanNode[]
}

export interface PlanQuestion {
  id: string
  question: string
}

export interface PlanManifest {
  version: number
  goal: string
  layers: PlanLayer[]
  questions?: PlanQuestion[]
}

export interface PlanTemplate {
  id: string
  model: string
  execution_mode: string
  prompt: string
  description: string
  reasoning_effort?: string
}

export interface PlanDraft {
  head: WorkflowPlan | null
  manifest?: PlanManifest
  questions?: PlanQuestion[]
  templates: PlanTemplate[]
}

export interface PlanAnswer {
  question_id: string
  answer: string
}

export interface PlanReviseRequest {
  revision: number
  manifest?: unknown
  goal?: string
  feedback?: string
  answers?: PlanAnswer[]
}

export interface PlanApproveRequest {
  revision: number
}

export type DynamicWorkflowMode = 'approve' | 'auto'

export interface DynamicWorkflowRunRequest {
  instructions: string
  mode?: DynamicWorkflowMode
}

export interface DynamicWorkflowRunResponse {
  instance_id: string
  status: string
  session_id?: string
}
