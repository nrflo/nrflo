import { apiGet, apiPost } from './client'
import type { AgentSession } from '@/types/workflow'

export type ObserverScope = 'workflow' | 'project' | 'global'

export interface ObserverLaunchRequest {
  scope: ObserverScope
  project_id?: string
  workflow_id?: string
}

export interface ObserverLaunchResponse {
  session_id: string
}

export interface ObserverListResponse {
  sessions: AgentSession[]
  count: number
}

export async function launchObserver(req: ObserverLaunchRequest): Promise<ObserverLaunchResponse> {
  return apiPost<ObserverLaunchResponse>('/api/v1/observers', req)
}

export async function listObservers(): Promise<ObserverListResponse> {
  return apiGet<ObserverListResponse>('/api/v1/observers')
}
