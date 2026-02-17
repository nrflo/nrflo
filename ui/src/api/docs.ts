import { apiGet } from './client'
import type { AgentManualResponse } from '@/types/docs'

export async function getAgentManual(): Promise<AgentManualResponse> {
  return apiGet<AgentManualResponse>('/api/v1/docs/agent-manual')
}
