export type Status = 'open' | 'in_progress' | 'closed'
export type IssueType = 'bug' | 'feature' | 'task' | 'epic'

export interface Ticket {
  id: string
  title: string
  description: string | null
  status: Status
  priority: number
  issue_type: IssueType
  parent_ticket_id?: string | null
  created_at: string
  updated_at: string
  closed_at: string | null
  created_by: string
  close_reason: string | null
}

export interface WorkflowProgress {
  workflow_name: string
  current_phase: string
  completed_phases: number
  total_phases: number
  status: string
}

export interface PendingTicket extends Ticket {
  is_blocked: boolean
  blocked_by?: string[]
  workflow_progress?: WorkflowProgress
}

export interface Dependency {
  issue_id: string
  depends_on_id: string
  type: string
  created_at: string
  created_by: string
  depends_on_title?: string
  issue_title?: string
}

export interface TicketWithDeps extends Ticket {
  blockers: Dependency[]
  blocks: Dependency[]
  is_blocked?: boolean
  blocked_by?: string[]
  children?: Ticket[]
  parent_ticket?: Ticket | null
  siblings?: Ticket[]
}

export interface CreateTicketRequest {
  id?: string
  title: string
  description?: string
  priority?: number
  issue_type?: IssueType
  created_by: string
  parent_ticket_id?: string
}

export interface UpdateTicketRequest {
  title?: string
  description?: string
  status?: Status
  priority?: number
  issue_type?: IssueType
  parent_ticket_id?: string
}

export interface TicketListResponse {
  tickets: PendingTicket[]
  total_count: number
  page: number
  per_page: number
  total_pages: number
}

export interface SearchResponse {
  tickets: PendingTicket[]
  query: string
}

export interface DailyStats {
  date: string
  tickets_created: number
  tickets_closed: number
  tokens_spent: number
  agent_time_sec: number
}

export interface StatusResponse {
  counts: {
    open: number
    in_progress: number
    closed: number
    blocked: number
    total: number
  }
  ready_count: number
  pending_tickets: PendingTicket[]
  recent_closed: Ticket[]
}
