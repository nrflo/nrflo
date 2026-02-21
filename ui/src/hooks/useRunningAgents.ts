import { useQuery } from '@tanstack/react-query'
import { fetchRunningAgents } from '@/api/agents'
import type { RunningAgentsResponse } from '@/types/agents'

export const runningAgentsKeys = {
  all: ['running-agents'] as const,
}

export function useRunningAgents() {
  return useQuery<RunningAgentsResponse>({
    queryKey: runningAgentsKeys.all,
    queryFn: fetchRunningAgents,
    refetchInterval: 30_000,
    staleTime: 5_000,
  })
}
