import {
  useQuery,
  useMutation,
  useQueryClient,
  type UseQueryOptions,
} from '@tanstack/react-query'
import {
  listChains,
  getChain,
  createChain,
  updateChain,
  startChain,
  cancelChain,
  deleteChain,
  appendToChain,
  removeFromChain,
  runEpicWorkflow,
  type ListChainsParams,
  type RunEpicWorkflowParams,
} from '@/api/chains'
import type {
  ChainExecution,
  ChainCreateRequest,
  ChainUpdateRequest,
  ChainAppendRequest,
  ChainRemoveRequest,
} from '@/types/chain'
import { useProjectStore } from '@/stores/projectStore'

// Query key factory — follows ticketKeys pattern from useTickets.ts
export const chainKeys = {
  all: ['chains'] as const,
  lists: () => [...chainKeys.all, 'list'] as const,
  list: (params?: ListChainsParams) => [...chainKeys.lists(), params] as const,
  details: () => [...chainKeys.all, 'detail'] as const,
  detail: (id: string) => [...chainKeys.details(), id] as const,
}

export function useChainList(
  params?: ListChainsParams,
  options?: Omit<UseQueryOptions<ChainExecution[]>, 'queryKey' | 'queryFn'>
) {
  const project = useProjectStore((s) => s.currentProject)
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  return useQuery({
    queryKey: [...chainKeys.list(params), project],
    queryFn: () => listChains(params),
    enabled: projectsLoaded && (options?.enabled ?? true),
    ...options,
  })
}

export function useChain(
  id: string,
  options?: Omit<UseQueryOptions<ChainExecution>, 'queryKey' | 'queryFn'>
) {
  const project = useProjectStore((s) => s.currentProject)
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  return useQuery({
    queryKey: [...chainKeys.detail(id), project],
    queryFn: () => getChain(id),
    enabled: projectsLoaded && !!id && (options?.enabled ?? true),
    ...options,
  })
}

export function useCreateChain() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: ChainCreateRequest) => createChain(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: chainKeys.lists() })
    },
  })
}

export function useUpdateChain() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: ChainUpdateRequest }) =>
      updateChain(id, data),
    onSuccess: (chain: ChainExecution) => {
      queryClient.invalidateQueries({ queryKey: chainKeys.lists() })
      queryClient.invalidateQueries({ queryKey: chainKeys.detail(chain.id) })
    },
  })
}

export function useStartChain() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => startChain(id),
    onSuccess: (_data, id) => {
      queryClient.invalidateQueries({ queryKey: chainKeys.detail(id) })
      queryClient.invalidateQueries({ queryKey: chainKeys.lists() })
    },
  })
}

export function useCancelChain() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => cancelChain(id),
    onSuccess: (_data, id) => {
      queryClient.invalidateQueries({ queryKey: chainKeys.detail(id) })
      queryClient.invalidateQueries({ queryKey: chainKeys.lists() })
    },
  })
}

export function useDeleteChain() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteChain(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: chainKeys.lists() })
    },
  })
}

export function useAppendToChain() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: ChainAppendRequest }) =>
      appendToChain(id, data),
    onSuccess: (chain: ChainExecution) => {
      queryClient.invalidateQueries({ queryKey: chainKeys.detail(chain.id) })
      queryClient.invalidateQueries({ queryKey: chainKeys.lists() })
    },
  })
}

export function useRemoveFromChain() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: ChainRemoveRequest }) =>
      removeFromChain(id, data),
    onSuccess: (chain: ChainExecution) => {
      queryClient.invalidateQueries({ queryKey: chainKeys.detail(chain.id) })
      queryClient.invalidateQueries({ queryKey: chainKeys.lists() })
    },
  })
}

export function useRunEpicWorkflow() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ ticketId, params }: { ticketId: string; params: RunEpicWorkflowParams }) =>
      runEpicWorkflow(ticketId, params),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: chainKeys.lists() })
    },
  })
}
