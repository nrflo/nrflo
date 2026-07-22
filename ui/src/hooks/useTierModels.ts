import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { listTierModels, setTierChain, type SetTierChainEntry, type TierModel } from '@/api/tierModels'

export const tierModelKeys = {
  all: ['tier-models'] as const,
  list: () => [...tierModelKeys.all, 'list'] as const,
}

export function useTierModels() {
  return useQuery({ queryKey: tierModelKeys.list(), queryFn: listTierModels })
}

export function useSetTierChain() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ tier, entries }: { tier: number; entries: SetTierChainEntry[] }) => setTierChain(tier, entries),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: tierModelKeys.list() }),
  })
}

// resolveTierChain walks from `tier` down to the nearest populated tier
// (mirrors ResolveAgentChain's inheritance in
// be/internal/service/system_agent_chain.go), returning that tier's entries
// ordered by position, or [] if no tier 1..tier has any rows.
export function resolveTierChain(rows: TierModel[], tier: number | null | undefined): TierModel[] {
  if (!tier || tier < 1) return []
  for (let t = tier; t >= 1; t--) {
    const entries = rows.filter((r) => r.tier === t).sort((a, b) => a.position - b.position)
    if (entries.length > 0) return entries
  }
  return []
}
