import { useQuery } from '@tanstack/react-query'
import { getAgentManual } from '@/api/docs'

export const docsKeys = {
  all: ['docs'] as const,
  agentManual: () => [...docsKeys.all, 'agent-manual'] as const,
}

export function useAgentManual() {
  return useQuery({
    queryKey: docsKeys.agentManual(),
    queryFn: getAgentManual,
    staleTime: 5 * 60 * 1000,
  })
}
