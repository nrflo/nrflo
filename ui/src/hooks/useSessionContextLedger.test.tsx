import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { applyLedgerEvent, useSessionContextLedger } from './useSessionContextLedger'
import type { WSEvent } from '@/hooks/useWebSocket'
import type { LedgerLiveTotals } from '@/types/contextLedger'

function totals(overrides: Partial<LedgerLiveTotals> = {}): LedgerLiveTotals {
  return {
    session_id: 's1',
    total_tokens: 100,
    entry_count: 3,
    totals_by_kind: { dialog: 100 },
    ...overrides,
  }
}

describe('applyLedgerEvent', () => {
  it('replaces prev with the payload when session_id matches', () => {
    const next = applyLedgerEvent(undefined, totals(), 's1')
    expect(next).toEqual(totals())
  })

  it('ignores a payload for a different session, keeping prev', () => {
    const prev = totals({ total_tokens: 100 })
    const next = applyLedgerEvent(prev, totals({ session_id: 's2', total_tokens: 999 }), 's1')
    expect(next).toBe(prev)
  })

  it('ignores any payload when sessionId is undefined', () => {
    const prev = totals()
    const next = applyLedgerEvent(prev, totals({ total_tokens: 999 }), undefined)
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

vi.mock('@/api/contextLedger', () => ({
  fetchSessionContextLedger: vi.fn().mockResolvedValue(null),
}))

function makeEvent(data: LedgerLiveTotals): WSEvent {
  return {
    type: 'agent.context_ledger',
    project_id: 'p',
    ticket_id: '',
    timestamp: '2026-01-01T00:00:00Z',
    data: data as unknown as Record<string, unknown>,
  }
}

describe('useSessionContextLedger', () => {
  let queryClient: QueryClient

  beforeEach(() => {
    vi.clearAllMocks()
    capturedHandler = null
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  })

  function wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  }

  it('updates liveTotals and invalidates the query on a matching agent.context_ledger event', () => {
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
    const { result } = renderHook(() => useSessionContextLedger('s1', true), { wrapper })

    expect(mockAddEventListener).toHaveBeenCalled()
    expect(result.current.liveTotals).toBeUndefined()

    act(() => {
      capturedHandler?.(makeEvent(totals({ total_tokens: 250, entry_count: 5 })))
    })

    expect(result.current.liveTotals).toEqual(totals({ total_tokens: 250, entry_count: 5 }))
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['session-context-ledger', 's1'] })
  })

  it('ignores an event for a different session_id', () => {
    const { result } = renderHook(() => useSessionContextLedger('s1', true), { wrapper })

    act(() => {
      capturedHandler?.(makeEvent(totals({ session_id: 's2', total_tokens: 999 })))
    })

    expect(result.current.liveTotals).toBeUndefined()
  })

  it('does not subscribe when disabled', () => {
    renderHook(() => useSessionContextLedger('s1', false), { wrapper })
    expect(mockAddEventListener).not.toHaveBeenCalled()
  })
})
