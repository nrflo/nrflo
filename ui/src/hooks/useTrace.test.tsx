import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { useTrace, traceKeys } from './useTrace'
import * as traceApi from '@/api/trace'

vi.mock('@/api/trace')

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
  vi.mocked(traceApi.getWorkflowTrace).mockReset()
})

describe('useTrace', () => {
  it('key factory namespaces by instance id', () => {
    expect(traceKeys.instance('wfi-1')).toEqual(['workflow-trace', 'wfi-1'])
  })

  it('fetches the trace for an instance id', async () => {
    vi.mocked(traceApi.getWorkflowTrace).mockResolvedValue({
      instance_id: 'wfi-1',
      project_id: 'p',
      workflow: 'wf',
      status: 'active',
      started_at: '2025-01-01T00:00:00Z',
    })
    const { result } = renderHook(() => useTrace('wfi-1'), { wrapper: createWrapper() })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(traceApi.getWorkflowTrace).toHaveBeenCalledWith('wfi-1')
    expect(result.current.data?.instance_id).toBe('wfi-1')
  })

  it('is disabled without an instance id', () => {
    const { result } = renderHook(() => useTrace(undefined), { wrapper: createWrapper() })
    expect(result.current.fetchStatus).toBe('idle')
    expect(traceApi.getWorkflowTrace).not.toHaveBeenCalled()
  })
})
