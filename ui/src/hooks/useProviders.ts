import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { listProviders, updateProvider, type ProviderName, type CLIMode } from '@/api/providers'

export const providerKeys = {
  all: ['providers'] as const,
}

export function useProviders() {
  return useQuery({
    queryKey: providerKeys.all,
    queryFn: listProviders,
  })
}

export function useUpdateProvider() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ name, modes }: { name: ProviderName; modes: CLIMode[] }) =>
      updateProvider(name, modes),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: providerKeys.all })
    },
  })
}
