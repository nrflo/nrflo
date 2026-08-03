import { useQuery } from '@tanstack/react-query'
import { useProjectStore } from '@/stores/projectStore'
import { listSessions, listGlobalSessions, type ListSessionsParams } from '@/api/sessions'

export type SessionsScope = 'project' | 'global'

export const sessionKeys = {
  all: ['sessions'] as const,
  list: (scope: SessionsScope, params: ListSessionsParams) =>
    [...sessionKeys.all, scope, params.limit ?? 0] as const,
}

export function useSessions(scope: SessionsScope, params: ListSessionsParams = {}) {
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  return useQuery({
    queryKey: sessionKeys.list(scope, params),
    queryFn: () => (scope === 'global' ? listGlobalSessions(params) : listSessions(params)),
    enabled: projectsLoaded,
    staleTime: 2000,
  })
}
