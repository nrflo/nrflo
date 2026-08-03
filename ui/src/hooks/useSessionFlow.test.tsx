import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { useSessionFlow, useSessionStats, sessionFlowKeys } from './useSessionFlow'
import * as sessionsApi from '@/api/sessions'

vi.mock('@/api/sessions')

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn((selector) => selector({ currentProject: 'p', projectsLoaded: true })),
}))

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  }
}

beforeEach(() => {
  vi.mocked(sessionsApi.getSessionFlow).mockReset()
  vi.mocked(sessionsApi.getSessionStats).mockReset()
})

describe('sessionFlowKeys', () => {
  it('namespaces flow and stats keys by session id', () => {
    expect(sessionFlowKeys.flow('sid-1')).toEqual(['session-flow', 'sid-1'])
    expect(sessionFlowKeys.stats('sid-1')).toEqual(['session-stats', 'sid-1'])
  })
})

describe('useSessionFlow', () => {
  it('fetches the flow for a session id', async () => {
    vi.mocked(sessionsApi.getSessionFlow).mockResolvedValue({
      root_session_id: 'sid-1',
      nodes: [],
      edges: [],
      truncated: false,
    })
    const { result } = renderHook(() => useSessionFlow('sid-1'), { wrapper: createWrapper() })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(sessionsApi.getSessionFlow).toHaveBeenCalledWith('sid-1')
  })

  it('is disabled without a session id', () => {
    const { result } = renderHook(() => useSessionFlow(undefined), { wrapper: createWrapper() })
    expect(result.current.fetchStatus).toBe('idle')
    expect(sessionsApi.getSessionFlow).not.toHaveBeenCalled()
  })
})

describe('useSessionStats', () => {
  it('fetches stats for a session id', async () => {
    vi.mocked(sessionsApi.getSessionStats).mockResolvedValue({
      root_session_id: 'sid-1',
      tool_calls: [],
      self_cost_usd: 0,
      subtree_cost_usd: 0,
      self_tokens: 0,
      subtree_tokens: 0,
    })
    const { result } = renderHook(() => useSessionStats('sid-1'), { wrapper: createWrapper() })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(sessionsApi.getSessionStats).toHaveBeenCalledWith('sid-1')
  })

  it('is disabled without a session id', () => {
    const { result } = renderHook(() => useSessionStats(undefined), { wrapper: createWrapper() })
    expect(result.current.fetchStatus).toBe('idle')
    expect(sessionsApi.getSessionStats).not.toHaveBeenCalled()
  })
})
