import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { applyDigestEvent, useSessionHandoffDigest } from './useSessionHandoffDigest'
import type { WSEvent } from '@/hooks/useWebSocket'
import type { HandoffDigestEvent } from '@/types/handoffDigest'

function digestEvent(overrides: Partial<HandoffDigestEvent> = {}): HandoffDigestEvent {
  return {
    session_id: 's1',
    content: 'digest content',
    version: 1,
    fold_count: 2,
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('applyDigestEvent', () => {
  it('replaces prev with the payload when session_id matches', () => {
    const next = applyDigestEvent(undefined, digestEvent(), 's1')
    expect(next).toEqual(digestEvent())
  })

  it('ignores a payload for a different session, keeping prev', () => {
    const prev = digestEvent({ fold_count: 2 })
    const next = applyDigestEvent(prev, digestEvent({ session_id: 's2', fold_count: 99 }), 's1')
    expect(next).toBe(prev)
  })

  it('ignores any payload when sessionId is undefined', () => {
    const prev = digestEvent()
    const next = applyDigestEvent(prev, digestEvent({ fold_count: 99 }), undefined)
    expect(next).toBe(prev)
  })
})

// ---- WS mock for the hook-level test ----
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

vi.mock('@/api/handoffDigest', () => ({
  fetchSessionHandoffDigest: vi.fn().mockResolvedValue(null),
}))

function makeEvent(data: HandoffDigestEvent): WSEvent {
  return {
    type: 'agent.handoff_digest',
    project_id: 'p',
    ticket_id: '',
    timestamp: '2026-01-01T00:00:00Z',
    data: data as unknown as Record<string, unknown>,
  }
}

describe('useSessionHandoffDigest', () => {
  let queryClient: QueryClient

  beforeEach(() => {
    vi.clearAllMocks()
    capturedHandler = null
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  })

  function wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  }

  it('updates the live overlay and invalidates the query on a matching agent.handoff_digest event', () => {
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
    const { result } = renderHook(() => useSessionHandoffDigest('s1', true), { wrapper })

    expect(mockAddEventListener).toHaveBeenCalled()
    expect(result.current.live).toBeUndefined()

    act(() => {
      capturedHandler?.(makeEvent(digestEvent({ content: 'new content', fold_count: 5 })))
    })

    expect(result.current.live).toEqual(digestEvent({ content: 'new content', fold_count: 5 }))
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['session-handoff-digest', 's1'] })
  })

  it('ignores an event for a different session_id', () => {
    const { result } = renderHook(() => useSessionHandoffDigest('s1', true), { wrapper })

    act(() => {
      capturedHandler?.(makeEvent(digestEvent({ session_id: 's2', fold_count: 99 })))
    })

    expect(result.current.live).toBeUndefined()
  })

  it('does not subscribe when disabled', () => {
    renderHook(() => useSessionHandoffDigest('s1', false), { wrapper })
    expect(mockAddEventListener).not.toHaveBeenCalled()
  })
})
