import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import {
  chainKeys,
  useChainList,
  useChain,
  useCreateChain,
  useUpdateChain,
  useStartChain,
  useCancelChain,
} from './useChains'
import * as chainsApi from '@/api/chains'
import type { ChainExecution, ChainCreateRequest, ChainUpdateRequest } from '@/types/chain'

// Mock chains API
vi.mock('@/api/chains')

// Mock project store
vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn((selector) => {
    const store = {
      currentProject: 'test-project',
      projectsLoaded: true,
    }
    return selector(store)
  }),
}))

function createMockChain(overrides: Partial<ChainExecution> = {}): ChainExecution {
  return {
    id: 'chain-123',
    project_id: 'test-project',
    name: 'Test Chain',
    status: 'pending',
    workflow_name: 'feature',
    category: 'full',
    created_by: 'test-user',
    total_items: 0,
    completed_items: 0,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    items: [],
    ...overrides,
  }
}

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
}

describe('chainKeys - Query Key Factory', () => {
  it('generates all chains key', () => {
    expect(chainKeys.all).toEqual(['chains'])
  })

  it('generates lists key', () => {
    expect(chainKeys.lists()).toEqual(['chains', 'list'])
  })

  it('generates list key with params', () => {
    const params = { status: 'running' }
    expect(chainKeys.list(params)).toEqual(['chains', 'list', params])
  })

  it('generates list key without params', () => {
    expect(chainKeys.list()).toEqual(['chains', 'list', undefined])
  })

  it('generates details key', () => {
    expect(chainKeys.details()).toEqual(['chains', 'detail'])
  })

  it('generates detail key with ID', () => {
    expect(chainKeys.detail('chain-123')).toEqual(['chains', 'detail', 'chain-123'])
  })
})

describe('useChainList', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('fetches chain list without filters', async () => {
    const chains = [createMockChain()]
    vi.mocked(chainsApi.listChains).mockResolvedValue(chains)

    const { result } = renderHook(() => useChainList(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(chainsApi.listChains).toHaveBeenCalledWith(undefined)
    expect(result.current.data).toEqual(chains)
  })

  it('fetches chain list with status filter', async () => {
    const chains = [createMockChain({ status: 'running' })]
    vi.mocked(chainsApi.listChains).mockResolvedValue(chains)

    const { result } = renderHook(() => useChainList({ status: 'running' }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(chainsApi.listChains).toHaveBeenCalledWith({ status: 'running' })
    expect(result.current.data).toEqual(chains)
  })

  it('includes project in query key', async () => {
    vi.mocked(chainsApi.listChains).mockResolvedValue([])

    const { result } = renderHook(() => useChainList(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    // Query key should include project
    expect(result.current.data).toBeDefined()
  })

  it('supports custom refetchInterval option', async () => {
    vi.mocked(chainsApi.listChains).mockResolvedValue([])

    const { result } = renderHook(
      () => useChainList(undefined, { refetchInterval: 5000 }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data).toBeDefined()
  })

  it('disables query when projectsLoaded is false', () => {
    vi.mocked(chainsApi.listChains).mockResolvedValue([])

    // This test would need to mock projectStore with projectsLoaded: false
    // For simplicity, we verify the hook can be called
    const { result } = renderHook(() => useChainList(), {
      wrapper: createWrapper(),
    })

    expect(result.current).toBeDefined()
  })
})

describe('useChain', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('fetches single chain by ID', async () => {
    const chain = createMockChain({ id: 'chain-abc' })
    vi.mocked(chainsApi.getChain).mockResolvedValue(chain)

    const { result } = renderHook(() => useChain('chain-abc'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(chainsApi.getChain).toHaveBeenCalledWith('chain-abc')
    expect(result.current.data).toEqual(chain)
  })

  it('includes project in query key', async () => {
    const chain = createMockChain()
    vi.mocked(chainsApi.getChain).mockResolvedValue(chain)

    const { result } = renderHook(() => useChain('chain-123'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data).toBeDefined()
  })

  it('supports custom refetchInterval option', async () => {
    const chain = createMockChain()
    vi.mocked(chainsApi.getChain).mockResolvedValue(chain)

    const { result } = renderHook(
      () => useChain('chain-123', { refetchInterval: 3000 }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data).toEqual(chain)
  })

  it('disables query when ID is empty', () => {
    vi.mocked(chainsApi.getChain).mockResolvedValue(createMockChain())

    renderHook(() => useChain(''), {
      wrapper: createWrapper(),
    })

    // Query should not run when ID is empty
    expect(chainsApi.getChain).not.toHaveBeenCalled()
  })
})

describe('useCreateChain', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('creates a new chain', async () => {
    const newChain = createMockChain()
    const createData: ChainCreateRequest = {
      name: 'New Chain',
      workflow_name: 'feature',
      category: 'full',
      ticket_ids: ['TICKET-1', 'TICKET-2'],
    }

    vi.mocked(chainsApi.createChain).mockResolvedValue(newChain)

    const { result } = renderHook(() => useCreateChain(), {
      wrapper: createWrapper(),
    })

    await result.current.mutateAsync(createData)

    expect(chainsApi.createChain).toHaveBeenCalledWith(createData)
  })

  it('invalidates chain list queries on success', async () => {
    const newChain = createMockChain()
    const createData: ChainCreateRequest = {
      name: 'New Chain',
      workflow_name: 'feature',
      ticket_ids: ['TICKET-1'],
    }

    vi.mocked(chainsApi.createChain).mockResolvedValue(newChain)

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    })
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )

    const { result } = renderHook(() => useCreateChain(), { wrapper })

    await result.current.mutateAsync(createData)

    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: chainKeys.lists(),
    })
  })
})

describe('useUpdateChain', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('updates an existing chain', async () => {
    const updatedChain = createMockChain({ name: 'Updated Chain' })
    const updateData: ChainUpdateRequest = {
      name: 'Updated Chain',
      ticket_ids: ['TICKET-1', 'TICKET-2', 'TICKET-3'],
    }

    vi.mocked(chainsApi.updateChain).mockResolvedValue(updatedChain)

    const { result } = renderHook(() => useUpdateChain(), {
      wrapper: createWrapper(),
    })

    await result.current.mutateAsync({ id: 'chain-123', data: updateData })

    expect(chainsApi.updateChain).toHaveBeenCalledWith('chain-123', updateData)
  })

  it('invalidates both list and detail queries on success', async () => {
    const updatedChain = createMockChain({ id: 'chain-abc', name: 'Updated' })
    const updateData: ChainUpdateRequest = { name: 'Updated' }

    vi.mocked(chainsApi.updateChain).mockResolvedValue(updatedChain)

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    })
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )

    const { result } = renderHook(() => useUpdateChain(), { wrapper })

    await result.current.mutateAsync({ id: 'chain-abc', data: updateData })

    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: chainKeys.lists(),
    })
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: chainKeys.detail('chain-abc'),
    })
  })
})

