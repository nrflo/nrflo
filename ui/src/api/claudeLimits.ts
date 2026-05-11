import { apiGet } from './client'
import type { ClaudeLimits } from '@/types/claudeLimits'

export function getClaudeLimits(): Promise<ClaudeLimits> {
  return apiGet<ClaudeLimits>('/api/v1/claude-limits')
}
