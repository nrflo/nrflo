import { apiGet } from './client'
import type { LogsResponse } from '@/types/logs'

export async function getLogs(type: 'be' | 'fe'): Promise<LogsResponse> {
  return apiGet<LogsResponse>(`/api/v1/logs?type=${type}`)
}
