import { apiGet, apiPost, ApiError } from './client'
import type {
  ApprovalDecision,
  ConsoleChatDetail,
  ConsoleChatListResponse,
  ConsoleChatMessagesResponse,
  ConsoleChatSummary,
  CreateConsoleChatRequest,
  CreateConsoleChatResponse,
} from '@/types/consoleChat'

export class TurnActiveError extends Error {
  constructor() {
    super('a turn is already running')
    this.name = 'TurnActiveError'
  }
}

export async function listConsoleChats(): Promise<ConsoleChatSummary[]> {
  const resp = await apiGet<ConsoleChatListResponse>('/api/v1/console/chats')
  return resp.sessions ?? []
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

export async function sendConsoleChatMessage(sid: string, text: string): Promise<void> {
  try {
    await apiPost<void>(`/api/v1/console/chats/${encodeURIComponent(sid)}/messages`, { text })
  } catch (e) {
    if (e instanceof ApiError && e.status === 409) {
      throw new TurnActiveError()
    }
    throw e
  }
}

export async function replyConsoleChatApproval(sid: string, aid: string, decision: ApprovalDecision): Promise<void> {
  await apiPost<void>(`/api/v1/console/chats/${encodeURIComponent(sid)}/approvals/${encodeURIComponent(aid)}`, { decision })
}

export async function closeConsoleChat(sid: string): Promise<void> {
  await apiPost<void>(`/api/v1/console/chats/${encodeURIComponent(sid)}/close`)
}