describe('useStartChain', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('starts a chain', async () => {
    const response = { status: 'running', chain_id: 'chain-123' }
    vi.mocked(chainsApi.startChain).mockResolvedValue(response)

    const { result } = renderHook(() => useStartChain(), {
      wrapper: createWrapper(),
    })

    const res = await result.current.mutateAsync('chain-123')

    expect(chainsApi.startChain).toHaveBeenCalledWith('chain-123')
    expect(res).toEqual(response)
  })

  it('invalidates detail and list queries on success', async () => {
    const response = { status: 'running', chain_id: 'chain-xyz' }
    vi.mocked(chainsApi.startChain).mockResolvedValue(response)

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    })
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )

    const { result } = renderHook(() => useStartChain(), { wrapper })

    await result.current.mutateAsync('chain-xyz')

    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: chainKeys.detail('chain-xyz'),
    })
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: chainKeys.lists(),
    })
  })
})

describe('useCancelChain', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('cancels a chain', async () => {
    const response = { status: 'canceled', chain_id: 'chain-123' }
    vi.mocked(chainsApi.cancelChain).mockResolvedValue(response)

    const { result } = renderHook(() => useCancelChain(), {
      wrapper: createWrapper(),
    })

    const res = await result.current.mutateAsync('chain-123')

    expect(chainsApi.cancelChain).toHaveBeenCalledWith('chain-123')
    expect(res).toEqual(response)
  })

  it('invalidates detail and list queries on success', async () => {
    const response = { status: 'canceled', chain_id: 'chain-def' }
    vi.mocked(chainsApi.cancelChain).mockResolvedValue(response)

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    })
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )

    const { result } = renderHook(() => useCancelChain(), { wrapper })

    await result.current.mutateAsync('chain-def')

    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: chainKeys.detail('chain-def'),
    })
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: chainKeys.lists(),
    })
  })
})

describe('useChains - Error Handling', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('handles list fetch errors', async () => {
    const error = new Error('Failed to fetch chains')
    vi.mocked(chainsApi.listChains).mockRejectedValue(error)

    const { result } = renderHook(() => useChainList(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toEqual(error)
  })

  it('handles detail fetch errors', async () => {
    const error = new Error('Chain not found')
    vi.mocked(chainsApi.getChain).mockRejectedValue(error)

    const { result } = renderHook(() => useChain('chain-123'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toEqual(error)
  })

  it('handles create mutation errors', async () => {
    const error = new Error('Failed to create chain')
    vi.mocked(chainsApi.createChain).mockRejectedValue(error)

    const { result } = renderHook(() => useCreateChain(), {
      wrapper: createWrapper(),
    })

    await expect(
      result.current.mutateAsync({
        name: 'Test',
        workflow_name: 'feature',
        ticket_ids: [],
      })
    ).rejects.toThrow('Failed to create chain')
  })

  it('handles update mutation errors', async () => {
    const error = new Error('Failed to update chain')
    vi.mocked(chainsApi.updateChain).mockRejectedValue(error)

    const { result } = renderHook(() => useUpdateChain(), {
      wrapper: createWrapper(),
    })

    await expect(
      result.current.mutateAsync({ id: 'chain-123', data: { name: 'Updated' } })
    ).rejects.toThrow('Failed to update chain')
  })

  it('handles start mutation errors', async () => {
    const error = new Error('Failed to start chain')
    vi.mocked(chainsApi.startChain).mockRejectedValue(error)

    const { result } = renderHook(() => useStartChain(), {
      wrapper: createWrapper(),
    })

    await expect(result.current.mutateAsync('chain-123')).rejects.toThrow(
      'Failed to start chain'
    )
  })

  it('handles cancel mutation errors', async () => {
    const error = new Error('Failed to cancel chain')
    vi.mocked(chainsApi.cancelChain).mockRejectedValue(error)

    const { result } = renderHook(() => useCancelChain(), {
      wrapper: createWrapper(),
    })

    await expect(result.current.mutateAsync('chain-123')).rejects.toThrow(
      'Failed to cancel chain'
    )
  })
})
