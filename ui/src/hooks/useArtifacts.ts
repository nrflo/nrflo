import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useProjectStore } from '@/stores/projectStore'
import { listArtifacts, deleteArtifact } from '@/api/artifacts'

export const artifactKeys = {
  all: ['artifacts'] as const,
  instance: (iid: string) => ['artifacts', 'instance', iid] as const,
}

export function useArtifacts(workflowInstanceId: string | undefined) {
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  return useQuery({
    queryKey: artifactKeys.instance(workflowInstanceId ?? ''),
    queryFn: () => listArtifacts(workflowInstanceId!),
    enabled: projectsLoaded && !!workflowInstanceId,
  })
}

export function useDeleteArtifact() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id }: { id: string; workflowInstanceId: string }) => deleteArtifact(id),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: artifactKeys.instance(variables.workflowInstanceId) })
    },
  })
}
