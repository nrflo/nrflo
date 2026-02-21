import { apiGet } from './client'
import type { RunningAgentsResponse } from '@/types/agents'

export function fetchRunningAgents(): Promise<RunningAgentsResponse> {
  return apiGet<RunningAgentsResponse>('/api/v1/agents/running')
}
