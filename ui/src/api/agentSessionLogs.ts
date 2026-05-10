import { apiGet, apiPost } from './client'
import type { AgentSessionLogsResponse, LiveAgentSessionsResponse } from '@/types/agentSessionLogs'

export interface FetchAgentSessionLogsParams {
  page?: number
  perPage?: number
}

export async function fetchAgentSessionLogs(
  params?: FetchAgentSessionLogsParams
): Promise<AgentSessionLogsResponse> {
  const searchParams = new URLSearchParams()
  if (params?.page) searchParams.set('page', String(params.page))
  if (params?.perPage) searchParams.set('per_page', String(params.perPage))
  const query = searchParams.toString()
  return apiGet<AgentSessionLogsResponse>(`/api/v1/agent-session-logs${query ? `?${query}` : ''}`)
}

export async function fetchLiveAgentSessions(): Promise<LiveAgentSessionsResponse> {
  return apiGet<LiveAgentSessionsResponse>('/api/v1/agent-session-logs/live')
}

export async function killAgentSession(id: string): Promise<void> {
  return apiPost<void>(`/api/v1/agent-sessions/${id}/kill`)
}
