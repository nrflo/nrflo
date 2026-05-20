import { useQuery } from '@tanstack/react-query'
import { getAgentManual } from '@/api/docs'
import type { DocKind } from '@/types/docs'

export const docsKeys = {
  all: ['docs'] as const,
  agentManual: (kind: DocKind) => [...docsKeys.all, 'agent-manual', kind] as const,
}

export function useAgentManual(kind: DocKind) {
  return useQuery({
    queryKey: docsKeys.agentManual(kind),
    queryFn: () => getAgentManual(kind),
    staleTime: 5 * 60 * 1000,
  })
}
