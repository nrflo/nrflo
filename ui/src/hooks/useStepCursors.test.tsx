import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { stepCursorKeys, useStepCursors } from './useStepCursors'
import { fetchStepCursors } from '@/api/stepCursors'
import type { WSEvent } from '@/hooks/useWebSocket'
import type { StepAdvancedEvent, StepCursorsResponse } from '@/types/stepwise'

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

vi.mock('@/api/stepCursors', () => ({
  fetchStepCursors: vi.fn(),
}))

function makeResponse(overrides: Partial<StepCursorsResponse> = {}): StepCursorsResponse {
  return {
    workflow_instance_id: 'wi1',
    cursors: {
      implementation: {
        node_id: 'implementation',
        revision: 1,
        current_index: 1,
        total: 3,
        current_step_id: 'step-2',
        done: false,
        updated_at: '2026-01-01T00:00:00Z',
        steps: [],
      },
    },
    ...overrides,
  }
}

function makeEvent(overrides: Partial<StepAdvancedEvent> = {}): StepAdvancedEvent {
  return {
    workflow_instance_id: 'wi1',
    node_id: 'implementation',
    step_id: 'step-3',
    step_index: 2,
    total: 3,
    rejected_count: 0,
    rotated: false,
    ...overrides,
  }
}

function makeWsEvent(data: StepAdvancedEvent): WSEvent {
  return {
    type: 'step.advanced',
    project_id: 'p',
    ticket_id: '',
    timestamp: '2026-01-01T00:00:00Z',
    data: data as unknown as Record<string, unknown>,
  }
}

describe('useStepCursors', () => {
  let queryClient: QueryClient

  beforeEach(() => {
    vi.clearAllMocks()
    capturedHandler = null
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  })

  function wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  }

  it('initial fetch populates data', async () => {
    vi.mocked(fetchStepCursors).mockResolvedValue(makeResponse())
    const { result } = renderHook(() => useStepCursors('wi1'), { wrapper })

    await waitFor(() => expect(result.current.data).toBeDefined())
    expect(result.current.data?.cursors.implementation.current_index).toBe(1)
    expect(fetchStepCursors).toHaveBeenCalledWith('wi1')
  })

  it('patches and invalidates the query on a step.advanced event for the same instance', async () => {
    vi.mocked(fetchStepCursors).mockResolvedValue(makeResponse())
    const { result } = renderHook(() => useStepCursors('wi1'), { wrapper })
    await waitFor(() => expect(result.current.data).toBeDefined())

    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    act(() => {
      capturedHandler?.(makeWsEvent(makeEvent({ step_index: 2, step_id: 'step-3' })))
    })

    expect(
      queryClient.getQueryData<StepCursorsResponse>(stepCursorKeys.instance('wi1'))?.cursors
        .implementation.current_index
    ).toBe(2)
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: stepCursorKeys.instance('wi1') })
  })

  it('ignores a step.advanced event for a different workflow_instance_id', async () => {
    vi.mocked(fetchStepCursors).mockResolvedValue(makeResponse())
    const { result } = renderHook(() => useStepCursors('wi1'), { wrapper })
    await waitFor(() => expect(result.current.data).toBeDefined())

    act(() => {
      capturedHandler?.(makeWsEvent(makeEvent({ workflow_instance_id: 'wi-other', step_index: 2 })))
    })

    expect(
      queryClient.getQueryData<StepCursorsResponse>(stepCursorKeys.instance('wi1'))?.cursors
        .implementation.current_index
    ).toBe(1)
  })

  it('removeEventListener runs on unmount', () => {
    vi.mocked(fetchStepCursors).mockResolvedValue(makeResponse())
    const { unmount } = renderHook(() => useStepCursors('wi1'), { wrapper })
    expect(mockAddEventListener).toHaveBeenCalled()
    unmount()
    expect(mockRemoveEventListener).toHaveBeenCalled()
  })

  it('issues no request when disabled (no instanceId)', () => {
    renderHook(() => useStepCursors(undefined), { wrapper })
    expect(fetchStepCursors).not.toHaveBeenCalled()
    expect(mockAddEventListener).not.toHaveBeenCalled()
  })
})
