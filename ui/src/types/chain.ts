// Chain execution statuses
export type ChainStatus = 'pending' | 'running' | 'completed' | 'failed' | 'canceled'
export type ChainItemStatus = 'pending' | 'running' | 'completed' | 'failed' | 'skipped' | 'canceled'

// Chain execution — matches be/internal/model/chain.go ChainExecution
export interface ChainExecution {
  id: string
  project_id: string
  name: string
  status: ChainStatus
  workflow_name: string
  epic_ticket_id?: string
  created_by: string
  total_items: number
  completed_items: number
  created_at: string
  updated_at: string
  items?: ChainExecutionItem[]
}

// Chain item — matches be/internal/model/chain.go ChainExecutionItem (JSON output)
export interface ChainExecutionItem {
  id: string
  chain_id: string
  ticket_id: string
  ticket_title?: string
  position: number
  status: ChainItemStatus
  workflow_instance_id?: string
  total_tokens_used?: number
  started_at?: string
  ended_at?: string
}

// Request types — matches be/internal/types/chain_request.go
export interface ChainCreateRequest {
  name: string
  workflow_name: string
  epic_ticket_id?: string
  ticket_ids: string[]
}

export interface ChainUpdateRequest {
  name?: string
  ticket_ids?: string[]
}

export interface ChainAppendRequest {
  ticket_ids: string[]
}
