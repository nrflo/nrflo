// Mirrors be/internal/service/stepwise_read.go — keep field names/types in
// sync with the Go json tags.

export type StepStatus = 'pending' | 'active' | 'done' | 'rejected_retrying'

export interface StepProgressStep {
  step_id: string
  title: string
  status: StepStatus
  completed_at?: string
  session_id?: string
  summary?: string
  evidence_keys?: string[]
  rejections?: number
  rotated?: boolean
  rotation_allowed?: boolean
}

export interface StepCursorProgress {
  node_id: string
  revision: number
  current_index: number
  total: number
  current_step_id?: string
  done: boolean
  updated_at: string
  steps: StepProgressStep[]
}

export interface StepCursorsResponse {
  workflow_instance_id: string
  cursors: Record<string, StepCursorProgress>
}

// WS payload for the 'step.advanced' event.
export interface StepAdvancedEvent {
  workflow_instance_id: string
  node_id: string
  step_id: string
  step_index: number
  total: number
  rejected_count: number
  rotated: boolean
}
