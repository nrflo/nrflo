export type ChannelKind = 'slack' | 'telegram' | 'script'

export type NotificationEventType =
  | 'orchestration.completed'
  | 'orchestration.failed'
  | 'agent.completed'
  | 'agent.context_saving'
  | 'agent.stall_restart'

export interface NotificationChannel {
  id: string
  project_id: string
  workflow_id: string
  name: string
  kind: ChannelKind
  enabled: boolean
  config: string
  event_types: NotificationEventType[]
  message_template?: string
  created_at: string
  updated_at: string
}

export interface NotificationVariablesResponse {
  variables: string[]
  defaults: {
    slack: string
    telegram: string
  }
}

export interface NotificationDelivery {
  id: number
  channel_id: string
  event_type: string
  status: string
  attempts: number
  last_error: string
  next_attempt_at: string | null
  created_at: string
}
