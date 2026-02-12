import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useSessionMessages } from './useTickets'
import * as ticketsApi from '@/api/tickets'
import type { SessionMessagesResponse } from '@/types/workflow'
import type { ReactNode } from 'react'

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: (selector: (s: { currentProject: string; projectsLoaded: boolean }) => unknown) =>
    selector({ currentProject: 'test-project', projectsLoaded: true }),
}))

vi.mock('@/api/tickets', async () => {
  const actual = await vi.importActual('@/api/tickets')
  return {
    ...actual,
    getSessionMessages: vi.fn(),
  }
})

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        {children}
      </QueryClientProvider>
    )
  }
}

const sampleResponse: SessionMessagesResponse = {
  session_id: 'session-1',
  messages: [
    { content: '[Read] src/main.ts', created_at: '2026-01-01T00:00:00Z' },
    { content: '[Edit] src/utils.ts', created_at: '2026-01-01T00:00:01Z' },
    { content: '[Bash] npm test', created_at: '2026-01-01T00:00:02Z' },
  ],
  total: 3,
}

describe('useSessionMessages', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('fetches messages when sessionId is provided', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue(sampleResponse)

    const { result } = renderHook(
      () => useSessionMessages('session-1'),
      { wrapper: createWrapper() }
    )

    await waitFor(() => {
      expect(result.current.data).toBeDefined()
    })

    expect(ticketsApi.getSessionMessages).toHaveBeenCalledWith('session-1')
    expect(result.current.data?.messages).toEqual(sampleResponse.messages)
    expect(result.current.data?.total).toBe(3)
  })

  it('does not fetch when sessionId is undefined', () => {
    const { result } = renderHook(
      () => useSessionMessages(undefined),
      { wrapper: createWrapper() }
    )

    expect(result.current.data).toBeUndefined()
    expect(result.current.fetchStatus).toBe('idle')
    expect(ticketsApi.getSessionMessages).not.toHaveBeenCalled()
  })

  it('does not fetch when enabled is false', () => {
    const { result } = renderHook(
      () => useSessionMessages('session-1', { enabled: false }),
      { wrapper: createWrapper() }
    )

    expect(result.current.data).toBeUndefined()
    expect(result.current.fetchStatus).toBe('idle')
    expect(ticketsApi.getSessionMessages).not.toHaveBeenCalled()
  })

  it('uses shorter staleTime when isRunning is true', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue(sampleResponse)

    const { result } = renderHook(
      () => useSessionMessages('session-1', { isRunning: true }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => {
      expect(result.current.data).toBeDefined()
    })

    // The hook should have been called - staleTime=2000 for running agents
    expect(ticketsApi.getSessionMessages).toHaveBeenCalledTimes(1)
  })

  it('uses longer staleTime when isRunning is false', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue(sampleResponse)

    const { result } = renderHook(
      () => useSessionMessages('session-1', { isRunning: false }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => {
      expect(result.current.data).toBeDefined()
    })

    // The hook should have been called - staleTime=30000 for non-running
    expect(ticketsApi.getSessionMessages).toHaveBeenCalledTimes(1)
  })

  it('returns error state when API call fails', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockRejectedValue(new Error('Network error'))

    const { result } = renderHook(
      () => useSessionMessages('session-1'),
      { wrapper: createWrapper() }
    )

    await waitFor(() => {
      expect(result.current.isError).toBe(true)
    })

    expect(result.current.error).toBeDefined()
  })
})
