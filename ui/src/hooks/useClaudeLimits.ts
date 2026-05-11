import { useQuery } from '@tanstack/react-query'
import { getClaudeLimits } from '@/api/claudeLimits'
import type { ClaudeLimits } from '@/types/claudeLimits'

export const claudeLimitsKeys = {
  global: () => ['claude-limits', 'global'] as const,
}

export function useClaudeLimits() {
  return useQuery<ClaudeLimits>({
    queryKey: claudeLimitsKeys.global(),
    queryFn: getClaudeLimits,
    staleTime: 60_000,
  })
}
