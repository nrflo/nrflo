import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useRetryFailedAgent, useRetryFailedProjectAgent, ticketKeys, projectWorkflowKeys } from './useTickets'
import * as workflowsApi from '@/api/workflows'
import * as projectWorkflowsApi from '@/api/projectWorkflows'

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (selector: (s: { currentProject: string; projectsLoaded: boolean }) => unknown) =>
    selector({ currentProject: 'test-project', projectsLoaded: true }),
}))

vi.mock('@/api/workflows', () => ({
  retryFailedAgent: vi.fn(),
}))

vi.mock('@/api/projectWorkflows', () => ({
  retryFailedProjectAgent: vi.fn(),
}))

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
}

describe('useRetryFailedAgent', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('calls retryFailedAgent API with correct parameters', async () => {
    vi.mocked(workflowsApi.retryFailedAgent).mockResolvedValue({ status: 'retrying' })

    const { result } = renderHook(() => useRetryFailedAgent(), {
      wrapper: createWrapper(),
    })

    result.current.mutate({
      ticketId: 'TICKET-123',
      params: {
        workflow: 'feature',
        session_id: 'sess-abc',
      },
    })

    await waitFor(() => {
      expect(workflowsApi.retryFailedAgent).toHaveBeenCalledWith('TICKET-123', {
        workflow: 'feature',
        session_id: 'sess-abc',
      })
    })
  })

  it('invalidates workflow query on success', async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })

    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
    vi.mocked(workflowsApi.retryFailedAgent).mockResolvedValue({ status: 'retrying' })

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )

    const { result } = renderHook(() => useRetryFailedAgent(), { wrapper })

    result.current.mutate({
      ticketId: 'TICKET-123',
      params: {
        workflow: 'feature',
        session_id: 'sess-abc',
      },
    })

    await waitFor(() => {
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ticketKeys.workflow('TICKET-123'),
      })
    })
  })

  it('invalidates agentSessions query on success', async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })

    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
    vi.mocked(workflowsApi.retryFailedAgent).mockResolvedValue({ status: 'retrying' })

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )

    const { result } = renderHook(() => useRetryFailedAgent(), { wrapper })

    result.current.mutate({
      ticketId: 'TICKET-456',
      params: {
        workflow: 'bugfix',
        session_id: 'sess-xyz',
      },
    })

    await waitFor(() => {
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ticketKeys.agentSessions('TICKET-456'),
      })
    })
  })

  it('sets isPending to true during mutation', async () => {
    vi.mocked(workflowsApi.retryFailedAgent).mockImplementation(
      () => new Promise((resolve) => setTimeout(() => resolve({ status: 'retrying' }), 100))
    )

    const { result } = renderHook(() => useRetryFailedAgent(), {
      wrapper: createWrapper(),
    })

    expect(result.current.isPending).toBe(false)

    result.current.mutate({
      ticketId: 'TICKET-123',
      params: {
        workflow: 'feature',
        session_id: 'sess-abc',
      },
    })

    await waitFor(() => {
      expect(result.current.isPending).toBe(true)
    })
  })

  it('propagates errors from API', async () => {
    const error = new Error('Retry failed')
    vi.mocked(workflowsApi.retryFailedAgent).mockRejectedValue(error)

    const { result } = renderHook(() => useRetryFailedAgent(), {
      wrapper: createWrapper(),
    })

    result.current.mutate({
      ticketId: 'TICKET-123',
      params: {
        workflow: 'feature',
        session_id: 'sess-abc',
      },
    })

    await waitFor(() => {
      expect(result.current.isError).toBe(true)
      expect(result.current.error).toEqual(error)
    })
  })
})

describe('useRetryFailedProjectAgent', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('calls retryFailedProjectAgent API with correct parameters', async () => {
    vi.mocked(projectWorkflowsApi.retryFailedProjectAgent).mockResolvedValue({ status: 'retrying' })

    const { result } = renderHook(() => useRetryFailedProjectAgent(), {
      wrapper: createWrapper(),
    })

    result.current.mutate({
      projectId: 'test-project',
      params: {
        workflow: 'feature',
        session_id: 'sess-project-123',
      },
    })

    await waitFor(() => {
      expect(projectWorkflowsApi.retryFailedProjectAgent).toHaveBeenCalledWith('test-project', {
        workflow: 'feature',
        session_id: 'sess-project-123',
      })
    })
  })

  it('invalidates project workflow query on success', async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })

    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
    vi.mocked(projectWorkflowsApi.retryFailedProjectAgent).mockResolvedValue({ status: 'retrying' })

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )

    const { result } = renderHook(() => useRetryFailedProjectAgent(), { wrapper })

    result.current.mutate({
      projectId: 'my-project',
      params: {
        workflow: 'feature',
        session_id: 'sess-project-abc',
      },
    })

    await waitFor(() => {
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: projectWorkflowKeys.workflow('my-project'),
      })
    })
  })

  it('sets isPending to true during mutation', async () => {
    vi.mocked(projectWorkflowsApi.retryFailedProjectAgent).mockImplementation(
      () => new Promise((resolve) => setTimeout(() => resolve({ status: 'retrying' }), 100))
    )

    const { result } = renderHook(() => useRetryFailedProjectAgent(), {
      wrapper: createWrapper(),
    })

    expect(result.current.isPending).toBe(false)

    result.current.mutate({
      projectId: 'test-project',
      params: {
        workflow: 'feature',
        session_id: 'sess-project-123',
      },
    })

    await waitFor(() => {
      expect(result.current.isPending).toBe(true)
    })
  })

  it('propagates errors from API', async () => {
    const error = new Error('Project retry failed')
    vi.mocked(projectWorkflowsApi.retryFailedProjectAgent).mockRejectedValue(error)

    const { result } = renderHook(() => useRetryFailedProjectAgent(), {
      wrapper: createWrapper(),
    })

    result.current.mutate({
      projectId: 'test-project',
      params: {
        workflow: 'feature',
        session_id: 'sess-project-123',
      },
    })

    await waitFor(() => {
      expect(result.current.isError).toBe(true)
      expect(result.current.error).toEqual(error)
    })
  })
})
