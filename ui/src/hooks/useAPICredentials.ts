import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  listAPICredentials,
  createAPICredential,
  updateAPICredential,
  deleteAPICredential,
} from '@/api/apiCredentials'
import type { APICredentialUpdateRequest } from '@/types/apiCredential'

export const apiCredentialKeys = {
  all: ['api-credentials'] as const,
  list: (projectId?: string) => [...apiCredentialKeys.all, 'list', projectId ?? ''] as const,
}

export function useAPICredentials(projectId?: string) {
  return useQuery({
    queryKey: apiCredentialKeys.list(projectId),
    queryFn: () => listAPICredentials(projectId),
  })
}

export function useCreateAPICredential() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: createAPICredential,
    onSuccess: () => qc.invalidateQueries({ queryKey: apiCredentialKeys.all }),
  })
}

export function useUpdateAPICredential() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: APICredentialUpdateRequest }) =>
      updateAPICredential(id, data),
    onSuccess: () => qc.invalidateQueries({ queryKey: apiCredentialKeys.all }),
  })
}

export function useDeleteAPICredential() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: deleteAPICredential,
    onSuccess: () => qc.invalidateQueries({ queryKey: apiCredentialKeys.all }),
  })
}
