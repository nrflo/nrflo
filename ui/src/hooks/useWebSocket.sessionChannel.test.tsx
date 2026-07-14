// vi.hoisted runs before static imports — provide localStorage so the
// connectionsStore persist middleware does not throw (mirrors
// useWebSocket.reconnect.test.tsx).
vi.hoisted(() => {
  const data: Record<string, string> = {}
  Object.defineProperty(globalThis, 'localStorage', {
    value: {
      getItem: (k: string) => data[k] ?? null,
      setItem: (k: string, v: string) => { data[k] = v },
      removeItem: (k: string) => { delete data[k] },
      clear: () => { for (const k of Object.keys(data)) delete data[k] },
    },
    writable: true,
    configurable: true,
  })
})

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import type { ReactNode } from 'react'
import { QueryClientProvider } from '@tanstack/react-query'
import { useWebSocket } from './useWebSocket'
import { useConnectionsStore } from '../stores/connectionsStore'
import { clearSeqs, getLastSeq } from './useWSReducer'
import { subscriptionKey } from './useWSProtocol'
import { createTestQueryClient } from '../test/utils'

class MockWS {
  static CONNECTING = 0
  static OPEN = 1
  static CLOSING = 2
  static CLOSED = 3

  url: string
  readyState = MockWS.CONNECTING
  onopen: (() => void) | null = null
  onclose: ((e: { code: number; reason: string }) => void) | null = null
  onerror: ((e: unknown) => void) | null = null
  onmessage: ((e: unknown) => void) | null = null
  binaryType = 'blob'
  close = vi.fn()
  send = vi.fn()

  constructor(url: string) {
    this.url = url
    wsInstances.push(this)
  }
}

const wsInstances: MockWS[] = []

function makeWrapper() {
  const qc = createTestQueryClient()
  return {
    qc,
    Wrapper: function Wrapper({ children }: { children: ReactNode }) {
      return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    },
  }
}

const LOCAL = { id: 'local', name: 'Local', baseURL: '', isLocal: true }

function openSocket(idx: number) {
  wsInstances[idx].readyState = MockWS.OPEN
  wsInstances[idx].onopen?.()
}

function fireClose(idx: number) {
  wsInstances[idx].readyState = MockWS.CLOSED
  wsInstances[idx].onclose?.({ code: 1006, reason: '' })
}

function deliver(idx: number, payload: unknown) {
  wsInstances[idx].onmessage?.({ data: JSON.stringify(payload) })
}

describe('useWebSocket — session channel', () => {
  beforeEach(() => {
    wsInstances.length = 0
    clearSeqs()
    sessionStorage.clear()
    global.WebSocket = MockWS as unknown as typeof WebSocket
    useConnectionsStore.setState({ list: [LOCAL], activeId: 'local' })
    vi.useFakeTimers()
    vi.spyOn(Math, 'random').mockReturnValue(0)
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
    clearSeqs()
    sessionStorage.clear()
    useConnectionsStore.setState({ list: [LOCAL], activeId: 'local' })
  })

  it('subscribeSession sends a subscribe_session message on the open socket', () => {
    const { Wrapper } = makeWrapper()
    const { result, unmount } = renderHook(() => useWebSocket({ enabled: true }), { wrapper: Wrapper })

    openSocket(0)
    act(() => { result.current.subscribeSession('sid-1') })

    expect(wsInstances[0].send).toHaveBeenCalledWith(
      JSON.stringify({ action: 'subscribe_session', session_id: 'sid-1' })
    )
    unmount()
  })

  it('re-sends subscribe_session for every tracked session on reconnect (ws.onopen)', () => {
    const { Wrapper } = makeWrapper()
    const { result, unmount } = renderHook(() => useWebSocket({ enabled: true }), { wrapper: Wrapper })

    openSocket(0)
    act(() => { result.current.subscribeSession('sid-1') })
    act(() => { result.current.subscribeSession('sid-2') })
    wsInstances[0].send.mockClear()

    // Simulate a drop and reconnect
    act(() => { fireClose(0) })
    act(() => { vi.advanceTimersByTime(3_000) })
    expect(wsInstances).toHaveLength(2)

    act(() => { openSocket(1) })

    expect(wsInstances[1].send).toHaveBeenCalledWith(
      JSON.stringify({ action: 'subscribe_session', session_id: 'sid-1' })
    )
    expect(wsInstances[1].send).toHaveBeenCalledWith(
      JSON.stringify({ action: 'subscribe_session', session_id: 'sid-2' })
    )
    unmount()
  })

  it('unsubscribeSession stops it from being resent on the next reconnect', () => {
    const { Wrapper } = makeWrapper()
    const { result, unmount } = renderHook(() => useWebSocket({ enabled: true }), { wrapper: Wrapper })

    openSocket(0)
    act(() => { result.current.subscribeSession('sid-1') })
    act(() => { result.current.unsubscribeSession('sid-1') })

    act(() => { fireClose(0) })
    act(() => { vi.advanceTimersByTime(3_000) })
    act(() => { openSocket(1) })

    expect(wsInstances[1].send).not.toHaveBeenCalledWith(
      expect.stringContaining('subscribe_session')
    )
    unmount()
  })

  it('an envelope carrying session_id never reaches seq tracking, even with a sequence number', () => {
    const { Wrapper } = makeWrapper()
    const { unmount } = renderHook(() => useWebSocket({ enabled: true }), { wrapper: Wrapper })

    openSocket(0)
    const subKey = subscriptionKey('proj-1', 'TICKET-1')
    expect(getLastSeq(subKey)).toBeUndefined()

    // Regression guard: this envelope looks like a normal seq-tracked event
    // (project_id/ticket_id/sequence) but also carries session_id, which
    // must force the early return before dispatchV2Event's seq bookkeeping.
    act(() => {
      deliver(0, {
        type: 'console_chat.delta',
        project_id: 'proj-1',
        ticket_id: 'TICKET-1',
        session_id: 'sid-1',
        sequence: 7,
        protocol_version: 2,
        timestamp: '2026-01-01T00:00:00Z',
        data: { item_id: 'item-1', text: 'hi' },
      })
    })

    expect(getLastSeq(subKey)).toBeUndefined()
    unmount()
  })

  it('a session-scoped messages.updated invalidates only session-messages, not the seq-tracked path', () => {
    const { Wrapper, qc } = makeWrapper()
    const invalidateSpy = vi.spyOn(qc, 'invalidateQueries')
    const { unmount } = renderHook(() => useWebSocket({ enabled: true }), { wrapper: Wrapper })

    openSocket(0)
    act(() => {
      deliver(0, {
        type: 'messages.updated',
        project_id: 'proj-1',
        ticket_id: 'TICKET-1',
        session_id: 'sid-1',
        timestamp: '2026-01-01T00:00:00Z',
      })
    })

    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['session-messages', 'sid-1'] })
    unmount()
  })

  it('surfaces session_subscription_denied via onSessionSubscriptionDenied', () => {
    const onDenied = vi.fn()
    const { Wrapper } = makeWrapper()
    const { unmount } = renderHook(
      () => useWebSocket({ enabled: true, onSessionSubscriptionDenied: onDenied }),
      { wrapper: Wrapper }
    )

    openSocket(0)
    act(() => {
      deliver(0, { type: 'ack', action: 'session_subscription_denied', session_id: 'sid-forbidden' })
    })

    expect(onDenied).toHaveBeenCalledWith('sid-forbidden')
    unmount()
  })
})
