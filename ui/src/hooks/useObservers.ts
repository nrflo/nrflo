import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { launchObserver, listObservers, type ObserverLaunchRequest } from '@/api/observers'
import { useInteractiveSessionsStore } from '@/stores/interactiveSessionsStore'
import { useProjectStore } from '@/stores/projectStore'

export const observerKeys = {
  all: ['observers'] as const,
  list: (project: string) => [...observerKeys.all, project] as const,
}

export function useObservers() {
  const project = useProjectStore((s) => s.currentProject)
  return useQuery({
    queryKey: observerKeys.list(project),
    queryFn: listObservers,
    enabled: !!project,
  })
}

export function useLaunchObserver() {
  const qc = useQueryClient()
  const project = useProjectStore((s) => s.currentProject)
  const add = useInteractiveSessionsStore((s) => s.add)

  return useMutation({
    mutationFn: (req: ObserverLaunchRequest) => launchObserver(req),
    onSuccess: (data, req) => {
      add({
        sessionId: data.session_id,
        agentType: 'observer',
        scope: req.scope === 'global' || !req.project_id
          ? { type: 'project', projectId: project }
          : { type: 'project', projectId: req.project_id },
        workflow: req.workflow_id ?? 'observer',
        startedAt: Date.now(),
      })
      qc.invalidateQueries({ queryKey: observerKeys.all })
    },
  })
}
