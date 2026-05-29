// vi.hoisted runs before static imports — provide localStorage so the
// connectionsStore persist middleware does not throw.
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
import { clearSeqs } from './useWSReducer'
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
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  }
}

const LOCAL = { id: 'local', name: 'Local', baseURL: '', isLocal: true }

function fireClose(idx: number) {
  wsInstances[idx].readyState = MockWS.CLOSED
  wsInstances[idx].onclose?.({ code: 1006, reason: '' })
}

// ---- Infinite reconnect ----

describe('useWebSocket — infinite reconnect (no attempt cap)', () => {
  let origWS: typeof WebSocket

  beforeEach(() => {
    wsInstances.length = 0
    clearSeqs()
    sessionStorage.clear()
    origWS = global.WebSocket
    global.WebSocket = MockWS as unknown as typeof WebSocket
    useConnectionsStore.setState({ list: [LOCAL], activeId: 'local' })
    vi.useFakeTimers()
    vi.spyOn(Math, 'random').mockReturnValue(0)
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
    global.WebSocket = origWS
    clearSeqs()
    sessionStorage.clear()
    useConnectionsStore.setState({ list: [LOCAL], activeId: 'local' })
  })

  it('keeps reconnecting past the old 5-attempt cap', () => {
    const { unmount } = renderHook(() => useWebSocket({ enabled: true }), {
      wrapper: makeWrapper(),
    })
    expect(wsInstances).toHaveLength(1)

    for (let i = 0; i < 8; i++) {
      act(() => { fireClose(i) })
      act(() => { vi.advanceTimersByTime(35_000) })
      expect(wsInstances).toHaveLength(i + 2)
    }

    unmount()
  })

  it('reconnect delay is capped: socket at attempt 10 reconnects within 30 seconds', () => {
    const { unmount } = renderHook(() => useWebSocket({ enabled: true }), {
      wrapper: makeWrapper(),
    })

    // Drive through 10 close/reconnect cycles to reach attempt 10
    for (let i = 0; i < 10; i++) {
      act(() => { fireClose(i) })
      act(() => { vi.advanceTimersByTime(35_000) })
    }
    expect(wsInstances).toHaveLength(11)

    act(() => { fireClose(10) })
    // 29999ms: timer has not fired yet (delay = 30000ms with no jitter)
    act(() => { vi.advanceTimersByTime(29_999) })
    expect(wsInstances).toHaveLength(11)
    // 30000ms: timer fires
    act(() => { vi.advanceTimersByTime(1) })
    expect(wsInstances).toHaveLength(12)

    unmount()
  })

  it('onopen resets attempt counter; subsequent close uses base delay (3s)', () => {
    const { result, unmount } = renderHook(() => useWebSocket({ enabled: true }), {
      wrapper: makeWrapper(),
    })

    // Add a subscription so onopen sends a subscribe message
    act(() => { result.current.subscribe('ticket-1') })

    // Build up attempt counter through 3 failures
    for (let i = 0; i < 3; i++) {
      act(() => { fireClose(i) })
      act(() => { vi.advanceTimersByTime(35_000) })
    }
    expect(wsInstances).toHaveLength(4)

    // Open the 4th socket — this resets reconnectAttemptsRef to 0
    const socket3 = wsInstances[3]
    act(() => { socket3.onopen?.() })

    // subscribe message must have been sent
    expect(socket3.send).toHaveBeenCalledWith(
      expect.stringContaining('"action":"subscribe"')
    )

    // Next close: attempts=0 → delay=3000ms (not 24000ms as it would be at attempt 3)
    act(() => { fireClose(3) })
    act(() => { vi.advanceTimersByTime(2_999) })
    expect(wsInstances).toHaveLength(4) // timer hasn't fired yet
    act(() => { vi.advanceTimersByTime(1) })
    expect(wsInstances).toHaveLength(5) // fired exactly at 3000ms

    unmount()
  })
})

// ---- Recovery triggers ----

describe('useWebSocket — online/visibilitychange recovery', () => {
  let origWS: typeof WebSocket

  beforeEach(() => {
    wsInstances.length = 0
    clearSeqs()
    sessionStorage.clear()
    origWS = global.WebSocket
    global.WebSocket = MockWS as unknown as typeof WebSocket
    useConnectionsStore.setState({ list: [LOCAL], activeId: 'local' })
    vi.useFakeTimers()
    vi.spyOn(Math, 'random').mockReturnValue(0)
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
    global.WebSocket = origWS
    clearSeqs()
    sessionStorage.clear()
    useConnectionsStore.setState({ list: [LOCAL], activeId: 'local' })
  })

  it('window online triggers immediate reconnect while socket is down', () => {
    const { unmount } = renderHook(() => useWebSocket({ enabled: true }), {
      wrapper: makeWrapper(),
    })

    act(() => { fireClose(0) }) // pending timer at 3000ms

    // Online event before timer fires: recovery cancels timer and calls connect()
    act(() => { window.dispatchEvent(new Event('online')) })
    expect(wsInstances).toHaveLength(2) // new socket created immediately

    unmount()
  })

  it('visibilitychange to visible triggers immediate reconnect while socket is down', () => {
    Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true })

    const { unmount } = renderHook(() => useWebSocket({ enabled: true }), {
      wrapper: makeWrapper(),
    })

    act(() => { fireClose(0) })
    act(() => { document.dispatchEvent(new Event('visibilitychange')) })
    expect(wsInstances).toHaveLength(2)

    unmount()
  })

  it('online recovery resets attempt counter; next close uses base delay (3s)', () => {
    const { unmount } = renderHook(() => useWebSocket({ enabled: true }), {
      wrapper: makeWrapper(),
    })

    // Drive up attempt counter to 4 (delay would be 30000ms without reset)
    for (let i = 0; i < 4; i++) {
      act(() => { fireClose(i) })
      act(() => { vi.advanceTimersByTime(35_000) })
    }
    expect(wsInstances).toHaveLength(5)

    // Close socket4 → pending timer at 30000ms (attempt=4, capped)
    act(() => { fireClose(4) })
    // Online recovery: clears timer, resets attempts to 0, calls connect()
    act(() => { window.dispatchEvent(new Event('online')) })
    expect(wsInstances).toHaveLength(6)

    // Attempts reset to 0 → next close delay = 3000ms
    act(() => { fireClose(5) })
    act(() => { vi.advanceTimersByTime(2_999) })
    expect(wsInstances).toHaveLength(6)
    act(() => { vi.advanceTimersByTime(1) })
    expect(wsInstances).toHaveLength(7)

    unmount()
  })

  it('window online does not create extra socket when socket is OPEN', () => {
    const { unmount } = renderHook(() => useWebSocket({ enabled: true }), {
      wrapper: makeWrapper(),
    })

    wsInstances[0].readyState = MockWS.OPEN
    act(() => { window.dispatchEvent(new Event('online')) })
    expect(wsInstances).toHaveLength(1)

    unmount()
  })

  it('window online does not create extra socket when socket is CONNECTING', () => {
    const { unmount } = renderHook(() => useWebSocket({ enabled: true }), {
      wrapper: makeWrapper(),
    })

    // readyState = CONNECTING (0) by default in MockWS
    expect(wsInstances[0].readyState).toBe(MockWS.CONNECTING)
    act(() => { window.dispatchEvent(new Event('online')) })
    expect(wsInstances).toHaveLength(1)

    unmount()
  })
})
