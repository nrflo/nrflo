import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { listEnvVars, putEnvVar, deleteEnvVar } from '@/api/projectEnvVars'

export const projectEnvVarKeys = {
  all: ['project-env-vars'] as const,
  list: (projectId: string) => [...projectEnvVarKeys.all, 'list', projectId] as const,
}

export function useProjectEnvVars(projectId: string) {
  return useQuery({
    queryKey: projectEnvVarKeys.list(projectId),
    queryFn: () => listEnvVars(projectId),
    enabled: !!projectId,
  })
}

export function usePutProjectEnvVar() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ projectId, name, value }: { projectId: string; name: string; value: string }) =>
      putEnvVar(projectId, name, value),
    onSuccess: (_data, { projectId }) => {
      qc.invalidateQueries({ queryKey: projectEnvVarKeys.list(projectId) })
    },
  })
}

export function useDeleteProjectEnvVar() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ projectId, name }: { projectId: string; name: string }) =>
      deleteEnvVar(projectId, name),
    onSuccess: (_data, { projectId }) => {
      qc.invalidateQueries({ queryKey: projectEnvVarKeys.list(projectId) })
    },
  })
}
