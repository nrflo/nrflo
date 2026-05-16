import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { listServiceTokens, createServiceToken, deleteServiceToken } from '@/api/serviceTokens'

export const serviceTokenKeys = {
  all: ['service-tokens'] as const,
  list: () => [...serviceTokenKeys.all, 'list'] as const,
}

export function useServiceTokens() {
  return useQuery({
    queryKey: serviceTokenKeys.list(),
    queryFn: listServiceTokens,
  })
}

export function useCreateServiceToken() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ projectId, name }: { projectId: string; name: string }) =>
      createServiceToken(projectId, name),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: serviceTokenKeys.all })
    },
  })
}

export function useDeleteServiceToken() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteServiceToken(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: serviceTokenKeys.all })
    },
  })
}
