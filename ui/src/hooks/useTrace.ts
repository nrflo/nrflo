import { useQuery } from '@tanstack/react-query'
import { useProjectStore } from '@/stores/projectStore'
import { getWorkflowTrace } from '@/api/trace'

export const traceKeys = {
  all: ['workflow-trace'] as const,
  instance: (iid: string) => [...traceKeys.all, iid] as const,
}

export function useTrace(workflowInstanceId: string | undefined) {
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  return useQuery({
    queryKey: traceKeys.instance(workflowInstanceId ?? ''),
    queryFn: () => getWorkflowTrace(workflowInstanceId!),
    enabled: projectsLoaded && !!workflowInstanceId,
    staleTime: 2000,
  })
}
