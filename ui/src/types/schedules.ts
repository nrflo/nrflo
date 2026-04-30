export type ScheduleRunStatus = 'pending' | 'triggered' | 'running' | 'failed'

export interface ScheduleRunWorkflow {
  workflow: string
  instance_id: string
  error?: string
}

export interface ScheduledTask {
  id: string
  project_id: string
  name: string
  description: string
  cron_expression: string
  workflows: string[]
  enabled: boolean
  last_triggered_at?: string
  next_run_at?: string
  created_at: string
  updated_at: string
}

export interface ScheduleRun {
  id: string
  scheduled_task_id: string
  project_id: string
  triggered_at: string
  status: ScheduleRunStatus
  workflows: ScheduleRunWorkflow[]
  error?: string
}

export interface ScheduledTaskCreateRequest {
  id?: string
  name: string
  description?: string
  cron_expression: string
  workflows: string[]
  enabled?: boolean
}

export interface ScheduledTaskUpdateRequest {
  name?: string
  description?: string
  cron_expression?: string
  workflows?: string[]
  enabled?: boolean
}
