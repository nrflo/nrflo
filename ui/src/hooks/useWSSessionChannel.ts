import { useCallback, useRef, type MutableRefObject } from 'react'
import type { QueryClient } from '@tanstack/react-query'
import type { WSEventV2, WSSubscribeMessage } from './useWSProtocol'
import { throttledInvalidate as inv } from './useWSInvalidate'
import { sessionKeys } from './useSessions'
import { sessionFlowKeys } from './useSessionFlow'

const isDev = import.meta.env.DEV

// Session-channel events are ephemeral (no seq, never event-logged, no
// replay/snapshot — be/internal/ws/hub_session.go): a session-scoped event
// must never reach the seq tracker or snapshot buffer. Live chat state is
// handled by useConsoleChatStream, not the query cache; the Sessions tab
// (list + per-session flow/stats) is the one query-cache consumer here,
// invalidated on session.cost_updated and console_chat.sibling_opened.
export function handleSessionScopedEvent(event: WSEventV2, qc: QueryClient): void {
  if (event.type === 'messages.updated' && event.session_id) {
    qc.invalidateQueries({ queryKey: ['session-messages', event.session_id] })
    return
  }
  if (event.type === 'session.cost_updated' || event.type === 'console_chat.sibling_opened') {
    inv(qc, sessionKeys.all)
    if (event.session_id) {
      inv(qc, sessionFlowKeys.flow(event.session_id))
      inv(qc, sessionFlowKeys.stats(event.session_id))
    }
  }
}

export interface WSSessionChannel {
  sessionSubscriptionsRef: MutableRefObject<Set<string>>
  subscribeSession: (sessionId: string) => void
  unsubscribeSession: (sessionId: string) => void
  resendSessionSubscriptions: (ws: WebSocket) => void
}

// Session-channel (console-chat) subscription bookkeeping, split out of
// useWebSocket.ts to stay under the file size ratchet. The channel is
// authorized server-side per subscribe and ephemeral — no seq/replay/
// snapshot — so it needs none of the cursor machinery the ticket/project
// subscriptions use. See hooks/CLAUDE.md "Session channel".
export function useWSSessionChannel(wsRef: MutableRefObject<WebSocket | null>): WSSessionChannel {
  const sessionSubscriptionsRef = useRef<Set<string>>(new Set())

  const subscribeSession = useCallback((sessionId: string) => {
    sessionSubscriptionsRef.current.add(sessionId)
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      const message: WSSubscribeMessage = { action: 'subscribe_session', session_id: sessionId }
      if (isDev) console.debug('[ws] subscribe_session:', message)
      wsRef.current.send(JSON.stringify(message))
    }
  }, [wsRef])

  const unsubscribeSession = useCallback((sessionId: string) => {
    sessionSubscriptionsRef.current.delete(sessionId)
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      const message: WSSubscribeMessage = { action: 'unsubscribe_session', session_id: sessionId }
      wsRef.current.send(JSON.stringify(message))
    }
  }, [wsRef])

  // Re-send on reconnect — a reconnect silently drops the channel otherwise
  // since the BE authorizes per subscribe, not per connection.
  const resendSessionSubscriptions = useCallback((ws: WebSocket) => {
    sessionSubscriptionsRef.current.forEach((sessionId) => {
      const message: WSSubscribeMessage = { action: 'subscribe_session', session_id: sessionId }
      if (isDev) console.debug('[ws] subscribe_session:', message)
      ws.send(JSON.stringify(message))
    })
  }, [])

  return { sessionSubscriptionsRef, subscribeSession, unsubscribeSession, resendSessionSubscriptions }
}

// A 403 on subscribe_session would otherwise show as a silent empty chat.
export function deniedSessionID(ackMessage: { action?: string; session_id?: string }): string | null {
  return ackMessage.action === 'session_subscription_denied' && ackMessage.session_id ? ackMessage.session_id : null
}
