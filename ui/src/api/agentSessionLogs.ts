import { apiGet } from './client'
import type { AgentSessionLogsResponse } from '@/types/agentSessionLogs'

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
