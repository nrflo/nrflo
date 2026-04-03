import { apiGet } from './client'
import type { LogsResponse } from '@/types/logs'

export async function getLogs(type: 'be', filter?: string): Promise<LogsResponse> {
  let url = `/api/v1/logs?type=${type}`
  if (filter) url += `&filter=${encodeURIComponent(filter)}`
  return apiGet<LogsResponse>(url)
}
