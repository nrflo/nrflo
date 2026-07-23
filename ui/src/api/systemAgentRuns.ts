import { apiGet } from './client'
import type { SystemAgentRunsResponse } from '@/types/systemAgentRuns'

export interface ListSystemAgentRunsParams {
  limit?: number
  since?: string
}

export async function listSystemAgentRuns(
  params?: ListSystemAgentRunsParams
): Promise<SystemAgentRunsResponse> {
  const searchParams = new URLSearchParams()
  if (params?.limit !== undefined) searchParams.set('limit', String(params.limit))
  if (params?.since) searchParams.set('since', params.since)
  const query = searchParams.toString()
  return apiGet<SystemAgentRunsResponse>(`/api/v1/system-agent-runs${query ? `?${query}` : ''}`)
}
