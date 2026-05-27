import { apiGet } from './client'
import type { AvailableTool } from '@/types/availableTool'

/** List the tools an agent can be granted (builtins + project python tools). */
export async function listAvailableTools(): Promise<AvailableTool[]> {
  return apiGet<AvailableTool[]>('/api/v1/available-tools')
}
