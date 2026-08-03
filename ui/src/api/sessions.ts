import { apiGet } from './client'
import type { SessionListResponse, SessionFlowResponse, SessionStatsResponse } from '@/types/session'

export interface ListSessionsParams {
  limit?: number
}

function buildQuery(params?: ListSessionsParams): string {
  const searchParams = new URLSearchParams()
  if (params?.limit !== undefined) searchParams.set('limit', String(params.limit))
  const query = searchParams.toString()
  return query ? `?${query}` : ''
}

export function listSessions(params?: ListSessionsParams): Promise<SessionListResponse> {
  return apiGet<SessionListResponse>(`/api/v1/sessions${buildQuery(params)}`)
}

// Global scope lists sessions across all projects; the endpoint isn't
// project-scoped so the X-Project header the client always attaches is
// simply ignored server-side, same as GET /api/v1/settings.
export function listGlobalSessions(params?: ListSessionsParams): Promise<SessionListResponse> {
  return apiGet<SessionListResponse>(`/api/v1/sessions/global${buildQuery(params)}`)
}

export function getSessionFlow(sid: string): Promise<SessionFlowResponse> {
  return apiGet<SessionFlowResponse>(`/api/v1/sessions/${encodeURIComponent(sid)}/flow`)
}

export function getSessionStats(sid: string): Promise<SessionStatsResponse> {
  return apiGet<SessionStatsResponse>(`/api/v1/sessions/${encodeURIComponent(sid)}/stats`)
}
