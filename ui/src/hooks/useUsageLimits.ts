import { useQuery } from '@tanstack/react-query'
import { getUsageLimits } from '@/api/usageLimits'
import type { UsageLimits } from '@/types/usageLimits'

export function useUsageLimits() {
  return useQuery<UsageLimits>({
    queryKey: ['usage-limits'],
    queryFn: getUsageLimits,
    refetchInterval: 5 * 60 * 1000,
    staleTime: 2 * 60 * 1000,
  })
}
