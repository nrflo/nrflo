import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useConsoleChatStream } from './useConsoleChatStream'
import type { WSEvent } from '@/hooks/useWebSocket'

// ---- WS mock ----
let capturedHandler: ((e: WSEvent) => void) | null = null
const mockSubscribeSession = vi.fn()
const mockUnsubscribeSession = vi.fn()
const mockAddEventListener = vi.fn((fn: (e: WSEvent) => void) => {
  capturedHandler = fn
})
const mockRemoveEventListener = vi.fn()

vi.mock('@/providers/WebSocketProvider', () => ({
  useWebSocketContext: () => ({
    subscribeSession: mockSubscribeSession,
    unsubscribeSession: mockUnsubscribeSession,
    addEventListener: mockAddEventListener,
    removeEventListener: mockRemoveEventListener,
  }),
}))

// The stream should derive cost/contextLeft purely from live WS events, so
// stub the detail/history queries to return no data — no network attempted.
vi.mock('./useConsoleChats', () => ({
  useConsoleChat: () => ({ data: undefined }),
  useConsoleChatMessages: () => ({ data: undefined, isLoading: false }),
}))

function makeEvent(overrides: Partial<WSEvent> = {}): WSEvent {
  return {
    type: 'session.cost_updated',
    project_id: 'p',
    ticket_id: '',
    timestamp: '2026-01-01T00:00:00Z',
    session_id: 's1',
    data: { cost_estimate: 1.5, pricing_known: true },
    ...overrides,
  }
}

describe('useConsoleChatStream', () => {
  let queryClient: QueryClient

  beforeEach(() => {
    vi.clearAllMocks()
    capturedHandler = null
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  })

  function wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  }

  describe('session.cost_updated', () => {
    it('applies cost when session_id matches the chat sid', () => {
      const { result } = renderHook(() => useConsoleChatStream('s1'), { wrapper })
      expect(result.current.cost).toBeUndefined()

      act(() => {
        capturedHandler?.(makeEvent({ session_id: 's1' }))
      })

      expect(result.current.cost).toBe(1.5)
    })

    it('ignores cost for a different (foreign) session_id', () => {
      const { result } = renderHook(() => useConsoleChatStream('s1'), { wrapper })

      act(() => {
        capturedHandler?.(makeEvent({ session_id: 's2' }))
      })

      expect(result.current.cost).toBeUndefined()
    })

    it('ignores cost with no envelope session_id (project-scoped push)', () => {
      const { result } = renderHook(() => useConsoleChatStream('s1'), { wrapper })

      act(() => {
        capturedHandler?.(makeEvent({ session_id: undefined }))
      })

      expect(result.current.cost).toBeUndefined()
    })
  })

  describe('agent.context_updated', () => {
    function contextEvent(sessionId: string | undefined): WSEvent {
      return makeEvent({
        type: 'agent.context_updated',
        session_id: sessionId,
        data: { context_left: 42 },
      })
    }

    it('applies contextLeft when session_id matches the chat sid', () => {
      const { result } = renderHook(() => useConsoleChatStream('s1'), { wrapper })

      act(() => {
        capturedHandler?.(contextEvent('s1'))
      })

      expect(result.current.contextLeft).toBe(42)
    })

    it('ignores contextLeft for a different (foreign) session_id', () => {
      const { result } = renderHook(() => useConsoleChatStream('s1'), { wrapper })

      act(() => {
        capturedHandler?.(contextEvent('s2'))
      })

      expect(result.current.contextLeft).toBeUndefined()
    })

    it('ignores contextLeft with no envelope session_id', () => {
      const { result } = renderHook(() => useConsoleChatStream('s1'), { wrapper })

      act(() => {
        capturedHandler?.(contextEvent(undefined))
      })

      expect(result.current.contextLeft).toBeUndefined()
    })
  })
})
