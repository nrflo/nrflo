import { apiGet } from './client'
import type { UsageLimits } from '@/types/usageLimits'

export function getUsageLimits(): Promise<UsageLimits> {
  return apiGet<UsageLimits>('/api/v1/usage-limits')
}
