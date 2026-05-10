import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  fetchAgentSessionLogs,
  fetchLiveAgentSessions,
  killAgentSession,
  type FetchAgentSessionLogsParams,
} from '@/api/agentSessionLogs'
import { useProjectStore } from '@/stores/projectStore'

export const agentSessionLogKeys = {
  all: ['agent-session-logs'] as const,
  lists: () => [...agentSessionLogKeys.all, 'list'] as const,
  list: (params?: FetchAgentSessionLogsParams) => [...agentSessionLogKeys.lists(), params] as const,
  live: (projectId: string) => [...agentSessionLogKeys.all, 'live', projectId] as const,
}

export function useAgentSessionLogs(params?: FetchAgentSessionLogsParams) {
  const project = useProjectStore((s) => s.currentProject)
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  return useQuery({
    queryKey: [...agentSessionLogKeys.list(params), project],
    queryFn: () => fetchAgentSessionLogs(params),
    enabled: projectsLoaded,
  })
}

export function useLiveAgentSessions() {
  const project = useProjectStore((s) => s.currentProject)
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  return useQuery({
    queryKey: agentSessionLogKeys.live(project),
    queryFn: fetchLiveAgentSessions,
    enabled: projectsLoaded,
    staleTime: Infinity,
    refetchOnWindowFocus: false,
  })
}

export function useKillAgentSession() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (sessionId: string) => killAgentSession(sessionId),
    onSuccess: () => {
      const project = useProjectStore.getState().currentProject
      queryClient.invalidateQueries({ queryKey: agentSessionLogKeys.live(project) })
      queryClient.invalidateQueries({ queryKey: agentSessionLogKeys.all })
    },
  })
}
