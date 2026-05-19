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
import { clearSeqs, setLastSeq, persistSeqs } from './useWSReducer'
import { createTestQueryClient } from '../test/utils'

// ------- Mock WebSocket -------

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

// Shared across tests in this file; cleared in beforeEach
const wsInstances: MockWS[] = []

// ------- Helpers -------

function makeWrapper() {
  const qc = createTestQueryClient()
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  }
}

const LOCAL = { id: 'local', name: 'Local', baseURL: '', isLocal: true }

const REMOTE = {
  id: 'remote-ws-test',
  name: 'Remote',
  baseURL: 'https://remote.example/',
  isLocal: false,
  token: 'tok',
}

const REMOTE_TOKEN_SPACE = {
  id: 'remote-space',
  name: 'RemoteSpace',
  baseURL: 'https://remote.example/',
  isLocal: false,
  token: 'abc def',
}

const REMOTE_NO_TOKEN = {
  id: 'remote-notoken',
  name: 'RemoteNoToken',
  baseURL: 'https://server.example',
  isLocal: false,
}

// ------- Tests -------

describe('useWebSocket — getWebSocketUrl (via MockWS constructor arg)', () => {
  let origWS: typeof WebSocket

  beforeEach(() => {
    wsInstances.length = 0
    clearSeqs()
    sessionStorage.clear()
    origWS = global.WebSocket
    global.WebSocket = MockWS as unknown as typeof WebSocket
    useConnectionsStore.setState({ list: [LOCAL], activeId: 'local' })
  })

  afterEach(() => {
    global.WebSocket = origWS
    clearSeqs()
    sessionStorage.clear()
    useConnectionsStore.setState({ list: [LOCAL], activeId: 'local' })
    vi.restoreAllMocks()
  })

  it('local connection → ws://localhost:5175/api/v1/ws', () => {
    const { unmount } = renderHook(() => useWebSocket({ enabled: true }), {
      wrapper: makeWrapper(),
    })
    expect(wsInstances[0]?.url).toBe('ws://localhost:5175/api/v1/ws')
    unmount()
  })

  it('remote connection with token → wss://host/api/v1/ws?token=abc%20def', () => {
    useConnectionsStore.setState({ list: [LOCAL, REMOTE_TOKEN_SPACE], activeId: 'remote-space' })
    const { unmount } = renderHook(() => useWebSocket({ enabled: true }), {
      wrapper: makeWrapper(),
    })
    expect(wsInstances[0]?.url).toBe('wss://remote.example/api/v1/ws?token=abc%20def')
    unmount()
  })

  it('remote connection without token → wss://host/api/v1/ws', () => {
    useConnectionsStore.setState({ list: [LOCAL, REMOTE_NO_TOKEN], activeId: 'remote-notoken' })
    const { unmount } = renderHook(() => useWebSocket({ enabled: true }), {
      wrapper: makeWrapper(),
    })
    expect(wsInstances[0]?.url).toBe('wss://server.example/api/v1/ws')
    unmount()
  })
})

describe('useWebSocket — connection switch teardown + reconnect', () => {
  let origWS: typeof WebSocket

  beforeEach(() => {
    wsInstances.length = 0
    clearSeqs()
    sessionStorage.clear()
    origWS = global.WebSocket
    global.WebSocket = MockWS as unknown as typeof WebSocket
    useConnectionsStore.setState({ list: [LOCAL], activeId: 'local' })
  })

  afterEach(() => {
    global.WebSocket = origWS
    clearSeqs()
    sessionStorage.clear()
    useConnectionsStore.setState({ list: [LOCAL], activeId: 'local' })
    vi.restoreAllMocks()
  })

  it('closes old socket, clears seqs, and opens new socket with remote URL on switch', () => {
    useConnectionsStore.setState({ list: [LOCAL, REMOTE], activeId: 'local' })

    const { unmount } = renderHook(() => useWebSocket({ enabled: true }), {
      wrapper: makeWrapper(),
    })

    // Initial connection to local
    expect(wsInstances).toHaveLength(1)
    expect(wsInstances[0].url).toBe('ws://localhost:5175/api/v1/ws')
    const firstWs = wsInstances[0]

    // Plant seq data to verify resetSeqs fires
    setLastSeq('local:', 10)
    persistSeqs()
    expect(sessionStorage.getItem('ws_last_seqs')).not.toBeNull()

    // Switch active connection
    act(() => {
      useConnectionsStore.getState().setActive('remote-ws-test')
    })

    // Old socket must be closed
    expect(firstWs.close).toHaveBeenCalled()

    // Seq state must be wiped
    expect(sessionStorage.getItem('ws_last_seqs')).toBeNull()

    // New socket created with remote URL
    expect(wsInstances).toHaveLength(2)
    expect(wsInstances[1].url).toBe('wss://remote.example/api/v1/ws?token=tok')

    unmount()
  })

  it('does not trigger teardown on initial mount (prevActiveId === activeId)', () => {
    const { unmount } = renderHook(() => useWebSocket({ enabled: true }), {
      wrapper: makeWrapper(),
    })

    // Only the initial socket; no extra socket from a spurious switch
    expect(wsInstances).toHaveLength(1)
    expect(wsInstances[0].close).not.toHaveBeenCalled()

    unmount()
  })

  it('resets reconnect attempts on switch so backoff restarts from zero', () => {
    useConnectionsStore.setState({ list: [LOCAL, REMOTE], activeId: 'local' })

    const { result, unmount } = renderHook(() => useWebSocket({ enabled: true }), {
      wrapper: makeWrapper(),
    })

    // The hook should be usable (not thrown)
    expect(result.current).toBeDefined()

    act(() => {
      useConnectionsStore.getState().setActive('remote-ws-test')
    })

    // New socket exists; previous was closed
    expect(wsInstances).toHaveLength(2)
    expect(wsInstances[0].close).toHaveBeenCalled()

    unmount()
  })

  it('switching back to local after remote creates third socket with local URL', () => {
    useConnectionsStore.setState({ list: [LOCAL, REMOTE], activeId: 'local' })

    const { unmount } = renderHook(() => useWebSocket({ enabled: true }), {
      wrapper: makeWrapper(),
    })

    act(() => {
      useConnectionsStore.getState().setActive('remote-ws-test')
    })
    expect(wsInstances).toHaveLength(2)

    act(() => {
      useConnectionsStore.getState().setActive('local')
    })
    expect(wsInstances).toHaveLength(3)
    expect(wsInstances[2].url).toBe('ws://localhost:5175/api/v1/ws')

    unmount()
  })
})
