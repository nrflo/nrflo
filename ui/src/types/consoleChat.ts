import type { AgentSessionStatus, MessageWithTime } from './workflow'

export interface ConsoleChatSummary {
  session_id: string
  engine: string
  model: string
  project_id: string
  status: AgentSessionStatus
  started_at: string
  ended_at?: string
  context_left?: number
  live: boolean
}

export interface PendingApproval {
  approval_id: string
  kind: string
  command: string
  cwd: string
  reason: string
}

// turn/work_dir/pending_approvals are only present when live=true — a chat
// whose engine is gone (e.g. a hard server kill) omits them rather than
// fabricating them (be/internal/api/handlers_console_chat_list.go).
export interface ConsoleChatDetail extends ConsoleChatSummary {
  turn?: 'idle' | 'running'
  work_dir?: string
  pending_approvals?: PendingApproval[]
}

export interface ConsoleChatListResponse {
  sessions: ConsoleChatSummary[]
}

export interface CreateConsoleChatRequest {
  engine: string
  model: string
}

export interface CreateConsoleChatResponse {
  session_id: string
  engine: string
  model: string
  status: AgentSessionStatus
}

export interface ConsoleChatMessagesResponse {
  session_id: string
  messages: MessageWithTime[]
  total: number
}

export type ApprovalDecision = 'allow' | 'deny'

// WS session-channel payload shapes (be/internal/console/chat_events.go)
export interface ConsoleChatDeltaPayload {
  item_id: string
  text: string
}

export interface ConsoleChatThinkingPayload {
  item_id?: string
  text: string
}

export interface ConsoleChatTurnPayload {
  state: 'idle' | 'running'
}

export type ConsoleChatApprovalRequestPayload = PendingApproval

export interface ConsoleChatApprovalResolvedPayload {
  approval_id: string
  decision: ApprovalDecision
  reason?: string
}

export interface ConsoleChatErrorPayload {
  text: string
  is_error: boolean
}
