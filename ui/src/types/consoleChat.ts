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
  // Console profile the chat was created from (e.g. 't0-decider',
  // 't0-hands'); omitted for chats created via the manual/Custom path.
  profile?: string
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
  // Tools auto-allowed by allow_for_session decisions (claude/api engines'
  // server-side allowlist; codex resolves acceptForSession natively and
  // reports none). Revocable via DELETE .../session-approvals/{tool}.
  session_approvals?: string[]
  cost_estimate?: number
}

export interface ConsoleChatListResponse {
  sessions: ConsoleChatSummary[]
}

export interface CreateConsoleChatRequest {
  engine: string
  model: string
  // Optional create-time effort override; must be in the model's
  // supported_efforts.
  reasoning_effort?: string
  // Optional injectable system-template override; empty/omitted preserves
  // today's mode-default behavior.
  system_template_id?: string
  // Optional console profile (e.g. 't0-decider', 't0-hands'); when set, the
  // server resolves engine/model/effort/system-template/tool-catalogue
  // defaults from the profile registry.
  profile?: string
}

export interface CreateConsoleChatResponse {
  session_id: string
  engine: string
  model: string
  status: AgentSessionStatus
  profile?: string
}

export interface ConsoleChatMessagesResponse {
  session_id: string
  messages: MessageWithTime[]
  total: number
}

// allow_for_session remembers the tool for the rest of the chat: codex maps
// it natively (acceptForSession); the claude engine keeps a server-side
// per-tool allowlist. Resolved WS pushes normalize it back to 'allow'.
export type ApprovalDecision = 'allow' | 'allow_for_session' | 'deny'

// GET /console/catalog — server-owned engine/model discovery + live
// resumable chats (be/internal/types/console.go). The same source the
// native TUI picker uses; `models` is null when an engine has no registry
// rows (Go nil slice).
export interface ConsoleModelOption {
  id: string
  display_name: string
  brand?: string
  provider?: string
  mapped_model?: string
  // Row's configured (default) effort; create-time overrides must come
  // from supported_efforts.
  reasoning_effort?: string
  supported_efforts?: string[]
}

export interface ConsoleEngineOption {
  id: string
  display_name: string
  kind: 'cli' | 'api'
  brand?: string
  enabled: boolean
  disabled_reason?: string
  requires_model: boolean
  models: ConsoleModelOption[] | null
}

export interface ConsoleSessionOption {
  session_id: string
  engine: string
  model?: string
  status: string
  started_at?: string
  context_left?: number
  profile?: string
}

// Named console profile presets (t0-decider / t0-hands) — picking one at
// create time prefills engine/model/effort/system-template from these
// defaults (NewChatForm.tsx) instead of the manual/Custom path.
export interface ConsoleProfileOption {
  name: string
  display_name: string
  description?: string
  default_engine: string
  default_model_id: string
  default_effort?: string
  context_budget_tokens: number
  refinery_default: boolean
  system_template_id?: string
}

export interface ConsoleCatalog {
  project_id: string
  engines: ConsoleEngineOption[]
  sessions: ConsoleSessionOption[]
  profiles: ConsoleProfileOption[]
}

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
  // Resolutions are pushed in the normalized allow/deny vocabulary — the
  // pump maps approve_for_session down to 'allow' (chat_events.go).
  decision: 'allow' | 'deny'
  reason?: string
}

export interface ConsoleChatErrorPayload {
  text: string
  is_error: boolean
}

// Full session-approved tool list, pushed whenever it changes
// (approve_for_session resolution or a revoke).
export interface ConsoleChatSessionApprovalsPayload {
  tools: string[]
}

// console_chat.sibling_opened session-channel push (chat_service_sibling.go)
// — fired on the ORIGIN session when a model-switch or hands-sibling spawn
// creates a sibling chat, so any other tab watching that session can also
// jump to the new one.
export interface ConsoleChatSiblingOpenedPayload {
  origin_session_id: string
  sibling_session_id: string
  reason: 'model_switch' | 'hands_sibling'
}

// session.cost_updated session-channel push (be/internal/spawner sessioncost
// broadcast) — debounced running-cost estimate for the session.
export interface ConsoleChatCostPayload {
  cost_estimate: number
  // false when the session's model has no seeded pricing — cost_estimate is
  // then 0 meaning "unknown", not "free", so the readout must be suppressed.
  pricing_known?: boolean
  tokens?: {
    input?: number
    output?: number
    cache_write?: number
    cache_read?: number
  }
}
