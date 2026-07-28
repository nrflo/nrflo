import { apiGet, apiPost, apiDelete, ApiError } from './client'
import type {
  ApprovalDecision,
  ConsoleCatalog,
  ConsoleChatDetail,
  ConsoleChatInvokeRequest,
  ConsoleChatInvokeResponse,
  ConsoleChatListResponse,
  ConsoleChatMessagesResponse,
  ConsoleChatSummary,
  ConsoleChatToolsResponse,
  ConsoleSkill,
  ConsoleSkillsResponse,
  ConsoleTool,
  CreateConsoleChatRequest,
  CreateConsoleChatResponse,
} from '@/types/consoleChat'

export class TurnActiveError extends Error {
  constructor() {
    super('a turn is already running')
    this.name = 'TurnActiveError'
  }
}

export class QueueFullError extends Error {
  constructor() {
    super('the prompt queue is full')
    this.name = 'QueueFullError'
  }
}

export async function getConsoleCatalog(): Promise<ConsoleCatalog> {
  return apiGet<ConsoleCatalog>('/api/v1/console/catalog')
}

export async function listConsoleChats(): Promise<ConsoleChatSummary[]> {
  const resp = await apiGet<ConsoleChatListResponse>('/api/v1/console/chats')
  return resp.sessions ?? []
}

export async function getConsoleSkills(): Promise<ConsoleSkill[]> {
  const resp = await apiGet<ConsoleSkillsResponse>('/api/v1/console/skills')
  return resp.skills ?? []
}

export async function getConsoleChatTools(sid: string): Promise<ConsoleTool[]> {
  const resp = await apiGet<ConsoleChatToolsResponse>(`/api/v1/console/chats/${encodeURIComponent(sid)}/tools`)
  return resp.tools ?? []
}

export async function invokeConsoleChatTool(
  sid: string,
  body: ConsoleChatInvokeRequest
): Promise<ConsoleChatInvokeResponse> {
  try {
    return await apiPost<ConsoleChatInvokeResponse>(`/api/v1/console/chats/${encodeURIComponent(sid)}/invoke`, body)
  } catch (e) {
    if (e instanceof ApiError && e.status === 409) {
      throw new TurnActiveError()
    }
    throw e
  }
}

export async function getConsoleChat(sid: string): Promise<ConsoleChatDetail> {
  return apiGet<ConsoleChatDetail>(`/api/v1/console/chats/${encodeURIComponent(sid)}`)
}

export async function getConsoleChatMessages(sid: string): Promise<ConsoleChatMessagesResponse> {
  return apiGet<ConsoleChatMessagesResponse>(`/api/v1/console/chats/${encodeURIComponent(sid)}/messages`)
}

export async function createConsoleChat(req: CreateConsoleChatRequest): Promise<CreateConsoleChatResponse> {
  return apiPost<CreateConsoleChatResponse>('/api/v1/console/chats', req)
}

export interface SwitchConsoleChatModelRequest {
  engine?: string
  model: string
  reasoning_effort?: string
}

export interface SiblingChatResponse {
  sibling_session_id: string
}

// Model changes never mutate the running engine — they spawn a sibling chat
// seeded from the current session (chat_service_sibling.go) and return its id.
export async function switchConsoleChatModel(
  sid: string,
  req: SwitchConsoleChatModelRequest
): Promise<SiblingChatResponse> {
  return apiPost<SiblingChatResponse>(`/api/v1/console/chats/${encodeURIComponent(sid)}/switch-model`, req)
}

// Opens a t0-hands sibling pre-seeded with the origin chat's refinery digest
// as first-message context.
export async function openConsoleChatHandsSibling(sid: string): Promise<SiblingChatResponse> {
  return apiPost<SiblingChatResponse>(`/api/v1/console/chats/${encodeURIComponent(sid)}/hands-sibling`)
}

// Resolves to true when a turn was in flight and the server queued the text
// for delivery when it ends; 409 now means only a full queue.
export async function sendConsoleChatMessage(sid: string, text: string): Promise<boolean> {
  try {
    const resp = await apiPost<{ queued?: boolean }>(`/api/v1/console/chats/${encodeURIComponent(sid)}/messages`, {
      text,
    })
    return resp?.queued === true
  } catch (e) {
    if (e instanceof ApiError && e.status === 409) {
      throw new QueueFullError()
    }
    throw e
  }
}

// decision='answer' resolves an AskUserQuestion card; answer is required then.
export async function replyConsoleChatApproval(
  sid: string,
  aid: string,
  decision: ApprovalDecision,
  answer?: string
): Promise<void> {
  await apiPost<void>(`/api/v1/console/chats/${encodeURIComponent(sid)}/approvals/${encodeURIComponent(aid)}`, {
    decision,
    ...(answer !== undefined ? { answer } : {}),
  })
}

// Revoke one tool's allow_for_session grant so its next use asks again.
export async function revokeConsoleChatSessionApproval(sid: string, tool: string): Promise<void> {
  await apiDelete<void>(
    `/api/v1/console/chats/${encodeURIComponent(sid)}/session-approvals/${encodeURIComponent(tool)}`
  )
}

// Toggle the session-level YOLO (approval-gate) override: POST turns it on,
// DELETE clears the override (falls back to the resolved global default).
export async function setConsoleChatYolo(sid: string, on: boolean): Promise<void> {
  const path = `/api/v1/console/chats/${encodeURIComponent(sid)}/yolo`
  if (on) {
    await apiPost<void>(path)
  } else {
    await apiDelete<void>(path)
  }
}

export async function closeConsoleChat(sid: string): Promise<void> {
  await apiPost<void>(`/api/v1/console/chats/${encodeURIComponent(sid)}/close`)
}

// Interrupt cancels the active turn but keeps the engine + conversation
// alive. A 409 means the turn already ended — benign, swallowed here.
export async function interruptConsoleChat(sid: string): Promise<void> {
  try {
    await apiPost<void>(`/api/v1/console/chats/${encodeURIComponent(sid)}/interrupt`)
  } catch (e) {
    if (e instanceof ApiError && e.status === 409) return
    throw e
  }
}
