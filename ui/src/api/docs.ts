import { apiGet } from './client'
import type { AgentManualResponse, DocKind } from '@/types/docs'

export async function getAgentManual(kind: DocKind): Promise<AgentManualResponse> {
  return apiGet<AgentManualResponse>(`/api/v1/docs/agent-manual?kind=${kind}`)
}
