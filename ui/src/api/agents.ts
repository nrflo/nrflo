import { apiGet } from './client'
import type { RunningAgentsResponse } from '@/types/agents'

export function fetchRunningAgents(): Promise<RunningAgentsResponse> {
  return apiGet<RunningAgentsResponse>('/api/v1/agents/running')
}

export async function fetchSessionPrompt(sessionId: string): Promise<{ prompt_context: string }> {
  const response = await fetch(`/api/v1/sessions/${sessionId}/prompt`, { method: 'GET' })
  if (response.status === 204) return { prompt_context: '' }
  if (!response.ok) throw new Error(`Failed to fetch prompt: ${response.status}`)
  return response.json()
}
