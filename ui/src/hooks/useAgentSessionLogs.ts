import { useQuery } from '@tanstack/react-query'
import { fetchAgentSessionLogs, type FetchAgentSessionLogsParams } from '@/api/agentSessionLogs'
import { useProjectStore } from '@/stores/projectStore'

export const agentSessionLogKeys = {
  all: ['agent-session-logs'] as const,
  lists: () => [...agentSessionLogKeys.all, 'list'] as const,
  list: (params?: FetchAgentSessionLogsParams) => [...agentSessionLogKeys.lists(), params] as const,
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
