import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  createCustomProvider,
  deleteCustomProvider,
  listCustomProviders,
  updateCustomProvider,
  type CreateCustomProviderRequest,
  type UpdateCustomProviderRequest,
} from '@/api/customProviders'

export const customProviderKeys = {
  all: ['custom-providers'] as const,
  list: () => [...customProviderKeys.all, 'list'] as const,
}

export function useCustomProviders() {
  return useQuery({ queryKey: customProviderKeys.list(), queryFn: listCustomProviders })
}

export function useCreateCustomProvider() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: CreateCustomProviderRequest) => createCustomProvider(data),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: customProviderKeys.list() }),
  })
}

export function useUpdateCustomProvider() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ name, data }: { name: string; data: UpdateCustomProviderRequest }) =>
      updateCustomProvider(name, data),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: customProviderKeys.list() }),
  })
}

export function useDeleteCustomProvider() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: deleteCustomProvider,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: customProviderKeys.list() }),
  })
}
