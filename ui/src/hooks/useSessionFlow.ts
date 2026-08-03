import { useQuery } from '@tanstack/react-query'
import { useProjectStore } from '@/stores/projectStore'
import { getSessionFlow, getSessionStats } from '@/api/sessions'

export const sessionFlowKeys = {
  all: ['session-flow'] as const,
  flow: (sid: string) => [...sessionFlowKeys.all, sid] as const,
  stats: (sid: string) => ['session-stats', sid] as const,
}

export function useSessionFlow(sid: string | undefined) {
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  return useQuery({
    queryKey: sessionFlowKeys.flow(sid ?? ''),
    queryFn: () => getSessionFlow(sid!),
    enabled: projectsLoaded && !!sid,
    staleTime: 2000,
  })
}

export function useSessionStats(sid: string | undefined) {
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  return useQuery({
    queryKey: sessionFlowKeys.stats(sid ?? ''),
    queryFn: () => getSessionStats(sid!),
    enabled: projectsLoaded && !!sid,
    staleTime: 2000,
  })
}
