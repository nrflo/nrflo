// Pure stream layer for console-chat transcripts — no React. Combines
// persisted history rows (GET /console/chats/{sid}/messages) with ephemeral
// WS session-channel pushes. See ui/CLAUDE.md feature index.
import type { MessageWithTime } from '@/types/workflow'
import type { WSEvent } from '@/hooks/useWebSocket'
import type {
  ConsoleChatApprovalRequestPayload,
  ConsoleChatApprovalResolvedPayload,
  ConsoleChatDeltaPayload,
  ConsoleChatErrorPayload,
  ConsoleChatThinkingPayload,
  ConsoleChatTurnPayload,
  PendingApproval,
} from '@/types/consoleChat'

export interface ResolvedApproval {
  approval_id: string
  decision: 'allow' | 'deny'
  reason?: string
}

export interface SessionStreamState {
  deltas: Map<string, string>
  thinking: string[]
  turn: 'idle' | 'running'
  // Set once a live console_chat.turn event has arrived — lets a consumer
  // prefer the reload-restored detail snapshot's turn until the live channel
  // has actually said anything, without reading a ref during render.
  turnLive: boolean
  // Every approval request seen this session, in request order — resolution
  // does NOT remove an entry: console_chat.approval_resolved carries only
  // {approval_id, decision, reason}, not the original kind/command/cwd, so
  // the card needs the request row kept around to render its terminal state.
  approvals: PendingApproval[]
  resolvedApprovals: Map<string, ResolvedApproval>
  contextLeft?: number
  errors: ConsoleChatErrorPayload[]
}

export function initialSessionStreamState(): SessionStreamState {
  return {
    deltas: new Map(),
    thinking: [],
    turn: 'idle',
    turnLive: false,
    approvals: [],
    resolvedApprovals: new Map(),
    errors: [],
  }
}

// sessionEventReducer folds one WS session-channel event into stream state.
// Non-console_chat event types (e.g. messages.updated, which is handled
// upstream by cache invalidation) pass through unchanged.
export function sessionEventReducer(state: SessionStreamState, event: WSEvent): SessionStreamState {
  const data = event.data as unknown

  switch (event.type) {
    case 'console_chat.delta': {
      const { item_id, text } = data as ConsoleChatDeltaPayload
      const deltas = new Map(state.deltas)
      deltas.set(item_id, (deltas.get(item_id) ?? '') + text)
      return { ...state, deltas }
    }
    case 'console_chat.thinking': {
      const { text } = data as ConsoleChatThinkingPayload
      return { ...state, thinking: [...state.thinking, text] }
    }
    case 'console_chat.turn': {
      const { state: turn } = data as ConsoleChatTurnPayload
      return { ...state, turn, turnLive: true }
    }
    case 'console_chat.approval_request': {
      const approval = data as ConsoleChatApprovalRequestPayload
      if (state.approvals.some((a) => a.approval_id === approval.approval_id)) return state
      return { ...state, approvals: [...state.approvals, approval] }
    }
    case 'console_chat.approval_resolved': {
      const resolved = data as ConsoleChatApprovalResolvedPayload
      const resolvedApprovals = new Map(state.resolvedApprovals)
      resolvedApprovals.set(resolved.approval_id, resolved)
      return { ...state, resolvedApprovals }
    }
    case 'console_chat.error': {
      const err = data as ConsoleChatErrorPayload
      return { ...state, errors: [...state.errors, err] }
    }
    case 'agent.context_updated': {
      // Pushed on the session channel too (pumpChatEvents, EventTokenUsage) —
      // covers both engines; claude's context update otherwise only reaches
      // the socket path.
      const { context_left } = data as { context_left?: number }
      return context_left == null ? state : { ...state, contextLeft: context_left }
    }
    default:
      return state
  }
}

export type MergedTranscriptItem =
  | { kind: 'message'; message: MessageWithTime }
  | { kind: 'live'; itemId: string; text: string }

// mergeStream combines persisted rows with the live delta buffer, dropping
// any delta whose accumulated text is already covered by a persisted
// category='text' row — codex streams deltas and then the engine persists
// the completed item, so both arrive; this is the dedupe point.
export function mergeStream(persisted: MessageWithTime[], deltas: Map<string, string>): MergedTranscriptItem[] {
  const items: MergedTranscriptItem[] = persisted.map((message) => ({ kind: 'message', message }))
  for (const [itemId, text] of deltas) {
    if (!text || isCoveredByPersistedText(text, persisted)) continue
    items.push({ kind: 'live', itemId, text })
  }
  return items
}

// A delta buffer is covered only once a persisted text row holds the SAME text
// (whitespace-insensitive), which is what happens when the item finalizes: the
// engine persists the item's complete text, and the accumulated deltas equal it.
// Matching on containment instead would suppress a stream in progress — early on
// the buffer is a few characters, and those are a substring of almost any
// earlier assistant message, so the streaming bubble would stay invisible until
// the buffer grew unique.
function isCoveredByPersistedText(text: string, persisted: MessageWithTime[]): boolean {
  const needle = text.trim()
  return persisted.some((m) => m.category === 'text' && m.content.trim() === needle)
}

interface ToolPayload {
  tool_use_id?: string
  ended_at?: string
}

// The API returns payload as a JSON *string* on the wire (repo.MessageWithTime
// .Payload is a Go string) even though workflow.ts's shared MessageWithTime
// type declares it Record<string, unknown> — parse defensively here rather
// than changing that type (MessageTable depends on it).
function parseToolPayload(payload: MessageWithTime['payload']): ToolPayload | null {
  if (payload == null) return null
  const raw = payload as unknown
  if (typeof raw === 'string') {
    try {
      return JSON.parse(raw) as ToolPayload
    } catch {
      return null
    }
  }
  return raw as ToolPayload
}

const TOOL_INVOKE_CATEGORIES = new Set(['tool', 'skill', 'subagent'])

export interface ToolPair {
  invoke: MessageWithTime
  invokeIndex: number
  result?: MessageWithTime
  resultIndex?: number
  toolUseId: string
  durationMs?: number
  running: boolean
}

// pairToolMessages pairs an invoke row (category tool/skill/subagent,
// payload.tool_use_id set by output_tool_span.go) with the result row
// immediately following it in the message stream — invoke and result are
// always emitted back-to-back (TrackToolInvoke then TrackMessage), and only
// the invoke row carries tool_use_id, so adjacency is the pairing signal.
// Duration comes from the invoke row's own payload.ended_at (stamped by
// CloseToolSpan when the tool returns), matching the trace's span math.
export function pairToolMessages(messages: MessageWithTime[]): ToolPair[] {
  const pairs: ToolPair[] = []

  for (let i = 0; i < messages.length; i++) {
    const invoke = messages[i]
    if (!TOOL_INVOKE_CATEGORIES.has(invoke.category)) continue

    const payload = parseToolPayload(invoke.payload)
    if (!payload?.tool_use_id) continue

    const next = messages[i + 1]
    const nextPayload = next ? parseToolPayload(next.payload) : null
    const isResultRow =
      !!next && (next.category === 'tool' || next.category === 'error') && !nextPayload?.tool_use_id

    const durationMs =
      payload.ended_at != null ? Date.parse(payload.ended_at) - Date.parse(invoke.created_at) : undefined

    pairs.push({
      invoke,
      invokeIndex: i,
      result: isResultRow ? next : undefined,
      resultIndex: isResultRow ? i + 1 : undefined,
      toolUseId: payload.tool_use_id,
      durationMs,
      running: payload.ended_at == null,
    })
  }

  return pairs
}
