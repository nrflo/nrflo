import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { systemAgentRunKeys, useSystemAgentRuns } from './useSystemAgentRuns'
import { listSystemAgentRuns } from '@/api/systemAgentRuns'
import type { WSEvent } from '@/hooks/useWebSocket'

let capturedHandler: ((e: WSEvent) => void) | null = null
const mockAddEventListener = vi.fn((fn: (e: WSEvent) => void) => {
  capturedHandler = fn
})
const mockRemoveEventListener = vi.fn()

vi.mock('@/providers/WebSocketProvider', () => ({
  useWebSocketContext: () => ({
    addEventListener: mockAddEventListener,
    removeEventListener: mockRemoveEventListener,
  }),
}))

vi.mock('@/api/systemAgentRuns', () => ({
  listSystemAgentRuns: vi.fn().mockResolvedValue({ items: [], limit: 50 }),
}))

function stepAdvanced(rotated: boolean): WSEvent {
  return {
    type: 'step.advanced',
    project_id: 'p',
    ticket_id: '',
    timestamp: '2026-01-01T00:00:00Z',
    data: { rotated },
  }
}

describe('useSystemAgentRuns', () => {
  let queryClient: QueryClient

  beforeEach(() => {
    vi.clearAllMocks()
    capturedHandler = null
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  })

  function wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  }

  it('invalidates the runs list on a step.advanced event with rotated: true', async () => {
    const { result } = renderHook(() => useSystemAgentRuns(50), { wrapper })
    await waitFor(() => expect(result.current.data).toBeDefined())

    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
    act(() => {
      capturedHandler?.(stepAdvanced(true))
    })

    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: systemAgentRunKeys.list(50) })
  })

  it('does not invalidate on a step.advanced event with rotated: false', async () => {
    const { result } = renderHook(() => useSystemAgentRuns(50), { wrapper })
    await waitFor(() => expect(result.current.data).toBeDefined())

    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
    act(() => {
      capturedHandler?.(stepAdvanced(false))
    })

    expect(invalidateSpy).not.toHaveBeenCalled()
  })

  it('fetches with the given limit', async () => {
    renderHook(() => useSystemAgentRuns(100), { wrapper })
    await waitFor(() => expect(listSystemAgentRuns).toHaveBeenCalledWith({ limit: 100 }))
  })
})
